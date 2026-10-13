package character

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type Service struct {
	pool     *pgxpool.Pool
	clock    platform.Clock
	progress *progression.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock, p *progression.Service) *Service {
	return &Service{pool, clock, p}
}

type InventoryItem struct {
	Item
	Owned   bool   `json:"owned"`
	Source  string `json:"source"`
	Revoked bool   `json:"revoked"`
}
type Profile struct {
	Recalculating  bool              `json:"recalculating"`
	Title          string            `json:"title"`
	Titles         []Title           `json:"titles"`
	Records        []Record          `json:"records"`
	Areas          []Area            `json:"areas"`
	Version        int64             `json:"version"`
	CatalogVersion string            `json:"catalog_version"`
	Equipment      map[string]string `json:"equipment"`
	TitleID        string            `json:"title_id"`
	Level          progression.Level `json:"level"`
	Inventory      []InventoryItem   `json:"inventory"`
}

func apiError(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func currentOwner(ctx context.Context) (string, error) {
	i, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", apiError(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return i.UserID, nil
}
func (s *Service) ensure(ctx context.Context, tx pgx.Tx, owner string) error {
	for _, i := range Catalog() {
		b, e := json.Marshal(i)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO cosmetic_items(id,slot,catalog_version,snapshot) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING", i.ID, i.Slot, CatalogVersion, b); e != nil {
			return e
		}
	}
	b, _ := json.Marshal(starterEquipment())
	_, e := tx.Exec(ctx, "INSERT INTO character_profiles(owner_id,equipment,updated_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", owner, b, s.clock.Now())
	return e
}
func (s *Service) achievementActive(ctx context.Context, tx pgx.Tx, owner, id string) (bool, error) {
	var active bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM user_achievements WHERE owner_id=$1 AND definition_id=$2 AND definition_version=$3 AND active)", owner, id, AchievementVersion).Scan(&active)
	return active, e
}
func (s *Service) EvaluateInTx(ctx context.Context, tx pgx.Tx, owner string) error {
	if e := s.ensure(ctx, tx, owner); e != nil {
		return e
	}
	if _, e := s.progress.RebuildInTx(ctx, tx, owner); e != nil {
		return e
	}
	achievementsChanged, e := s.evaluateAchievements(ctx, tx, owner)
	if e != nil {
		return e
	}
	balance, e := s.progress.Balance(ctx, tx, owner)
	if e != nil {
		return e
	}
	var equipment map[string]string
	var titleID string
	var b []byte
	if e = tx.QueryRow(ctx, "SELECT equipment,title_id FROM character_profiles WHERE owner_id=$1 FOR UPDATE", owner).Scan(&b, &titleID); e != nil {
		return e
	}
	if e = json.Unmarshal(b, &equipment); e != nil {
		return e
	}
	changed := achievementsChanged
	titles, e := s.titles(ctx, tx, owner, balance.Level.Number)
	if e != nil {
		return e
	}
	if !entitledTitle(titleID, titles) {
		titleID = "level:auto"
		changed = true
	}
	for _, i := range Catalog() {
		active := i.Starter || balance.Level.Number >= i.Level
		source := "level"
		if i.Starter {
			source = "starter"
		}
		if i.Achievement != "" {
			source = "achievement:" + i.Achievement
			active, e = s.achievementActive(ctx, tx, owner, i.Achievement)
			if e != nil {
				return e
			}
		}
		var permanent bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cosmetic_permanent_grants WHERE owner_id=$1 AND item_id=$2 AND active)", owner, i.ID).Scan(&permanent); e != nil {
			return e
		}
		if permanent {
			active = true
			source = "permanent"
		}
		var previous bool
		e = tx.QueryRow(ctx, "SELECT active FROM character_inventory WHERE owner_id=$1 AND item_id=$2", owner, i.ID).Scan(&previous)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		exists := e == nil
		e = nil
		if (!exists && active) || (exists && previous != active) {
			_, e = tx.Exec(ctx, "INSERT INTO character_inventory(owner_id,item_id,active,source,granted_at,revoked_at) VALUES($1,$2,$3,$4,$5,CASE WHEN $3 THEN NULL ELSE $5::timestamptz END) ON CONFLICT(owner_id,item_id) DO UPDATE SET active=EXCLUDED.active,revoked_at=EXCLUDED.revoked_at", owner, i.ID, active, source, s.clock.Now())
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, "INSERT INTO character_inventory_events(owner_id,item_id,active,reason,created_at) VALUES($1,$2,$3,$4,$5)", owner, i.ID, active, source, s.clock.Now())
			if e != nil {
				return e
			}
			changed = true
		}
		if !active && equipment[i.Slot] == i.ID {
			equipment[i.Slot] = starterEquipment()[i.Slot]
			changed = true
		}
	}
	if changed {
		b, e = json.Marshal(equipment)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE character_profiles SET equipment=$2,title_id=$4,version=version+1,updated_at=$3 WHERE owner_id=$1", owner, b, s.clock.Now(), titleID)
	}
	return e
}
func (s *Service) view(ctx context.Context, tx pgx.Tx, owner string) (Profile, error) {
	out := Profile{CatalogVersion: CatalogVersion, Inventory: []InventoryItem{}}
	if e := s.EvaluateInTx(ctx, tx, owner); e != nil {
		return out, e
	}
	b, e := s.progress.Balance(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.Level = b.Level
	out.Areas, e = s.areas(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	var data []byte
	e = tx.QueryRow(ctx, "SELECT version,equipment,title_id FROM character_profiles WHERE owner_id=$1", owner).Scan(&out.Version, &data, &out.TitleID)
	if e != nil {
		return out, e
	}
	out.Titles, e = s.titles(ctx, tx, owner, out.Level.Number)
	if e != nil {
		return out, e
	}
	for _, title := range out.Titles {
		if title.ID == out.TitleID {
			out.Title = title.Name
		}
	}
	out.Records, e = s.records(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	for _, record := range out.Records {
		if record.ID == "best-streak" {
			out.Recalculating = !record.Available
		}
	}
	if e = json.Unmarshal(data, &out.Equipment); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT item_id,active,source FROM character_inventory WHERE owner_id=$1", owner)
	if e != nil {
		return out, e
	}
	owned := map[string]InventoryItem{}
	for rows.Next() {
		var id, source string
		var active bool
		if e = rows.Scan(&id, &active, &source); e != nil {
			rows.Close()
			return out, e
		}
		owned[id] = InventoryItem{Owned: active, Revoked: !active, Source: source}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, i := range Catalog() {
		entry := owned[i.ID]
		entry.Item = i
		out.Inventory = append(out.Inventory, entry)
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, owner string) (Profile, error) {
	var out Profile
	e := s.progress.WithinOwner(ctx, owner, func(tx pgx.Tx) error { var e error; out, e = s.view(ctx, tx, owner); return e })
	return out, e
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = apiError(500, "INTERNAL_ERROR", "Не удалось обновить персонажа")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, out)
}
func (s *Service) mutate(r *http.Request, in any, fn func(context.Context, pgx.Tx, string, Profile) error) (any, error) {
	owner, e := currentOwner(r.Context())
	if e != nil {
		return nil, e
	}
	data, e := json.Marshal(struct {
		Input   any
		Version string
	}{in, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	out, e := (storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}).Execute(r.Context(), storage.Scope{UserID: owner, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, data, func(tx pgx.Tx) (storage.Result, error) {
		ctx := r.Context()
		if e := s.progress.LockOwner(ctx, tx, owner); e != nil {
			return storage.Result{}, e
		}
		p, e := s.view(ctx, tx, owner)
		if e != nil {
			return storage.Result{}, e
		}
		if conflict := platform.MatchVersion(r, p.Version); conflict != nil {
			return storage.Result{}, conflict
		}
		if e = fn(ctx, tx, owner, p); e != nil {
			return storage.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE character_profiles SET version=version+1,updated_at=$2 WHERE owner_id=$1", owner, s.clock.Now()); e != nil {
			return storage.Result{}, e
		}
		p, e = s.view(ctx, tx, owner)
		if e != nil {
			return storage.Result{}, e
		}
		b, e := json.Marshal(p)
		return storage.Result{Status: 200, Body: b}, e
	})
	if errors.Is(e, storage.ErrInvalidScope) {
		e = apiError(422, "VALIDATION_ERROR", "Нужен ключ запроса")
	}
	if errors.Is(e, storage.ErrIdempotencyMismatch) {
		e = apiError(409, "IDEMPOTENCY_MISMATCH", "Ключ уже использован")
	}
	if errors.Is(e, storage.ErrOperationInProgress) {
		e = apiError(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	return json.RawMessage(out.Body), e
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	MountAssets(router)
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountAchievements(r)
		s.mountTitles(r)
		for _, path := range []string{"/api/v1/character", "/api/v1/character/inventory", "/api/v1/skill-tree"} {
			r.Get(path, func(w http.ResponseWriter, r *http.Request) {
				owner, e := currentOwner(r.Context())
				var p Profile
				if e == nil {
					p, e = s.Get(r.Context(), owner)
				}
				s.reply(w, r, p, e)
			})
		}
		r.Patch("/api/v1/character/equipment", func(w http.ResponseWriter, r *http.Request) {
			in, p := platform.DecodeJSON[struct {
				Slot   string `json:"slot"`
				ItemID string `json:"item_id"`
			}](r)
			if p != nil {
				platform.WriteError(w, r, p)
				return
			}
			out, e := s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string, p Profile) error {
				item, ok := itemByID(in.ItemID)
				if !ok || !validSlot(in.Slot) || item.Slot != in.Slot {
					return apiError(422, "VALIDATION_ERROR", "Предмет не подходит этому слоту")
				}
				owned := false
				for _, i := range p.Inventory {
					if i.ID == in.ItemID {
						owned = i.Owned
					}
				}
				if !owned {
					return apiError(409, "ITEM_LOCKED", "Этот предмет ещё не открыт")
				}
				p.Equipment[in.Slot] = in.ItemID
				b, e := json.Marshal(p.Equipment)
				if e != nil {
					return e
				}
				_, e = tx.Exec(ctx, "UPDATE character_profiles SET equipment=$2 WHERE owner_id=$1", owner, b)
				return e
			})
			s.reply(w, r, out, e)
		})
	})
}
