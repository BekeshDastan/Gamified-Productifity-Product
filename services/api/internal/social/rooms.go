package social

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
	"unicode/utf8"
)

type RoomAction struct {
	Action      string `json:"action"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Metric      string `json:"metric"`
	Unit        string `json:"unit"`
	Target      int64  `json:"target"`
	DailyCap    int64  `json:"daily_cap"`
	Days        int    `json:"days"`
	Recipient   string `json:"recipient"`
	RulesHash   string `json:"rules_hash"`
	Consent     bool   `json:"consent"`
	Leaderboard bool   `json:"leaderboard"`
	Version     int64  `json:"version"`
	Amount      int64  `json:"amount"`
}
type Standing struct {
	UserID       string `json:"user_id"`
	Nickname     string `json:"nickname"`
	Amount       int64  `json:"amount"`
	Rank         int    `json:"rank"`
	Earned       bool   `json:"earned"`
	Confirmation string `json:"confirmation"`
}
type Checkin struct {
	ID      string `json:"id"`
	Amount  int64  `json:"amount"`
	Version int64  `json:"version"`
}
type Room struct {
	ID          string     `json:"id"`
	Owner       string     `json:"owner"`
	Name        string     `json:"name"`
	Mode        string     `json:"mode"`
	Metric      string     `json:"metric"`
	Unit        string     `json:"unit"`
	Target      int64      `json:"target"`
	DailyCap    int64      `json:"daily_cap"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	RulesHash   string     `json:"rules_hash"`
	FinalizedAt *time.Time `json:"finalized_at"`
	State       string     `json:"state"`
	Share       bool       `json:"share"`
	Leaderboard bool       `json:"leaderboard"`
	Version     int64      `json:"version"`
	Total       int64      `json:"total"`
	Members     []Standing `json:"members"`
	Checkin     *Checkin   `json:"checkin"`
}

const roomColumns = `r.id::text,r.owner_id::text,r.name,r.mode,r.metric,r.unit,r.target,r.daily_cap,r.starts_at,r.ends_at,r.rules_hash,r.finalized_at,m.state,m.share,m.leaderboard,m.version`

