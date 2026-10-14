package analytics

import (
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"time"
)

const ReportVersion = "report-v1"

type ReportInput struct {
	Kind     string `json:"kind"`
	StartsOn string `json:"starts_on"`
	Timezone string `json:"timezone"`
}
type Comparison struct {
	Metric          string   `json:"metric"`
	Unit            string   `json:"unit"`
	Current         float64  `json:"current"`
	CurrentObserved bool     `json:"current_observed"`
	Previous        *float64 `json:"previous"`
	Delta           *float64 `json:"delta"`
	Percent         *float64 `json:"percent"`
}

func reportRanges(in ReportInput) (Range, Range, error) {
	current := Range{}
	previous := Range{}
	if privateapi.Day(in.StartsOn) != nil {
		return current, previous, privateapi.Invalid("Укажите начало периода")
	}
	start, _ := time.Parse(time.DateOnly, in.StartsOn)
	var end time.Time
	switch in.Kind {
	case "week":
		if start.Weekday() != time.Monday {
			return current, previous, privateapi.Invalid("Неделя начинается в понедельник")
		}
		end = start.AddDate(0, 0, 7)
	case "month":
		if start.Day() != 1 {
			return current, previous, privateapi.Invalid("Месяц начинается с первого числа")
		}
		end = start.AddDate(0, 1, 0)
	default:
		return current, previous, privateapi.Invalid("Выберите неделю или месяц")
	}
	days := int(end.Sub(start) / (24 * time.Hour))
	from := start.AddDate(0, 0, -days)
	if from.Year() < 1 || end.Year() > 9998 {
		return current, previous, privateapi.Invalid("Период выходит за допустимые даты")
	}
	return Range{start.Format(time.DateOnly), end.Format(time.DateOnly)}, Range{from.Format(time.DateOnly), start.Format(time.DateOnly)}, nil
}
func compare(metric, unit string, current, previous float64, currentObserved, previousObserved bool) Comparison {
	out := Comparison{Metric: metric, Unit: unit, Current: current, CurrentObserved: currentObserved}
	if previousObserved {
		out.Previous = &previous
	}
	if currentObserved && previousObserved {
		delta := current - previous
		out.Delta = &delta
		if previous > 0 {
			percent := delta / previous * 100
			out.Percent = &percent
		}
	}
	return out
}
func reportComparisons(current, previous []Day, currentObserved, previousObserved bool) []Comparison {
	sum := func(days []Day) map[string]float64 {
		out := map[string]float64{}
		for _, d := range days {
			out["tasks"] += float64(d.Completed)
			out["habit_norms"] += float64(d.HabitSuccesses)
			out["focus"] += float64(d.FocusSeconds)
			out["goals"] += float64(d.GoalsAchieved)
			out["xp"] += float64(d.XPDelta)
			out["coins"] += float64(d.CoinsDelta)
		}
		return out
	}
	a, b := sum(current), sum(previous)
	out := []Comparison{}
	for _, v := range []struct{ metric, unit string }{{"tasks", "tasks"}, {"habit_norms", "periods"}, {"focus", "seconds"}, {"goals", "goals"}, {"xp", "xp"}, {"coins", "coins"}} {
		out = append(out, compare(v.metric, v.unit, a[v.metric], b[v.metric], currentObserved, previousObserved))
	}
	return out
}
func reportText(kind string, current, previous Range, items []Comparison, complete, previousComplete bool) string {
	label := "Недельный отчёт"
	if kind == "month" {
		label = "Месячный отчёт"
	}
	text := fmt.Sprintf("%s: %s — %s (конец не включён).", label, current.From, current.To)
	if !complete {
		text += " Наблюдения за периодом неполные: это только записанные результаты, без оценки всего периода."
	}
	for _, v := range items {
		if v.Metric == "tasks" {
			text += fmt.Sprintf(" Отмечено завершённых задач: %.0f.", v.Current)
		}
		if v.Metric == "habit_norms" {
			text += fmt.Sprintf(" Полных норм привычек: %.0f.", v.Current)
		}
		if v.Metric == "focus" {
			text += fmt.Sprintf(" Подтверждённая работа: %.2f минут.", v.Current/60)
		}
	}
	if previousComplete && complete {
		text += fmt.Sprintf(" Сравнение с равным по длине периодом %s — %s.", previous.From, previous.To)
	} else {
		text += " Для полного сравнения пока недостаточно наблюдений."
	}
	return text + " Ориентиры и исходные записи приведены отдельно. Это описание фактов, а не оценка человека."
}
