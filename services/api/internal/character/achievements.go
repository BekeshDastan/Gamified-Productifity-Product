package character

// Conditions are typed, versioned data. No SQL fragments or executable expressions are accepted.
type Metric string

const (
	Tasks             Metric = "tasks"
	GoalsCreated      Metric = "goals_created"
	GoalsAchieved     Metric = "goals_achieved"
	HabitsCreated     Metric = "habits_created"
	SuccessfulPeriods Metric = "successful_periods"
	BestStreak        Metric = "best_streak"
	Comebacks         Metric = "comebacks"
	FocusSessions     Metric = "focus_sessions"
	ConfirmedPlans    Metric = "confirmed_plans"
	AreasCreated      Metric = "areas_created"
	PlayerLevel       Metric = "level"
	CompletedSeasons  Metric = "completed_seasons"
	CoopChallenges    Metric = "coop_challenges"
	ImportantTasks    Metric = "important_tasks"
)
const AchievementVersion = "achievements-v1"

type Definition struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Metric      Metric `json:"metric"`
	Threshold   int64  `json:"threshold"`
	SourceURL   string `json:"source_url"`
}
type Facts struct {
	Values    map[Metric]int64
	Available map[Metric]bool
}

func (d Definition) Met(f Facts) bool {
	return f.Available[d.Metric] && f.Values[d.Metric] >= d.Threshold
}
func Definitions() []Definition {
	return []Definition{
		{ID: "task-1", Version: AchievementVersion, Name: "Первый шаг", Description: "Выполнить одну самостоятельную задачу", Metric: Metric("tasks"), Threshold: 1, SourceURL: "/app/quests"},
		{ID: "task-10", Version: AchievementVersion, Name: "Десять шагов", Description: "Выполнить 10 самостоятельных задач", Metric: Metric("tasks"), Threshold: 10, SourceURL: "/app/quests"},
		{ID: "task-25", Version: AchievementVersion, Name: "Точка опоры", Description: "Выполнить 25 самостоятельных задач", Metric: Metric("tasks"), Threshold: 25, SourceURL: "/app/quests"},
		{ID: "task-50", Version: AchievementVersion, Name: "Пятьдесят шагов", Description: "Выполнить 50 самостоятельных задач", Metric: Metric("tasks"), Threshold: 50, SourceURL: "/app/quests"},
		{ID: "task-100", Version: AchievementVersion, Name: "Сотня шагов", Description: "Выполнить 100 самостоятельных задач", Metric: Metric("tasks"), Threshold: 100, SourceURL: "/app/quests"},
		{ID: "goal-created-1", Version: AchievementVersion, Name: "Намерение", Description: "Создать первую цель", Metric: Metric("goals_created"), Threshold: 1, SourceURL: "/app/goals"},
		{ID: "goal-created-3", Version: AchievementVersion, Name: "Три намерения", Description: "Создать три цели", Metric: Metric("goals_created"), Threshold: 3, SourceURL: "/app/goals"},
		{ID: "goal-achieved-1", Version: AchievementVersion, Name: "Первая вершина", Description: "Подтвердить достижение одной цели", Metric: Metric("goals_achieved"), Threshold: 1, SourceURL: "/app/goals"},
		{ID: "goal-achieved-3", Version: AchievementVersion, Name: "Три вершины", Description: "Подтвердить достижение трёх целей", Metric: Metric("goals_achieved"), Threshold: 3, SourceURL: "/app/goals"},
		{ID: "habit-created-1", Version: AchievementVersion, Name: "Начало ритма", Description: "Создать одну привычку с нормой", Metric: Metric("habits_created"), Threshold: 1, SourceURL: "/app/habits"},
		{ID: "habit-created-3", Version: AchievementVersion, Name: "Свои ритмы", Description: "Создать три привычки с нормами", Metric: Metric("habits_created"), Threshold: 3, SourceURL: "/app/habits"},
		{ID: "period-7", Version: AchievementVersion, Name: "Семь норм", Description: "Полностью выполнить 7 запланированных норм привычек", Metric: Metric("successful_periods"), Threshold: 7, SourceURL: "/app/habits"},
		{ID: "period-30", Version: AchievementVersion, Name: "Тридцать норм", Description: "Полностью выполнить 30 запланированных норм привычек", Metric: Metric("successful_periods"), Threshold: 30, SourceURL: "/app/habits"},
		{ID: "period-100", Version: AchievementVersion, Name: "Сто норм", Description: "Полностью выполнить 100 запланированных норм привычек", Metric: Metric("successful_periods"), Threshold: 100, SourceURL: "/app/habits"},
		{ID: "streak-7", Version: AchievementVersion, Name: "Устойчивость", Description: "Серия 7 полных запланированных норм одной привычки", Metric: Metric("best_streak"), Threshold: 7, SourceURL: "/app/habits"},
		{ID: "streak-30", Version: AchievementVersion, Name: "Длинный ритм", Description: "Серия 30 полных запланированных норм одной привычки", Metric: Metric("best_streak"), Threshold: 30, SourceURL: "/app/habits"},
		{ID: "comeback-1", Version: AchievementVersion, Name: "Возвращение", Description: "Выполнить норму привычки после закрытого пропуска", Metric: Metric("comebacks"), Threshold: 1, SourceURL: "/app/habits"},
		{ID: "focus-10", Version: AchievementVersion, Name: "Десять фокусов", Description: "Подтвердить 10 фокус-сессий с результатом и не менее 5 минут работы каждая", Metric: Metric("focus_sessions"), Threshold: 10, SourceURL: "/app/focus"},
		{ID: "focus-50", Version: AchievementVersion, Name: "Пятьдесят фокусов", Description: "Подтвердить 50 фокус-сессий с результатом и не менее 5 минут работы каждая", Metric: Metric("focus_sessions"), Threshold: 50, SourceURL: "/app/focus"},
		{ID: "plan-1", Version: AchievementVersion, Name: "Свой день", Description: "Подтвердить план одного дня", Metric: Metric("confirmed_plans"), Threshold: 1, SourceURL: "/app/planning"},
		{ID: "plan-7", Version: AchievementVersion, Name: "Семь планов", Description: "Подтвердить планы 7 разных дней", Metric: Metric("confirmed_plans"), Threshold: 7, SourceURL: "/app/planning"},
		{ID: "plan-30", Version: AchievementVersion, Name: "Тридцать планов", Description: "Подтвердить планы 30 разных дней", Metric: Metric("confirmed_plans"), Threshold: 30, SourceURL: "/app/planning"},
		{ID: "area-1", Version: AchievementVersion, Name: "Направление выбрано", Description: "Создать одно направление жизни", Metric: Metric("areas_created"), Threshold: 1, SourceURL: "/app/settings/areas"},
		{ID: "area-3", Version: AchievementVersion, Name: "Три направления", Description: "Создать три направления жизни", Metric: Metric("areas_created"), Threshold: 3, SourceURL: "/app/settings/areas"},
		{ID: "level-2", Version: AchievementVersion, Name: "Второй уровень", Description: "Накопить 100 допустимого XP", Metric: Metric("level"), Threshold: 2, SourceURL: "/app/progress"},
		{ID: "level-3", Version: AchievementVersion, Name: "Третий уровень", Description: "Накопить 400 допустимого XP", Metric: Metric("level"), Threshold: 3, SourceURL: "/app/progress"},
		{ID: "level-5", Version: AchievementVersion, Name: "Пятый уровень", Description: "Накопить 1600 допустимого XP", Metric: Metric("level"), Threshold: 5, SourceURL: "/app/progress"},
		{ID: "season-1", Version: AchievementVersion, Name: "Сезон завершён", Description: "Завершить один подтверждённый сезон", Metric: Metric("completed_seasons"), Threshold: 1, SourceURL: "/app/seasons"},
		{ID: "coop-1", Version: AchievementVersion, Name: "Вместе", Description: "Завершить одно совместное испытание", Metric: Metric("coop_challenges"), Threshold: 1, SourceURL: "/app/friends"},
		{ID: "important-10", Version: AchievementVersion, Name: "Важное сделано", Description: "Выполнить 10 самостоятельных задач высокого приоритета", Metric: Metric("important_tasks"), Threshold: 10, SourceURL: "/app/quests"},
	}
}
