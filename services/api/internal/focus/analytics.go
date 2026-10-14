package focus

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// QuestTimesInTx is the bulk equivalent of QuestTimeInTx; sources remain mutually exclusive.
func QuestTimesInTx(ctx context.Context, tx pgx.Tx, owner string, ids []string) (map[string]QuestTime, error) {
	out := map[string]QuestTime{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,COALESCE(q.payload->>'actual_time_mode',CASE WHEN COALESCE((q.payload->>'actual_minutes')::bigint,0)>0 THEN 'manual_override' ELSE 'from_focus' END),COALESCE((q.payload->>'actual_minutes')::bigint,0),COALESCE(total.seconds,0),COALESCE(total.sessions,0) FROM quests q LEFT JOIN LATERAL(SELECT sum(s.seconds)::bigint seconds,count(*) FILTER(WHERE s.seconds>0) sessions FROM focus_sessions f CROSS JOIN LATERAL(SELECT COALESCE(sum(floor(extract(epoch FROM(ended_at-started_at)))),0)::bigint seconds FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work' AND ended_at IS NOT NULL) s WHERE f.owner_id=q.owner_id AND f.quest_id=q.id AND f.state='finished' AND f.result<>'rest') total ON true WHERE q.owner_id=$1 AND q.id=ANY($2::uuid[])`, owner, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var v QuestTime
		if e = rows.Scan(&v.QuestID, &v.Mode, &v.ManualMinutes, &v.FocusSeconds, &v.ConfirmedSessions); e != nil {
			return nil, e
		}
		v.Overridden = v.Mode == "manual_override"
		v.EffectiveSeconds = v.FocusSeconds
		if v.Overridden {
			v.EffectiveSeconds = v.ManualMinutes * 60
		}
		out[v.QuestID] = v
	}
	return out, rows.Err()
}
