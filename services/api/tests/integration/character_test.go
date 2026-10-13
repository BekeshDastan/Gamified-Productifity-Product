//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"testing"
)

func readCharacter(t *testing.T, r authResponse) character.Profile {
	t.Helper()
	var out struct {
		Data character.Profile `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("character %d %s", r.status, r.body)
	}
	return out.Data
}
func characterVersion(p character.Profile) string { return fmt.Sprintf(`"%d"`, p.Version) }
func TestCharacterInventoryOwnershipSlotsAndPersistence(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "character-a@example.test")
	b, _ := f.signedIn(t, "character-b@example.test")
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	owned := 0
	for _, i := range p.Inventory {
		if i.Owned {
			owned++
		}
	}
	if owned != 6 || len(p.Equipment) != 6 || len(p.Inventory) != 20 {
		t.Fatal(p)
	}
	input := map[string]string{"slot": "clothes", "item_id": "clothes-1"}
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 409 {
		t.Fatal("unearned", r.status)
	}
	input["item_id"] = "head-0"
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 422 {
		t.Fatal("slot", r.status)
	}
	qInput := questInput("Открыть одежду")
	qInput.Difficulty = 5
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qInput))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}))
	p = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	input["item_id"] = "clothes-1"
	key := "character-equip-once-0001"
	v := characterVersion(p)
	p = readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/equipment", v, input, key))
	again := readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/equipment", v, input, key))
	if p.Equipment["clothes"] != "clothes-1" || p.Version != again.Version {
		t.Fatal(p, again)
	}
	other := readCharacter(t, f.privateRequest(t, b, "GET", "/character", "", nil))
	if r := f.privateRequest(t, b, "PATCH", "/character/equipment", characterVersion(other), input); r.status != 409 {
		t.Fatal("borrowed another owner's inventory", r.status)
	}
	next := authClient(t)
	f.loginAs(t, next, "character-a@example.test")
	fresh := readCharacter(t, f.privateRequest(t, next, "GET", "/character", "", nil))
	if fresh.Equipment["clothes"] != "clothes-1" {
		t.Fatal("session persistence")
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), map[string]string{"reason": "Исправление"}))
	fresh = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	if fresh.Equipment["clothes"] != "clothes-0" {
		t.Fatal("revoked item remains equipped", fresh)
	}
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 409 {
		t.Fatal("stale version", r.status)
	}
}
