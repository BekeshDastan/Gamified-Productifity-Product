package progression

const Version = "economy-v1"
const MaxSafeXP int64 = 9007199254740991

func TaskXP(difficulty int) int64 {
	switch difficulty {
	case 1:
		return 5
	case 2:
		return 10
	case 3:
		return 25
	case 4:
		return 50
	case 5:
		return 100
	default:
		return 0
	}
}
func BaseXP(kind string, difficulty int) int64 {
	switch kind {
	case "task":
		return TaskXP(difficulty)
	case "habit":
		return 10
	case "challenge":
		return 25
	default:
		return 0
	}
}
func Quota(kind string) int64 {
	switch kind {
	case "task":
		return 200
	case "habit", "challenge":
		return 50
	default:
		return 0
	}
}

type Level struct {
	Number        int64 `json:"number"`
	Threshold     int64 `json:"threshold"`
	NextThreshold int64 `json:"next_threshold"`
	Inside        int64 `json:"inside"`
	ToNext        int64 `json:"to_next"`
}

func LevelFor(xp int64) Level {
	xp = max(0, min(MaxSafeXP, xp))
	n := xp / 100
	lo, hi := int64(0), int64(10000000)
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if mid == 0 || mid <= n/mid {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	threshold := 100 * lo * lo
	next := 100 * (lo + 1) * (lo + 1)
	return Level{Number: lo + 1, Threshold: threshold, NextThreshold: next, Inside: xp - threshold, ToNext: next - xp}
}
func Limited(base, kindUsed, totalUsed, kindCap int64) int64 {
	return max(0, min(base, kindCap-kindUsed, 300-totalUsed))
}

func CoinsForAward(xp int64) int64 { return max(0, xp) / 5 }
