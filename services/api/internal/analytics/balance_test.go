package analytics

import (
	"math"
	"testing"
)

func TestPersonalTargetsUnknownZeroAndClamping(t *testing.T) {
	for _, test := range []struct {
		actual, target *float64
		want           *float64
	}{{nil, balanceFloat(10), nil}, {balanceFloat(1), nil, nil}, {balanceFloat(1), balanceFloat(0), nil}, {balanceFloat(1), balanceFloat(-1), nil}, {balanceFloat(0), balanceFloat(10), balanceFloat(0)}, {balanceFloat(5), balanceFloat(10), balanceFloat(50)}, {balanceFloat(20), balanceFloat(10), balanceFloat(100)}, {balanceFloat(-2), balanceFloat(10), balanceFloat(0)}, {balanceFloat(math.NaN()), balanceFloat(1), nil}, {balanceFloat(1), balanceFloat(math.Inf(1)), nil}} {
		got := normalizeTarget(test.actual, test.target)
		if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
			t.Fatal(test, got)
		}
	}
}
func balanceFloat(v float64) *float64 { return &v }
