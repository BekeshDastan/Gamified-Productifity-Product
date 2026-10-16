package social

import "testing"

func TestRanksPreserveTiesAndInputOrderDoesNotMatter(t *testing.T) {
	rows := []Standing{{UserID: "b", Amount: 10}, {UserID: "d", Amount: 4}, {UserID: "a", Amount: 10}, {UserID: "c", Amount: 4}}
	rankStandings(rows)
	for i, want := range []int{1, 1, 3, 3} {
		if rows[i].Rank != want {
			t.Fatal(rows)
		}
	}
	if rows[0].UserID != "a" {
		t.Fatal(rows)
	}
}
func TestRoomRulesRejectIncomparableAndImpossibleTargets(t *testing.T) {
	in := RoomAction{Name: "Race", Mode: "pvp", Metric: "tasks", Unit: "tasks", Days: 7, DailyCap: 10, Target: 20}
	if !validRoom(in) {
		t.Fatal(in)
	}
	in.Unit = "xp"
	if validRoom(in) {
		t.Fatal("arbitrary XP comparison")
	}
	in.Unit = "tasks"
	in.Target = 71
	if validRoom(in) {
		t.Fatal("unreachable target")
	}
}
