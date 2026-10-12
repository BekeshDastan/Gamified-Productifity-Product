package planning

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (s *Service) week(ctx context.Context, tx pgx.Tx, id, from string) ([]Plan, error) {
	d, e := day(from)
	if e != nil {
		return nil, e
	}
	if e = s.lock(ctx, tx, id); e != nil {
		return nil, e
	}
	out := []Plan{}
	for i := 0; i < 7; i++ {
		date := d.AddDate(0, 0, i)
		p, e := s.get(ctx, tx, id, date.Format("2006-01-02"))
		if e != nil {
			return nil, e
		}
		a, b, e := bounds(p.Date, p.Timezone)
		if e != nil {
			return nil, e
		}
		blocks, e := s.calendar.PreviewTimedInTx(ctx, tx, id, date, date.AddDate(0, 0, 1), a, b)
		if e != nil {
			return nil, e
		}
		windows, e := windowsFor(p.Date, p.Timezone, p.Windows)
		if e != nil {
			return nil, e
		}
		available, e := calculate(p, windows, blocks)
		if e != nil {
			return nil, e
		}
		p.Availability = &available
		out = append(out, p)
	}
	return out, nil
}
func (s *Service) mountWeek(r chi.Router) {
	r.Get("/api/v1/plans/week", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		var out []Plan
		if e == nil {
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				var err error
				out, err = s.week(r.Context(), tx, id, r.URL.Query().Get("from"))
				return err
			})
		}
		s.reply(w, r, out, e)
	})
}
