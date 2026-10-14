package analytics

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// TemplateEvidenceInTx reuses canonical completion-time estimates and actual time.
// Callers expose numeric evidence and IDs, never task descriptions or notes.
func (s *Service) TemplateEvidenceInTx(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string) ([]TemplateSample, error) {
	records, e := s.taskTimes(ctx, tx, owner, in, timezone)
	if e != nil {
		return nil, e
	}
	return templateSamples(records), nil
}
