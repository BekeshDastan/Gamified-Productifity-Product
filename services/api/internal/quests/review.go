package quests

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ReviewArchiveInTx uses the same task history/projections as an explicit archive.
func (s *Service) ReviewArchiveInTx(ctx context.Context, tx pgx.Tx, owner, id string, version int64, reason string) (Quest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 || strings.ContainsRune(reason, 0) {
		return Quest{}, invalid("reason", "Укажите причину до 1000 символов")
	}
	r := (&http.Request{Header: make(http.Header)}).WithContext(ctx)
	r.Header.Set("If-Match", `"`+strconv.FormatInt(version, 10)+`"`)
	return s.archiveTx(r, tx, owner, id, false, reason)
}
