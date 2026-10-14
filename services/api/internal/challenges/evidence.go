package challenges

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type EvidenceInput struct {
	Visibility string `json:"visibility"`
	Consent    bool   `json:"consent"`
}
type Evidence struct {
	AttachmentID string     `json:"attachment_id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Visibility   string     `json:"visibility"`
	ConsentAt    *time.Time `json:"consent_at"`
	Version      int64      `json:"version"`
	Available    bool       `json:"available"`
	Confirmation string     `json:"confirmation"`
}

// Consent records this attachment's intended audience. Actual download remains
// owner-only until a separate participant authorization grants access.
func (s *Service) setEvidence(r *http.Request, tx pgx.Tx, owner, runID, checkinID, fileID string, in EvidenceInput) (Evidence, error) {
	out := Evidence{}
	if _, e := s.getCheckin(r.Context(), tx, owner, runID, checkinID); e != nil {
		return out, e
	}
	if !privateapi.UUID(fileID) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Файл не найден")
	}
	if (in.Visibility != "private" && in.Visibility != "participants") || (in.Visibility == "participants" && !in.Consent) || (in.Visibility == "private" && in.Consent) {
		return out, privateapi.Invalid("Для видимости участникам нужно отдельное согласие; частный файл его не требует")
	}
	var name, verdict string
	e := tx.QueryRow(r.Context(), "SELECT name,verdict FROM attachments WHERE owner_id=$1 AND id=$2 AND deleted_at IS NULL", owner, fileID).Scan(&name, &verdict)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Файл не найден")
	}
	if e != nil {
		return out, e
	}
	if verdict != "clean" {
		return out, privateapi.Problem(409, "FILE_NOT_READY", "Используйте файл, прошедший проверку")
	}
	var version int64
	var oldVisibility string
	var consent *time.Time
	e = tx.QueryRow(r.Context(), "SELECT version,visibility,consent_at FROM challenge_evidence WHERE owner_id=$1 AND checkin_id=$2 AND attachment_id=$3", owner, checkinID, fileID).Scan(&version, &oldVisibility, &consent)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if p := privateapi.MatchVersion(r, version); p != nil {
		return out, p
	}
	now := s.Clock.Now()
	if in.Visibility == "participants" && consent == nil {
		consent = &now
	}
	if in.Visibility == "private" {
		consent = nil
	}
	if version == 0 {
		var n int
		if e = tx.QueryRow(r.Context(), "SELECT count(*) FROM challenge_evidence WHERE owner_id=$1 AND checkin_id=$2", owner, checkinID).Scan(&n); e != nil {
			return out, e
		}
		if n >= 10 {
			return out, privateapi.Problem(409, "EVIDENCE_LIMIT", "Не более десяти файлов на отметку")
		}
	}
	if version == 0 || oldVisibility != in.Visibility {
		e = tx.QueryRow(r.Context(), "INSERT INTO challenge_evidence(owner_id,checkin_id,attachment_id,visibility,consent_at,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,checkin_id,attachment_id) DO UPDATE SET visibility=excluded.visibility,consent_at=excluded.consent_at,version=challenge_evidence.version+1,updated_at=excluded.updated_at RETURNING version", owner, checkinID, fileID, in.Visibility, consent, now).Scan(&version)
		if e != nil {
			return out, e
		}
	}
	out = Evidence{AttachmentID: fileID, Name: name, URL: "/api/v1/attachments/" + fileID + "/download", Visibility: in.Visibility, ConsentAt: consent, Version: version, Available: true, Confirmation: "self_report"}
	body, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO challenge_evidence_events(owner_id,checkin_id,attachment_id,version,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", owner, checkinID, fileID, version, body, now)
	return out, e
}
func (s *Service) evidence(ctx context.Context, tx pgx.Tx, owner, runID, checkinID string) ([]Evidence, error) {
	out := []Evidence{}
	if _, e := s.getCheckin(ctx, tx, owner, runID, checkinID); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT e.attachment_id::text,a.name,e.visibility,e.consent_at,e.version,(a.deleted_at IS NULL AND a.verdict='clean') FROM challenge_evidence e JOIN attachments a ON a.owner_id=e.owner_id AND a.id=e.attachment_id WHERE e.owner_id=$1 AND e.checkin_id=$2 ORDER BY e.attachment_id", owner, checkinID)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v := Evidence{Confirmation: "self_report"}
		if e = rows.Scan(&v.AttachmentID, &v.Name, &v.Visibility, &v.ConsentAt, &v.Version, &v.Available); e != nil {
			return out, e
		}
		if v.Available {
			v.URL = "/api/v1/attachments/" + v.AttachmentID + "/download"
		} else {
			v.Name = "Файл недоступен"
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) mountEvidence(r chi.Router) {
	r.Get("/api/v1/challenges/runs/{id}/check-ins/{checkin}/evidence", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.evidence(ctx, tx, owner, chi.URLParam(r, "id"), chi.URLParam(r, "checkin"))
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/challenges/runs/{id}/check-ins/{checkin}/evidence/{file}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in EvidenceInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.setEvidence(r, tx, owner, chi.URLParam(r, "id"), chi.URLParam(r, "checkin"), chi.URLParam(r, "file"), in)
			})
		})
	})
}
