package notifications

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Service) ConfigureDelivery(origin string, key []byte, mailer mail.Mailer) error {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || len(key) != 32 {
		return errors.New("invalid notification delivery configuration")
	}
	s.origin = strings.TrimSuffix(origin, "/")
	derive := hmac.New(sha256.New, key)
	derive.Write([]byte("ascend/notification-unsubscribe/v1"))
	s.unsubscribeKey = derive.Sum(nil)
	s.mailer = mailer
	return nil
}

type unsubscribeClaims struct {
	Owner   string `json:"owner"`
	Type    string `json:"type"`
	Expires int64  `json:"expires"`
	Version int    `json:"version"`
}

func (s *Service) unsubscribeToken(owner, kind string) string {
	raw, _ := json.Marshal(unsubscribeClaims{owner, kind, s.Clock.Now().AddDate(1, 0, 0).Unix(), 1})
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.unsubscribeKey)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) unsubscribeClaims(token string) (unsubscribeClaims, error) {
	var out unsubscribeClaims
	bad := errors.New("invalid unsubscribe token")
	if len(s.unsubscribeKey) != 32 || len(token) > 2048 {
		return out, bad
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return out, bad
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return out, bad
	}
	mac := hmac.New(sha256.New, s.unsubscribeKey)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(mac.Sum(nil), sig) {
		return out, bad
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil || json.Unmarshal(raw, &out) != nil || out.Version != 1 || !privateapi.UUID(out.Owner) || out.Expires <= s.Clock.Now().Unix() {
		return out, bad
	}
	if _, ok := titles[out.Type]; !ok {
		return out, bad
	}
	return out, nil
}
func (s *Service) mountUnsubscribe(r chi.Router) {
	r.Post("/api/v1/notifications/unsubscribe/preview", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Token string `json:"token"`
		}) (any, error) {
			if r.Header.Get("Origin") != s.origin || s.origin == "" {
				return nil, privateapi.Problem(403, "ORIGIN_REJECTED", "Откройте ссылку в приложении")
			}
			claims, e := s.unsubscribeClaims(in.Token)
			if e != nil {
				return nil, privateapi.Invalid("Ссылка недействительна или устарела")
			}
			return map[string]string{"type": claims.Type, "title": titles[claims.Type]}, nil
		})
	})
	r.Post("/api/v1/notifications/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Token   string `json:"token"`
			Confirm bool   `json:"confirm"`
		}) (any, error) {
			if r.Header.Get("Origin") != s.origin || s.origin == "" {
				return nil, privateapi.Problem(403, "ORIGIN_REJECTED", "Откройте ссылку в приложении")
			}
			claims, e := s.unsubscribeClaims(in.Token)
			if e != nil || !in.Confirm {
				return nil, privateapi.Invalid("Подтвердите отключение писем по действующей ссылке")
			}
			e = storage.WithinTx(r.Context(), s.Pool, func(tx pgx.Tx) error {
				var owner string
				if e := tx.QueryRow(r.Context(), `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, claims.Owner).Scan(&owner); e != nil {
					return e
				}
				p, e := s.preferences(r.Context(), tx, owner)
				if e != nil {
					return e
				}
				if !p.Channel(claims.Type).Email {
					return nil
				}
				input := p.PreferencesInput
				for i := range input.Channels {
					if input.Channels[i].Type == claims.Type {
						input.Channels[i].Email = false
					}
				}
				request := r.Clone(r.Context())
				request.Header = r.Header.Clone()
				request.Header.Set("If-Match", `"`+strconv.FormatInt(p.Version, 10)+`"`)
				if _, e = s.savePreferences(request, tx, owner, input); e != nil {
					return e
				}
				return jobs.OwnerChanged(r.Context(), tx, owner, s.Clock)
			})
			return map[string]bool{"unsubscribed": e == nil}, e
		})
	})
}
func (s *Service) mailMessage(owner string, n Notification, email string) mail.Message {
	return mail.Message{To: email, Subject: n.Title + " · ASCEND", Text: n.Message + "\n\nЛичный центр уведомлений: " + s.origin + "/app/notifications\n\nОтключить письма этого типа: " + s.origin + "/notifications/unsubscribe#token=" + s.unsubscribeToken(owner, n.Kind)}
}