func scanRoom(row pgx.Row) (Room, error) {
	r := Room{Members: []Standing{}}
	e := row.Scan(&r.ID, &r.Owner, &r.Name, &r.Mode, &r.Metric, &r.Unit, &r.Target, &r.DailyCap, &r.StartsAt, &r.EndsAt, &r.RulesHash, &r.FinalizedAt, &r.State, &r.Share, &r.Leaderboard, &r.Version)
	return r, e
}
func (s *Service) room(ctx context.Context, tx pgx.Tx, owner, id string) (Room, error) {
	if !privateapi.UUID(id) {
		return Room{}, missing()
	}
	r, e := scanRoom(tx.QueryRow(ctx, "SELECT "+roomColumns+" FROM social_rooms r JOIN social_members m ON m.room_id=r.id AND m.owner_id=$2 WHERE r.id=$1", id, owner))
	if errors.Is(e, pgx.ErrNoRows) {
		return r, missing()
	}
	if e != nil {
		return r, e
	}
	if owner != r.Owner {
		if e = friends(ctx, tx, owner, r.Owner); e != nil {
			return r, e
		}
	}
	return r, nil
}
func validRoom(in RoomAction) bool {
	if (in.Mode != "coop" && in.Mode != "pvp") || (in.Mode == "pvp" && in.Target > in.DailyCap*int64(in.Days)) || in.Days < 1 || in.Days > 30 || in.DailyCap < 1 || in.Target < 1 || in.Target > in.DailyCap*int64(in.Days)*20 || utf8.RuneCountInString(strings.TrimSpace(in.Name)) < 1 || utf8.RuneCountInString(in.Name) > 80 || strings.ContainsAny(in.Name, "\x00\r\n") {
		return false
	}
	switch in.Metric {
	case "tasks":
		return in.Unit == "tasks" && in.DailyCap <= 100
	case "focus":
		return in.Unit == "minutes" && in.DailyCap <= 240
	case "reviews":
		return in.Unit == "reviews" && in.DailyCap == 1
	case "manual":
		return (in.Unit == "count" || in.Unit == "pages" || in.Unit == "minutes") && in.DailyCap <= 1000
	}
	return false
}
func (s *Service) roomAction(ctx context.Context, tx pgx.Tx, owner string, in RoomAction) (any, error) {
	now := s.Clock.Now().UTC()
	if in.Action == "create" {
		if e := verified(ctx, tx, owner); e != nil {
			return nil, e
		}
		if !validRoom(in) || !in.Consent {
			return nil, privateapi.Invalid("Проверьте правила и согласие")
		}
		var count int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM social_rooms WHERE owner_id=$1 AND created_at>$2::timestamptz-interval '24 hours'", owner, now).Scan(&count); e != nil {
			return nil, e
		}
		if count >= 10 {
			return nil, limit()
		}
		start := now
		if in.Mode == "pvp" {
			start = now.Truncate(24 * time.Hour).Add(24 * time.Hour)
		}
		end := start.Truncate(24*time.Hour).AddDate(0, 0, in.Days)
		body, _ := json.Marshal(struct {
			Input      RoomAction
			Start, End time.Time
			Version    string
		}{in, start, end, "social-v1"})
		hash := sha256.Sum256(body)
		var id string
		e := tx.QueryRow(ctx, "INSERT INTO social_rooms(owner_id,name,mode,metric,unit,target,daily_cap,starts_at,ends_at,rules_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text", owner, strings.TrimSpace(in.Name), in.Mode, in.Metric, in.Unit, in.Target, in.DailyCap, start, end, hex.EncodeToString(hash[:]), now).Scan(&id)
		if e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO social_members(room_id,owner_id,state,share,leaderboard,joined_at) VALUES($1,$2,'joined',true,$3,$4)", id, owner, in.Leaderboard, now)
		if e != nil {
			return nil, e
		}
		return receipt(id), audit(ctx, tx, owner, "room_created", id, now)
	}
	r, e := s.room(ctx, tx, owner, in.ID)
	if e != nil {
		return nil, e
	}
	switch in.Action {
	case "finalize":
		if r.State != "joined" {
			return nil, conflict()
		}
		if e = s.fillRoom(ctx, tx, owner, &r); e != nil {
			return nil, e
		}
		if e = s.settle(ctx, tx, r); e != nil {
			return nil, e
		}
	case "invite":
		if owner != r.Owner || r.State != "joined" || !now.Before(r.EndsAt) || (r.Mode == "pvp" && !now.Before(r.StartsAt)) {
			return nil, conflict()
		}
		if e = verified(ctx, tx, owner); e != nil {
			return nil, e
		}
		if e = friends(ctx, tx, owner, in.Recipient); e != nil {
			return nil, e
		}
		var count int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM social_members WHERE room_id=$1", r.ID).Scan(&count); e != nil {
			return nil, e
		}
		if count >= 20 {
			return nil, limit()
		}
		tag, e := tx.Exec(ctx, "INSERT INTO social_members(room_id,owner_id,state) VALUES($1,$2,'invited') ON CONFLICT DO NOTHING", r.ID, in.Recipient)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, conflict()
		}
	case "join":
		if e = verified(ctx, tx, owner); e != nil {
			return nil, e
		}
		if r.State != "invited" || r.Version != in.Version || !in.Consent || r.RulesHash != in.RulesHash || !now.Before(r.EndsAt) || (r.Mode == "pvp" && !now.Before(r.StartsAt)) {
			return nil, conflict()
		}
		_, e = tx.Exec(ctx, "UPDATE social_members SET state='joined',share=true,leaderboard=$3,joined_at=$4,version=version+1 WHERE room_id=$1 AND owner_id=$2", r.ID, owner, in.Leaderboard, now)
	case "leave", "reject":
		if r.Version != in.Version || r.State != "joined" && r.State != "invited" {
			return nil, conflict()
		}
		_, e = tx.Exec(ctx, "UPDATE social_members SET state='left',share=false,leaderboard=false,version=version+1 WHERE room_id=$1 AND owner_id=$2", r.ID, owner)
	case "sharing":
		if r.Version != in.Version || r.State != "joined" {
			return nil, conflict()
		}
		_, e = tx.Exec(ctx, "UPDATE social_members SET share=$3,leaderboard=$4,version=version+1 WHERE room_id=$1 AND owner_id=$2", r.ID, owner, in.Consent, in.Leaderboard && in.Consent)
	case "checkin":
		if r.State != "joined" || r.Metric != "manual" || now.Before(r.StartsAt) || !now.Before(r.EndsAt) || in.Amount < 0 || in.Amount > r.DailyCap {
			return nil, privateapi.Invalid("Отметка вне правил испытания")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO social_checkins(room_id,owner_id,local_date,amount,created_at,updated_at) SELECT $1,$2,$3,$4,$5,$5 WHERE $6=0 ON CONFLICT(room_id,owner_id,local_date) DO NOTHING`, r.ID, owner, now.Format("2006-01-02"), in.Amount, now, in.Version)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			tag, e = tx.Exec(ctx, "UPDATE social_checkins SET amount=$4,version=version+1,updated_at=$5 WHERE room_id=$1 AND owner_id=$2 AND local_date=$3 AND version=$6", r.ID, owner, now.Format("2006-01-02"), in.Amount, now, in.Version)
			if e != nil {
				return nil, e
			}
			if tag.RowsAffected() == 0 {
				return nil, conflict()
			}
		}
	default:
		return nil, privateapi.Invalid("Неизвестное действие")
	}
	if e != nil {
		return nil, e
	}
	return receipt(r.ID), audit(ctx, tx, owner, "room_"+in.Action, r.ID, now)
}

// Sources are unique ledger facts; titles, notes and private URLs are never selected.
func (s *Service) amount(ctx context.Context, tx pgx.Tx, r Room, owner string, joined time.Time) (int64, error) {
	start := joined
	if start.Before(r.StartsAt) {
		start = r.StartsAt
	}
	end := s.Clock.Now()
	if end.After(r.EndsAt) {
		end = r.EndsAt
	}
	sources := ""
	switch r.Metric {
	case "tasks":
		sources = `SELECT e.first_awarded_at at,1::bigint amount FROM reward_entitlements e JOIN quests q ON q.owner_id=e.owner_id AND q.id::text=e.source_key WHERE e.owner_id=$1 AND e.source_kind='task' AND e.active AND q.status='completed' AND NOT EXISTS(SELECT 1 FROM quests child WHERE child.owner_id=q.owner_id AND child.parent_id=q.id)`
	case "focus":
		sources = `SELECT f.updated_at at,COALESCE((SELECT sum(floor(extract(epoch FROM(s.ended_at-s.started_at))))::bigint FROM focus_segments s WHERE s.owner_id=f.owner_id AND s.session_id=f.id AND s.kind='work' AND s.ended_at IS NOT NULL),0) amount FROM focus_sessions f WHERE f.owner_id=$1 AND f.state='finished' AND f.result<>'rest' AND f.started_at>=$2`
	case "reviews":
		sources = `SELECT created_at at,1::bigint amount FROM daily_reviews WHERE owner_id=$1`
	case "manual":
		sources = `SELECT created_at at,amount FROM social_checkins c WHERE owner_id=$1 AND room_id=$5 AND NOT EXISTS(SELECT 1 FROM social_reviews v WHERE v.checkin_id=c.id AND v.checkin_version=c.version AND v.score_state IN('rejected','disputed'))`
	}
	scale := int64(1)
	if r.Metric == "focus" {
		scale = 60
	}
	var n int64
	e := tx.QueryRow(ctx, `WITH args AS(SELECT $1::uuid,$2::timestamptz,$3::timestamptz,$4::bigint,$5::uuid,$6::bigint), sources AS (`+sources+`), days AS(SELECT (at AT TIME ZONE 'UTC')::date AS local_day,LEAST(sum(amount),$4*$6) amount FROM sources WHERE at>=$2 AND at<$3 GROUP BY 1) SELECT COALESCE(sum(amount),0)::bigint/$6 FROM days`, owner, start, end, r.DailyCap, r.ID, scale).Scan(&n)
	return n, e
}
func (s *Service) fillRoom(ctx context.Context, tx pgx.Tx, viewer string, r *Room) error {
	if r.State != "joined" {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT m.owner_id::text,COALESCE(NULLIF(p.nickname,''),'Участник'),m.joined_at,m.leaderboard FROM social_members m JOIN profiles p ON p.owner_id=m.owner_id WHERE m.room_id=$1 AND m.state='joined' AND m.share AND NOT EXISTS(SELECT 1 FROM social_blocks b WHERE (b.owner_id=$2 AND b.target_id=m.owner_id) OR(b.target_id=$2 AND b.owner_id=m.owner_id)) AND (m.owner_id=$3 OR EXISTS(SELECT 1 FROM social_friends f WHERE f.state='accepted' AND ((f.sender=m.owner_id AND f.recipient=$3) OR(f.recipient=m.owner_id AND f.sender=$3)))) ORDER BY m.owner_id`, r.ID, viewer, r.Owner)
	if e != nil {
		return e
	}
	type member struct {
		id, name string
		joined   time.Time
		board    bool
	}
	all := []member{}
	for rows.Next() {
		var m member
		if e = rows.Scan(&m.id, &m.name, &m.joined, &m.board); e != nil {
			rows.Close()
			return e
		}
		all = append(all, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, m := range all {
		n, e := s.amount(ctx, tx, *r, m.id, m.joined)
		if e != nil {
			return e
		}
		if r.Mode == "pvp" && !m.board {
			continue
		}
		r.Total += n
		confirmation := "recorded"
		if r.Metric == "manual" {
			confirmation = "self_report"
			var confirmed bool
			e = tx.QueryRow(ctx, `SELECT count(*)>0 AND bool_and(EXISTS(SELECT 1 FROM social_reviews v WHERE v.checkin_id=c.id AND v.checkin_version=c.version AND v.state='accepted')) FROM social_checkins c WHERE c.owner_id=$1 AND c.room_id=$2 AND c.amount>0`, m.id, r.ID).Scan(&confirmed)
			if e != nil {
				return e
			}
			if confirmed {
				confirmation = "partner_confirmed"
			}
		}
		r.Members = append(r.Members, Standing{UserID: m.id, Nickname: m.name, Amount: n, Confirmation: confirmation})
	}
	if r.Mode == "pvp" {
		rankStandings(r.Members)
	}
	for i := range r.Members {
		m := &r.Members[i]
		m.Earned = r.FinalizedAt != nil && m.Amount > 0 && ((r.Mode == "coop" && r.Total >= r.Target) || (r.Mode == "pvp" && m.Rank == 1 && len(r.Members) > 1))
	}
	var c Checkin
	e = tx.QueryRow(ctx, "SELECT id::text,amount,version FROM social_checkins WHERE room_id=$1 AND owner_id=$2 AND local_date=$3", r.ID, viewer, s.Clock.Now().UTC().Format("2006-01-02")).Scan(&c.ID, &c.Amount, &c.Version)
	if e == nil {
		r.Checkin = &c
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	return nil
}
func (s *Service) rooms(ctx context.Context, tx pgx.Tx, owner string) ([]Room, error) {
	rows, e := tx.Query(ctx, "SELECT r.id::text FROM social_rooms r JOIN social_members m ON m.room_id=r.id AND m.owner_id=$1 WHERE m.state IN('invited','joined') ORDER BY r.created_at DESC LIMIT 100", owner)
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
	out := []Room{}
	for _, id := range ids {
		r, e := s.room(ctx, tx, owner, id)
		if e != nil {
			var p *platform.APIError

			if errors.As(e, &p) && p.Status == 404 {
				continue
			}
			return nil, e
		}
		if e = s.fillRoom(ctx, tx, owner, &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
