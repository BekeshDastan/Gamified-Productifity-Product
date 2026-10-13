package planning

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

// MorningInTx reuses the owner/date plan and retains its first confirmed snapshot.
// The caller holds the common owner lock and commits the run and plan together.
func (s *Service) MorningInTx(r *http.Request, tx pgx.Tx, owner, date string, in Input, version int64, why string) (Plan, error) {
	if _, e := day(date); e != nil {
		return Plan{}, e
	}
	valid, e := Validate(in)
	if e != nil {
		return Plan{}, e
	}
	old, e := s.get(r.Context(), tx, owner, date)
	if e != nil {
		return old, e
	}
	if old.ConfirmedAt != nil {
		if e = reason(why); e != nil {
			return old, e
		}
	}
	saved, e := s.save(r.Context(), tx, owner, date, valid, version, true)
	if e != nil {
		return saved, e
	}
	if old.ConfirmedAt != nil {
		if e = s.journal(r.Context(), tx, owner, old, saved, "addendum", why); e != nil {
			return saved, e
		}
		return s.view(r.Context(), tx, owner, date)
	}
	request := r.Clone(r.Context())
	request.Header = r.Header.Clone()
	request.Header.Set("If-Match", `"`+strconv.FormatInt(saved.Version, 10)+`"`)
	return s.confirm(request, tx, owner, date)
}
func (s *Service) ViewInTx(ctx context.Context, tx pgx.Tx, owner, date string) (Plan, error) {
	if _, e := day(date); e != nil {
		return Plan{}, e
	}
	return s.view(ctx, tx, owner, date)
}
