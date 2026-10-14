package goals

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/jackc/pgx/v5"
)

// CreateInTx shares ordinary goal validation/history with confirmed structured
// flows. The caller owns the transaction and common owner lock.
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, owner string, in Create) (Goal, error) {
	valid, e := Validate(in.Input)
	if e != nil {
		return Goal{}, e
	}
	in.Input = valid
	if in.State != "active" && in.State != "draft" {
		return Goal{}, invalid("state", "Новая цель: черновик или активная")
	}
	if e = s.lock(ctx, tx, owner); e != nil {
		return Goal{}, e
	}
	return s.insert(ctx, tx, owner, in)
}

// AttachNewQuestInTx only attaches an owned unparented initial-version root
// without existing goal membership. It never transfers or rewrites a quest.
func (s *Service) AttachNewQuestInTx(ctx context.Context, tx pgx.Tx, owner string, g Goal, q quests.Quest) (Goal, error) {
	if e := s.lock(ctx, tx, owner); e != nil {
		return g, e
	}
	fresh, e := s.get(ctx, tx, owner, g.ID)
	if e != nil {
		return g, e
	}
	if fresh.Version != g.Version || fresh.ArchivedAt != nil || (fresh.State != "active" && fresh.State != "draft") {
		return fresh, problem(409, "VERSION_CONFLICT", "Цель изменилась")
	}
	var available bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM quests q WHERE q.owner_id=$1 AND q.id=$2 AND q.version=1 AND q.parent_id IS NULL AND q.archived_at IS NULL AND q.status IN('todo','backlog') AND NOT EXISTS(SELECT 1 FROM goal_quests l WHERE l.owner_id=q.owner_id AND l.quest_id=q.id))", owner, q.ID).Scan(&available)
	if e != nil {
		return g, e
	}
	if !available {
		return g, problem(409, "QUEST_NOT_NEW", "Нужна новая собственная задача без прежней связи")
	}
	var count int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM goal_quests WHERE owner_id=$1 AND goal_id=$2", owner, g.ID).Scan(&count); e != nil {
		return g, e
	}
	if count >= 500 {
		return g, invalid("quest_id", "Не более 500 прямых связей")
	}
	if _, e = tx.Exec(ctx, "INSERT INTO goal_quests(owner_id,goal_id,quest_id,required,weight) VALUES($1,$2,$3,true,1)", owner, g.ID, q.ID); e != nil {
		return g, e
	}
	return s.touch(ctx, tx, owner, fresh, "quest_linked", "Подтверждён новый шаг")
}
