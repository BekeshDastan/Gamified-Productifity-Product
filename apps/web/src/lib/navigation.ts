import {
  House,
  CalendarDays,
  ListTodo,
  Flag,
  Repeat2,
  Timer,
  Trophy,
  Sprout,
  UserRound,
  ChartNoAxesCombined,
  NotebookPen,
  Settings,
  Bell,
  Compass,
  LockKeyhole,
  Palette,
} from "lucide-react";
export const navigationGroups = [
  {
    label: "На каждый день",
    items: [
      { href: "/app", label: "Сегодня", icon: House },
      { href: "/app/quests", label: "Задачи", icon: ListTodo },
      { href: "/app/habits", label: "Привычки", icon: Repeat2 },
      { href: "/app/calendar", label: "Календарь", icon: CalendarDays },
      { href: "/app/planning", label: "План дня и недели", icon: ListTodo },
      { href: "/app/focus", label: "Фокус", icon: Timer },
    ],
  },
  {
    label: "Цели и обзор",
    items: [
      { href: "/app/goals", label: "Цели", icon: Flag },
      { href: "/app/journal", label: "Дневник и ритуалы", icon: NotebookPen },
      { href: "/app/analytics", label: "Аналитика", icon: ChartNoAxesCombined },
      { href: "/app/navigator", label: "Навигатор", icon: Compass },
    ],
  },
  {
    label: "Игровой прогресс",
    items: [
      { href: "/app/progress", label: "Опыт и награды", icon: ChartNoAxesCombined },
      { href: "/app/character", label: "Персонаж и навыки", icon: UserRound },
      { href: "/app/achievements", label: "Достижения", icon: Trophy },
      { href: "/app/rewards", label: "Магазин и награды", icon: Trophy },
      { href: "/app/challenges", label: "Испытания", icon: Trophy },
      { href: "/app/bosses", label: "Боссы по целям", icon: Flag },
      { href: "/app/seasons", label: "Сезоны", icon: Sprout },
    ],
  },
];
export const utilityNavigation = [
  { href: "/app/notifications", label: "Уведомления", icon: Bell },
  { href: "/app/settings", label: "Настройки", icon: Settings },
  { href: "/app/guide", label: "Как пользоваться", icon: Compass },
];
export const settingNavigation = [
  {
    href: "/app/settings/profile",
    label: "Профиль",
    description: "Имя и образ в вашем пространстве.",
    icon: UserRound,
  },
  {
    href: "/app/settings/schedule",
    label: "Расписание",
    description: "Часовой пояс, рабочие часы и отдых.",
    icon: CalendarDays,
  },
  {
    href: "/app/settings/areas",
    label: "Направления и теги",
    description: "Учёба, работа и другие важные части жизни.",
    icon: Sprout,
  },
  {
    href: "/app/settings/appearance",
    label: "Оформление",
    description: "Тема, игровые элементы и виджеты дня.",
    icon: Palette,
  },
  {
    href: "/app/settings/notifications",
    label: "Каналы уведомлений",
    description: "Напоминания, тихие часы, почта и устройства.",
    icon: Bell,
  },
  {
    href: "/app/settings/privacy",
    label: "Приватность",
    description: "Кто сможет видеть вашу активность.",
    icon: LockKeyhole,
  },
  {
    href: "/app/settings/security",
    label: "Безопасность",
    description: "Почта, пароль, активные входы и выход из аккаунта.",
    icon: LockKeyhole,
  },
  {
    href: "/app/onboarding",
    label: "Первые шаги",
    description: "Короткое знакомство и необязательная настройка.",
    icon: Compass,
  },
];
export function routeIsActive(path: string, href: string) {
  return path === href || (href !== "/app" && path.startsWith(href + "/"));
}
export const allNavigation = [
  ...navigationGroups.flatMap((group) => group.items),
  ...settingNavigation,
  ...utilityNavigation,
];
