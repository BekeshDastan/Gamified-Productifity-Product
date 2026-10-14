"use client";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { usePathname } from "next/navigation";
import { useRef, useState, type ReactNode, type MouseEvent } from "react";
import {
  House,
  CalendarDays,
  ListTodo,
  Menu,
  Plus,
  ArrowUpRight,
  UserRound,
  Sparkles,
  Headphones,
} from "lucide-react";
import {
  allNavigation,
  navigationGroups,
  utilityNavigation,
  settingNavigation,
  routeIsActive,
} from "@/lib/navigation";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { AccountThemeSelect, PersonalizationEffects } from "./personalization";
import { useAuth } from "@/lib/auth/provider";
import { AccountQueryProvider } from "@/lib/api/query-provider";
import { TaskViewObserver } from "./task-view-observer";
import { NotificationBell } from "./notifications";
import { QuickTask } from "./quick-task";
import { LoadingState, UnauthorizedState } from "./ui/states";

export function AppShell({ children }: { children: ReactNode }) {
  const auth = useAuth();
  return (
    <AccountQueryProvider accountId={auth.session?.account.id ?? null}>
      <ShellContent key={auth.session?.account.id ?? "guest"}>{children}</ShellContent>
    </AccountQueryProvider>
  );
}
function ShellContent({ children }: { children: ReactNode }) {
  const auth = useAuth(),
    pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false),
    [adding, setAdding] = useState(false),
    [quickState, setQuickState] = useState({ dirty: false, saving: false });
  const menuTrigger = useRef<HTMLButtonElement | null>(null),
    addTrigger = useRef<HTMLButtonElement | null>(null);
  const current = allNavigation.find((item) => routeIsActive(pathname, item.href));
  const SectionIcon = current?.icon ?? House;
  function navItems(items: typeof utilityNavigation) {
    return items.map(({ href, label, icon: Icon }) => (
      <Link
        key={href}
        className="nav-item"
        href={href}
        aria-current={routeIsActive(pathname, href) ? "page" : undefined}
        onNavigate={() => setMenuOpen(false)}
      >
        <Icon size={18} aria-hidden="true" />
        {label}
      </Link>
    ));
  }
  function navigation() {
    return (
      <nav aria-label="Разделы приложения">
        {navigationGroups.map((group, index) =>
          index < 2 ? (
            <div className="nav-group" key={group.label}>
              <p className="nav-caption">{group.label}</p>
              {navItems(group.items)}
            </div>
          ) : (
            <details
              className="nav-group nav-disclosure"
              key={group.label + pathname}
              open={group.items.some((item) => routeIsActive(pathname, item.href))}
            >
              <summary>{group.label}</summary>
              {navItems(group.items)}
            </details>
          ),
        )}
        <div className="nav-utilities">{navItems(utilityNavigation)}</div>
      </nav>
    );
  }
  function openAdd(event: MouseEvent<HTMLButtonElement>) {
    addTrigger.current = event.currentTarget;
    setQuickState({ dirty: false, saving: false });
    setAdding(true);
  }
  return (
    <>
      <PersonalizationEffects />
      <TaskViewObserver />
      <div className="app-shell" data-section={pathname.split("/")[2] ?? "today"}>
        <a href="#main-content" className="skip-link">
          К содержимому
        </a>
        <aside className="app-sidebar">
          <Link className="wordmark" href="/app" aria-label="ASCEND, мой день">
            <ArrowUpRight aria-hidden="true" />
            ASCEND
          </Link>
          <Button className="sidebar-add" onClick={openAdd}>
            <Plus size={18} aria-hidden="true" />
            Добавить задачу
          </Button>
          {navigation()}
          <Link href="/app/focus" className="sidebar-focus">
            <Headphones size={22} aria-hidden="true" />
            <span>
              <strong>Время для главного</strong>
              <small>Включите свой ритм</small>
            </span>
            <ArrowUpRight size={16} aria-hidden="true" />
          </Link>
          <p className="sidebar-note">Маленькие шаги. Большие перемены.</p>
        </aside>
        <div className="app-workspace">
          <header className="app-header">
            <div className="header-title">
              <span className="mobile-only">
                <Dialog
                  title="Разделы"
                  description="Ежедневные дела, цели и ваш прогресс."
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
              <span className="header-breadcrumb">
                <Sparkles size={15} aria-hidden="true" />
                <span>Моё пространство</span>
                <b>/</b>
                {current?.label ?? "ASCEND"}
              </span>
            </div>
            <AccountThemeSelect />
            <NotificationBell />
            <Link
              className="account-link"
              href="/app/settings/security"
              aria-label={
                auth.session ? `Аккаунт: ${auth.session.account.email}` : "Аккаунт и вход"
              }
            >
              <UserRound size={19} aria-hidden="true" />
              <span>Аккаунт</span>
            </Link>
          </header>
          <main id="main-content" tabIndex={-1} className="app-main">
            <div className="section-emblem" aria-hidden="true">
              <SectionIcon size={36} strokeWidth={1.5} />
            </div>
            {pathname.startsWith("/app/settings/") && (
              <nav className="settings-nav" aria-label="Настройки аккаунта">
                <Link href="/app/settings" className="text-link">
                  Все настройки
                </Link>
                {settingNavigation
                  .filter((item) => item.href.startsWith("/app/settings/"))
                  .map((item) => (
                    <Link
                      key={item.href}
                      href={item.href}
                      aria-current={pathname === item.href ? "page" : undefined}
                    >
                      {item.label}
                    </Link>
                  ))}
              </nav>
            )}
            {pathname !== "/app" &&
            pathname !== "/app/guide" &&
            !pathname.startsWith("/app/settings") &&
            pathname !== "/app/onboarding" &&
            !auth.session ? (
              auth.session === undefined ? (
                <LoadingState label="Проверяем вход…" />
              ) : (
                <UnauthorizedState />
              )
            ) : (
              children
            )}
          </main>
          <nav className="bottom-nav" aria-label="Быстрая навигация">
            <Link href="/app" aria-current={pathname === "/app" ? "page" : undefined}>
              <House size={21} aria-hidden="true" />
              Сегодня
            </Link>
            <Link
              href="/app/quests"
              aria-current={routeIsActive(pathname, "/app/quests") ? "page" : undefined}
            >
              <ListTodo size={21} aria-hidden="true" />
              Задачи
            </Link>
            <Button variant="ghost" aria-label="Добавить задачу" onClick={openAdd}>
              <Plus size={23} aria-hidden="true" />
              Добавить
            </Button>
            <Link
              href="/app/calendar"
              aria-current={pathname === "/app/calendar" ? "page" : undefined}
            >
              <CalendarDays size={21} aria-hidden="true" />
              Календарь
            </Link>
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
      <Dialog
        title="Добавить задачу"
        description="Запишите один конкретный шаг. Он появится в вашем сегодняшнем списке."
        open={adding}
        onOpenChange={(open) => {
          if (open) setAdding(true);
          else if (
            !quickState.saving &&
            (!quickState.dirty ||
              window.confirm("Задача ещё не сохранена. Закрыть и потерять введённый текст?"))
          )
            setAdding(false);
        }}
        onCloseAutoFocus={(event) => {
          if (addTrigger.current) {
            event.preventDefault();
            addTrigger.current.focus();
          }
        }}
      >
        <QuickTask onStateChange={setQuickState} />
      </Dialog>
    </>
  );
}
