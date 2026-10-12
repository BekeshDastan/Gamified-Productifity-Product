"use client";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { usePathname } from "next/navigation";
import { useRef, useState, type ReactNode } from "react";
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
  Users,
  Settings,
  Bell,
  Menu,
  Plus,
  ArrowUpRight,
  Compass,
  LockKeyhole,
} from "lucide-react";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { ThemeSelect } from "./theme-select";
import { useAuth } from "@/lib/auth/provider";
import { AccountQueryProvider } from "@/lib/api/query-provider";

const planned = [
  { label: "Календарь", icon: CalendarDays },
  { label: "Задачи", icon: ListTodo },
  { label: "Цели", icon: Flag },
  { label: "Привычки", icon: Repeat2 },
  { label: "Фокус", icon: Timer },
  { label: "Испытания", icon: Trophy },
  { label: "Сезоны", icon: Sprout },
  { label: "Персонаж", icon: UserRound },
  { label: "Достижения и награды", icon: Trophy },
  { label: "Аналитика", icon: ChartNoAxesCombined },
  { label: "Дневник", icon: NotebookPen },
  { label: "Друзья", icon: Users },
  { label: "Уведомления", icon: Bell },
];
const available = [
  { href: "/app/onboarding", label: "Первые шаги", icon: Compass },
  { href: "/app/settings/areas", label: "Направления и теги", icon: Sprout },
  { href: "/app/settings/schedule", label: "Расписание", icon: CalendarDays },
  { href: "/app/settings/profile", label: "Профиль", icon: UserRound },
  { href: "/app", label: "Сегодня", icon: House },
  { href: "/app/guide", label: "О приложении", icon: Compass },
  { href: "/app/settings/appearance", label: "Оформление", icon: Settings },
];

export function AppShell({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuTrigger = useRef<HTMLButtonElement | null>(null);
  function navigation() {
    return (
      <nav aria-label="Разделы приложения">
        <p className="nav-caption">ВАШЕ ПРОСТРАНСТВО</p>
        {available.map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            className="nav-item"
            href={href}
            aria-current={pathname === href ? "page" : undefined}
            onNavigate={() => setMenuOpen(false)}
          >
            <Icon size={19} aria-hidden="true" />
            {label}
          </Link>
        ))}
        <p className="nav-caption">В РАЗРАБОТКЕ</p>
        {planned.map(({ label, icon: Icon }) => (
          <span key={label} className="nav-item nav-planned" aria-disabled="true">
            <Icon size={18} aria-hidden="true" />
            <span>{label}</span>
            <span className="nav-soon">Скоро</span>
          </span>
        ))}
      </nav>
    );
  }
  return (
    <AccountQueryProvider accountId={auth.session?.account.id ?? null}>
      <div className="app-shell">
        <a href="#main-content" className="skip-link">
          К содержимому
        </a>
        <aside className="app-sidebar">
          <Link className="wordmark" href="/" aria-label="ASCEND, главная">
            <ArrowUpRight aria-hidden="true" />
            ASCEND
          </Link>
          {navigation()}
          <div className="sidebar-note">
            <LockKeyhole size={17} aria-hidden="true" />
            <span>
              Приватно по умолчанию.
              <br />В вашем ритме.
            </span>
          </div>
        </aside>
        <div className="app-workspace">
          <header className="app-header">
            <div className="header-title">
              <span className="mobile-only">
                <Dialog
                  title="Разделы"
                  description="Личный кабинет находится в разработке."
                  open={menuOpen}
                  onOpenChange={setMenuOpen}
                  onCloseAutoFocus={(event) => {
                    if (menuTrigger.current) {
                      event.preventDefault();
                      menuTrigger.current.focus();
                    }
                  }}
                  trigger={
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Открыть меню"
                      onClick={(event) => {
                        menuTrigger.current = event.currentTarget;
                      }}
                    >
                      <Menu size={21} aria-hidden="true" />
                    </Button>
                  }
                  className="mobile-drawer"
                >
                  {navigation()}
                </Dialog>
              </span>
              <span>{available.find((item) => item.href === pathname)?.label ?? "ASCEND"}</span>
            </div>
            <ThemeSelect />
            <span className="account-state">
              <LockKeyhole size={14} aria-hidden="true" />
              {auth.session
                ? "Личный аккаунт"
                : auth.session === undefined
                  ? "Проверяем вход…"
                  : "Гостевой режим"}
            </span>
          </header>
          <main id="main-content" tabIndex={-1} className="app-main">
            {children}
          </main>
          <nav className="bottom-nav" aria-label="Быстрая навигация">
            <Link href="/app" aria-current={pathname === "/app" ? "page" : undefined}>
              <House size={21} aria-hidden="true" />
              Сегодня
            </Link>
            <span aria-disabled="true">
              <CalendarDays size={21} aria-hidden="true" />
              Календарь<span className="sr-only"> — скоро</span>
            </span>
            <span aria-disabled="true">
              <Plus size={23} aria-hidden="true" />
              Добавить<span className="sr-only"> — скоро</span>
            </span>
            <span aria-disabled="true">
              <ChartNoAxesCombined size={21} aria-hidden="true" />
              Прогресс<span className="sr-only"> — скоро</span>
            </span>
            <Button
              variant="ghost"
              aria-label="Ещё — открыть меню"
              onClick={(event) => {
                menuTrigger.current = event.currentTarget;
                setMenuOpen(true);
              }}
            >
              <Menu size={21} aria-hidden="true" />
              Ещё
            </Button>
          </nav>
        </div>
      </div>
    </AccountQueryProvider>
  );
}
