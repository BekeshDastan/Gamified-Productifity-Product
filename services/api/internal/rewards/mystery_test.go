package rewards

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"testing"
)

func TestWeightedBoundaries(t *testing.T) {
	items := []character.WeightedItem{{Item: character.Item{ID: "a"}, Weight: 3}, {Item: character.Item{ID: "b"}, Weight: 1}}
	for n, want := range []string{"a", "a", "a", "b"} {
		got, e := ChooseWeighted(items, int64(n))
		if e != nil || got != want {
			t.Fatal(n, got, e)
		}
	}
	for _, n := range []int64{-1, 4} {
		if _, e := ChooseWeighted(items, n); e == nil {
			t.Fatal("out of range", n)
		}
	}
	if _, e := ChooseWeighted([]character.WeightedItem{{Weight: 0}}, 0); e == nil {
		t.Fatal("invalid weight")
	}
}
