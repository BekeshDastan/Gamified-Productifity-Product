package analytics

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type ReportReasonSource struct {
	ID      string    `json:"id"`
	Version int64     `json:"version"`
	At      time.Time `json:"at"`
	URL     string    `json:"url"`
}
type ReportReason struct {
	Text    string               `json:"text"`
	Count   int                  `json:"count"`
	Sources []ReportReasonSource `json:"sources"`
}
type ReportObligations struct {
	Planned      int `json:"planned"`
	Completed    int `json:"completed"`
	HabitDue     int `json:"habit_due"`
	HabitSuccess int `json:"habit_success"`
}

func reportObligations(days []Day) ReportObligations {
	var out ReportObligations
	for _, d := range days {
		out.Planned += d.Planned
		out.Completed += d.PlannedCompleted
		out.HabitDue += d.HabitObligations
		out.HabitSuccess += d.HabitSuccesses
	}
	return out
}

// Only explicit postponement reasons participate; diary and goal notes are never read.
func reportReason(ctx context.Context, tx pgx.Tx, owner string, r Range, zone *time.Location) (*ReportReason, error) {
	start, _ := time.ParseInLocation(time.DateOnly, r.From, zone)
	end, _ := time.ParseInLocation(time.DateOnly, r.To, zone)
	var out ReportReason
	e := tx.QueryRow(ctx, `SELECT btrim(reason),count(*) FROM task_events WHERE owner_id=$1 AND kind='postpone' AND created_at >= $2 AND created_at < $3 AND btrim(reason)<>'' GROUP BY btrim(reason) ORDER BY count(*) DESC,min(created_at),btrim(reason) LIMIT 1`, owner, start, end).Scan(&out.Text, &out.Count)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out.Sources = []ReportReasonSource{}
	rows, e := tx.Query(ctx, `SELECT quest_id::text,quest_version,created_at FROM task_events WHERE owner_id=$1 AND kind='postpone' AND created_at >= $2 AND created_at < $3 AND btrim(reason)=$4 ORDER BY created_at,id`, owner, start, end, out.Text)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var s ReportReasonSource
		if e = rows.Scan(&s.ID, &s.Version, &s.At); e != nil {
			return nil, e
		}
		s.URL = "/app/quests/" + s.ID
		out.Sources = append(out.Sources, s)
	}
	return &out, rows.Err()
}
func reportPlanText(v ReportObligations, reason *ReportReason) string {
	text := fmt.Sprintf(" Выполнено плановых обязательств: %d из %d. Полных норм привычек: %d из %d.", v.Completed, v.Planned, v.HabitSuccess, v.HabitDue)
	if reason == nil {
		return text + " Причина переноса не указана."
	}
	return text + fmt.Sprintf(" Самая частая указанная причина переноса: «%s» (%d событий).", reason.Text, reason.Count)
}
