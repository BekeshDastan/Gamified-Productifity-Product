package social

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"io"
	"mime"
	"net/http"
)

type ReviewAction struct {
	Action       string `json:"action"`
	ID           string `json:"id"`
	Recipient    string `json:"recipient"`
	AttachmentID string `json:"attachment_id"`
	Consent      bool   `json:"consent"`
	Version      int64  `json:"version"`
}
type Review struct {
	CheckinID string `json:"checkin_id"`
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Reviewer  string `json:"reviewer"`
	RoomID    string `json:"room_id"`
	RoomName  string `json:"room_name"`
	Amount    int64  `json:"amount"`
	Unit      string `json:"unit"`
	State     string `json:"state"`
	Version   int64  `json:"version"`
	File      bool   `json:"file"`
	Current   bool   `json:"matches_version"`
}

const reviewSelect = `SELECT v.id::text,v.checkin_id::text,v.owner_id::text,v.reviewer_id::text,c.room_id::text,r.name,c.amount,r.unit,v.state,v.version,v.attachment_id IS NOT NULL,c.version=v.checkin_version FROM social_reviews v JOIN social_checkins c ON c.id=v.checkin_id JOIN social_rooms r ON r.id=c.room_id `

func scanReview(row pgx.Row) (Review, error) {
	var v Review
	e := row.Scan(&v.ID, &v.CheckinID, &v.Owner, &v.Reviewer, &v.RoomID, &v.RoomName, &v.Amount, &v.Unit, &v.State, &v.Version, &v.File, &v.Current)
	return v, e
}
func (s *Service) review(ctx context.Context, tx pgx.Tx, owner, id string) (Review, error) {
	if !privateapi.UUID(id) {
		return Review{}, missing()
	}
	v, e := scanReview(tx.QueryRow(ctx, reviewSelect+" WHERE v.id=$1 AND $2 IN(v.owner_id,v.reviewer_id)", id, owner))
	if errors.Is(e, pgx.ErrNoRows) {
		return v, missing()
	}
	if e != nil {
		return v, e
	}
	if owner == v.Owner {
		return v, nil
	}
	if e = friends(ctx, tx, v.Owner, v.Reviewer); e != nil {
		return v, e
	}
	r, e := s.room(ctx, tx, owner, v.RoomID)
	if e != nil {
		return v, e
	}
	if r.State != "joined" || v.State == "withdrawn" {
		return v, missing()
	}
	var shared bool
	e = tx.QueryRow(ctx, "SELECT state='joined' AND share FROM social_members WHERE room_id=$1 AND owner_id=$2", v.RoomID, v.Owner).Scan(&shared)
	if e != nil {
		return v, e
	}
	if !shared {
		return v, missing()
	}
	return v, nil
}
func (s *Service) reviewAction(ctx context.Context, tx pgx.Tx, owner string, in ReviewAction) (any, error) {
	now := s.Clock.Now()
	id := in.ID
	var version int64
	state := in.Action
	if in.Action == "request" {
		if !privateapi.UUID(id) || !in.Consent {
			return nil, privateapi.Invalid("Подтвердите передачу отметки выбранному партнёру")
		}
		if e := friends(ctx, tx, owner, in.Recipient); e != nil {
			return nil, e
		}
		var roomID string
		var cv int64
		e := tx.QueryRow(ctx, "SELECT room_id::text,version FROM social_checkins WHERE id=$1 AND owner_id=$2 AND amount>0", id, owner).Scan(&roomID, &cv)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, missing()
		}
		if e != nil {
			return nil, e
		}
		r, e := s.room(ctx, tx, owner, roomID)
		if e != nil {
			return nil, e
		}
		if r.State != "joined" || !r.Share {
			return nil, conflict()
		}
		partner, e := s.room(ctx, tx, in.Recipient, roomID)
		if e != nil {
			return nil, e
		}
		if partner.State != "joined" {
			return nil, missing()
		}
		var file *string
		if in.AttachmentID != "" {
			if !privateapi.UUID(in.AttachmentID) {
				return nil, missing()
			}
			var yes bool
			e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM attachments WHERE owner_id=$1 AND id=$2 AND deleted_at IS NULL AND verdict='clean')", owner, in.AttachmentID).Scan(&yes)
			if e != nil {
				return nil, e
			}
			if !yes {
				return nil, missing()
			}
			file = &in.AttachmentID
		}
		var old int64
		e = tx.QueryRow(ctx, "SELECT version FROM social_reviews WHERE checkin_id=$1", id).Scan(&old)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		if old != in.Version {
			return nil, conflict()
		}
		e = tx.QueryRow(ctx, "INSERT INTO social_reviews(checkin_id,owner_id,reviewer_id,checkin_version,attachment_id,state,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'pending',$6,$6) ON CONFLICT(checkin_id) DO UPDATE SET score_state=CASE WHEN social_reviews.checkin_version=excluded.checkin_version THEN social_reviews.score_state ELSE 'pending' END,reviewer_id=excluded.reviewer_id,checkin_version=excluded.checkin_version,attachment_id=excluded.attachment_id,state='pending',version=social_reviews.version+1,updated_at=excluded.updated_at RETURNING id::text,version", id, owner, in.Recipient, cv, file, now).Scan(&id, &version)
		if e != nil {
			return nil, e
		}
		state = "pending"
	} else {
		v, e := s.review(ctx, tx, owner, id)
		if e != nil {
			return nil, e
		}
		if v.Version != in.Version {
			return nil, conflict()
		}
		if in.Action == "withdrawn" {
			if owner != v.Owner {
				return nil, missing()
			}
		} else if (in.Action != "accepted" && in.Action != "rejected" && in.Action != "disputed") || owner != v.Reviewer || !v.Current || v.State == "withdrawn" {
			return nil, conflict()
		}
		e = tx.QueryRow(ctx, "UPDATE social_reviews SET state=$2,score_state=CASE WHEN $2='withdrawn' THEN score_state ELSE $2 END,version=version+1,updated_at=$3 WHERE id=$1 RETURNING version", id, state, now).Scan(&version)
		if e != nil {
			return nil, e
		}
	}
	_, e := tx.Exec(ctx, "INSERT INTO social_review_events(review_id,actor_id,state,version,created_at) VALUES($1,$2,$3,$4,$5)", id, owner, state, version, now)
	return receipt(id), e
}
func (s *Service) reviews(ctx context.Context, tx pgx.Tx, owner string) ([]Review, error) {
	rows, e := tx.Query(ctx, "SELECT id::text FROM social_reviews WHERE $1 IN(owner_id,reviewer_id) ORDER BY updated_at DESC LIMIT 100", owner)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []Review{}
	for _, id := range ids {
		v, e := s.review(ctx, tx, owner, id)
		if isMissing(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

type ReviewFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) reviewFiles(ctx context.Context, tx pgx.Tx, owner string) ([]ReviewFile, error) {
	out := []ReviewFile{}
	rows, e := tx.Query(ctx, "SELECT id::text,name FROM attachments WHERE owner_id=$1 AND deleted_at IS NULL AND verdict='clean' ORDER BY created_at DESC LIMIT 100", owner)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var v ReviewFile
		if e = rows.Scan(&v.ID, &v.Name); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) downloadReview(w http.ResponseWriter, r *http.Request) {
	type file struct {
		name, kind string
		data       []byte
	}
	out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
		if e := lock(ctx, tx, owner); e != nil {
			return nil, e
		}
		v, e := s.review(ctx, tx, owner, chi.URLParam(r, "id"))
		if e != nil {
			return nil, e
		}
		if !v.File || !v.Current || v.State == "withdrawn" || s.files == nil {
			return nil, missing()
		}
		var key string
		var f file
		e = tx.QueryRow(ctx, "SELECT a.storage_key,a.name,a.mime FROM social_reviews v JOIN attachments a ON a.owner_id=v.owner_id AND a.id=v.attachment_id WHERE v.id=$1 AND a.deleted_at IS NULL AND a.verdict='clean'", v.ID).Scan(&key, &f.name, &f.kind)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, missing()
		}
		if e != nil {
			return nil, e
		}
		reader, e := s.files.Get(ctx, key)
		if e != nil {
			return nil, missing()
		}
		defer reader.Close()
		f.data, e = io.ReadAll(io.LimitReader(reader, 10<<20))
		return f, e
	})
	if e != nil {
		s.Reply(w, r, nil, e)
		return
	}
	f := out.(file)
	w.Header().Set("Content-Type", f.kind)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.name}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	_, _ = w.Write(f.data)
}
