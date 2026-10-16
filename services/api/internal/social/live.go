package social

import (
	"context"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func changed(ctx context.Context, tx pgx.Tx, owner string) error {
	_, e := tx.Exec(ctx, `UPDATE social_profiles p SET revision=gen_random_uuid() WHERE p.owner_id=$1 OR EXISTS(SELECT 1 FROM social_friends f WHERE ($1=f.sender AND p.owner_id=f.recipient) OR($1=f.recipient AND p.owner_id=f.sender)) OR EXISTS(SELECT 1 FROM social_members a JOIN social_members b ON b.room_id=a.room_id WHERE a.owner_id=$1 AND b.owner_id=p.owner_id)`, owner)
	return e
}
func route[T any](s *Service, r chi.Router, path string, read privateapi.Operation, write func(context.Context, pgx.Tx, string, T) (any, error)) {
	if read != nil {
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, o string) (any, error) {
				if e := lock(ctx, tx, o); e != nil {
					return nil, e
				}
				return read(ctx, tx, o)
			})
			s.Reply(w, r, out, e)
		})
	}
	if write != nil {
		r.Post(path, func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in T) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, o string) (any, error) {
					if e := lock(ctx, tx, o); e != nil {
						return nil, e
					}
					out, e := write(ctx, tx, o, in)
					if e != nil {
						return nil, e
					}
					if e = s.ReconcileInTx(ctx, tx, o); e != nil {
						return nil, e
					}
					return out, changed(ctx, tx, o)
				})
			})
		})
	}
}

type Dashboard struct {
	Owner      string       `json:"owner"`
	Nickname   string       `json:"nickname"`
	Friends    Friends      `json:"friends"`
	Grants     []Grant      `json:"grants"`
	Rooms      []Room       `json:"rooms"`
	Reviews    []Review     `json:"reviews"`
	Files      []ReviewFile `json:"files"`
	Community  Community    `json:"community"`
	Moderation Moderation   `json:"moderation"`
}

func (s *Service) dashboard(ctx context.Context, tx pgx.Tx, o string) (any, error) {
	d := Dashboard{Owner: o}
	var e error
	if e = tx.QueryRow(ctx, "SELECT COALESCE(NULLIF(nickname,''),'Вы') FROM profiles WHERE owner_id=$1", o).Scan(&d.Nickname); e != nil {
		return nil, e
	}
	if d.Friends, e = s.listFriends(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Grants, e = s.grants(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Rooms, e = s.rooms(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Reviews, e = s.reviews(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Files, e = s.reviewFiles(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Community, e = s.community(ctx, tx, o); e != nil {
		return nil, e
	}
	if d.Moderation, e = s.moderation(ctx, tx, o); e != nil {
		return nil, e
	}
	return d, nil
}

// Each connection ends before the router deadline. EventSource reconnects. Only
// an opaque per-viewer invalidation token crosses the stream, never shared data.
func (s *Service) changes(w http.ResponseWriter, r *http.Request) {
	owner, e := privateapi.Owner(r.Context())
	if e != nil {
		s.Reply(w, r, nil, e)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 500\n\n")
	flusher.Flush()
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	until := time.NewTimer(15 * time.Second)
	defer until.Stop()
	last := ""
	for {
		var revision string
		e = s.Pool.QueryRow(r.Context(), "SELECT revision::text FROM social_profiles WHERE owner_id=$1", owner).Scan(&revision)
		if e == nil && revision != last {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", revision)
			flusher.Flush()
			last = revision
		}
		select {
		case <-r.Context().Done():
			return
		case <-until.C:
			_, _ = fmt.Fprint(w, "event: reconnect\ndata: ready\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
		}
	}
}
