"use client";
import { useRef, useState } from "react";
import { Check, Star, ArrowUpRight, Sparkles, CircleCheck, CalendarDays } from "lucide-react";
import { QuickTask } from "./quick-task";
import { StartHint } from "./onboarding";
import { AccountPanel } from "./account-panel";
import { ErrorState, LoadingState } from "./ui/states";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { getDashboard } from "@/lib/dashboard";
import { actOnQuest, type Quest } from "@/lib/quests";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { FocusSummary } from "./focus";
import { DashboardHabits } from "./habits";
import { MonthlyBossSummary } from "./bosses";
import { ProgressionSummary, useRewardNotice } from "./progression";
export function Dashboard() {
  const auth = useAuth(),
    settings = useSettings(),
    cache = useQueryClient(),
    id = auth.session?.account.id;
  const [notice, setNotice] = useState("");
  const data = useQuery({
    queryKey: ["account", id, "dashboard"],
    enabled: !!id,
    queryFn: ({ signal }) => getDashboard(signal),
    refetchInterval: 30000,
  });
  if (auth.session === undefined) return <LoadingState label="Готовим ваш день…" />;
  if (!id)
    return (
      <>
        <h1 className="page-title">Меньше держать в голове. Больше делать.</h1>
        <p className="page-intro">
          Соберите задачи на сегодня, отмечайте привычки и замечайте свой прогресс.
        </p>
        <AccountPanel />
      </>
    );
  if ((data.isError || settings.isError) && (!data.data || !settings.data))
    return (
      <ErrorState
        error={data.error ?? settings.error}
        retry={() => {
          void data.refetch();
          void settings.refetch();
        }}
      />
    );
  if (!data.data || !settings.data) return <LoadingState label="Загружаем ваш день…" />;
  const d = data.data,
    p = settings.data;
  const changed = () => cache.invalidateQueries({ queryKey: ["account", id] });
  const time = (v: string) =>
    DateTime.fromISO(v)
      .setZone(d.timezone)
      .setLocale(p.language)
      .toLocaleString({
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
        hour12: p.time_format === "12h",
      });
  const widgets = {
    today: (
      <>
        <div className="section-heading-row">
          <h2>Задачи на сегодня</h2>
          <SafeLink className="text-link" href="/app/quests">
            Все задачи
          </SafeLink>
        </div>
        <p>
          {d.plan.confirmed_at
            ? d.plan.rest
              ? "День отдыха"
              : `План: ${d.plan.progress.completed} из ${d.plan.progress.committed} · ${d.plan.progress.score === null ? "без оценки" : `${d.plan.progress.score}%`}`
            : "Выберите посильный объём дня. Начать можно с одной задачи."}
        </p>
        <SafeLink className="text-link" href={`/app/planning?date=${d.date}`}>
          {d.plan.confirmed_at ? "Открыть план дня" : "Составить план дня"}
        </SafeLink>
        <QuickTask date={d.date} />
        <TaskList tasks={d.tasks} top={d.plan.top_ids} changed={changed} notify={setNotice} />
        {d.more_tasks && <p>Показаны первые 100 задач. Полный список доступен в разделе задач.</p>}
        <details className="form-disclosure">
          <summary>
            Сроки и просрочки · {d.deadlines.length}
            {d.more_deadlines ? "+" : ""}
          </summary>
          {d.deadlines.length === 0 ? (
            <p className="ui-hint">
              Нет открытых задач со сроком в ближайшие семь дней или раньше.
            </p>
          ) : (
            <ul>
              {d.deadlines.map((q) => (
                <li key={q.id}>
                  <SafeLink href={`/app/quests/${q.id}`}>{q.title}</SafeLink> ·{" "}
                  {q.deadline && time(q.deadline)}
                </li>
              ))}
            </ul>
          )}
          {d.more_deadlines && <SafeLink href="/app/quests">Все сроки</SafeLink>}
        </details>
      </>
    ),
    calendar: (
      <>
        <h2>Ближайшие события</h2>
        {d.events.length === 0 ? (
          <p className="ui-hint">В ближайшие семь дней нет блоков времени.</p>
        ) : (
          <ul>
            {d.events.map((e) => (
              <li key={e.id}>
                {e.title} · {time(e.start)} — {time(e.end)}
              </li>
            ))}
          </ul>
        )}
        {d.more_events && <p>Показаны первые 50 событий.</p>}
        <SafeLink href="/app/calendar">Открыть календарь</SafeLink>
      </>
    ),
    habits: <DashboardHabits />,
    focus: <FocusSummary />,
    progress: (
      <>
        <ProgressionSummary compact />
        <details className="form-disclosure">
          <summary>Босс месяца и подробности дня</summary>
          <MonthlyBossSummary />
          <h2>Прогресс дня</h2>
          {d.plan.confirmed_at ? (
            <p>
              {d.plan.rest
                ? "Отдых без оценки."
                : d.plan.progress.score === null
                  ? "Нет задач для расчёта."
                  : `Выполнено ${d.plan.progress.completed} из ${d.plan.progress.committed} задач плана.`}
            </p>
          ) : (
            <p className="ui-hint">Подтвердите план, чтобы сохранить исходный объём дня.</p>
          )}
          <SafeLink href="/app/goals">Мои цели</SafeLink>
        </details>
      </>
    ),
  };
  return (
    <section aria-label="Сводка дня">
      <div className="today-heading">
        <div>
          <time dateTime={d.date}>
            {DateTime.fromISO(d.date).setLocale(p.language).toFormat("cccc, d MMMM")}
          </time>
          <h1 className="page-title">Сегодня</h1>
          <p className="page-intro">
            Ваш день. Ваш темп.
            <br />
            Начнём с того, что действительно важно.
          </p>
        </div>
        <div className="button-row">
          <Button asChild variant="secondary">
            <SafeLink href="/app/focus">
              Начать фокус <ArrowUpRight size={18} aria-hidden="true" />
            </SafeLink>
          </Button>
        </div>
      </div>
      <div className="today-pulse" aria-label="Ваш день в цифрах">
        <div>
          <span className="pulse-icon lavender">
            <Sparkles size={20} aria-hidden="true" />
          </span>
          <span>
            В списке сегодня
            <strong>
              {d.tasks.length}
              {d.more_tasks ? "+" : ""}
              <small> задач</small>
            </strong>
          </span>
        </div>
        <div>
          <span className="pulse-icon mint">
            <CircleCheck size={20} aria-hidden="true" />
          </span>
          <span>
            Уже сделано
            <strong>
              {d.tasks.filter((t) => t.status === "completed").length}
              <small> из списка</small>
            </strong>
          </span>
        </div>
        <div>
          <span className="pulse-icon peach">
            <CalendarDays size={20} aria-hidden="true" />
          </span>
          <span>
            Ближайшие 7 дней
            <strong>
              {d.events.length}
              {d.more_events ? "+" : ""}
              <small> событий</small>
            </strong>
          </span>
        </div>
      </div>
      <StartHint />
      {data.isError && (
        <p role="alert" className="ui-error">
          Не удалось обновить день. Показаны последние загруженные данные.{" "}
          <Button variant="ghost" onClick={() => void data.refetch()}>
            Повторить обновление
          </Button>
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <div className="dashboard-grid">
        {p.personalization.widget_order
          .filter((w) => !p.personalization.hidden_widgets?.includes(w))
          .map((w) => (
            <Card key={w} className="dashboard-widget" data-widget={w}>
              {widgets[w]}
            </Card>
          ))}
      </div>
      <div className="form-actions">
        <Button variant="ghost" onClick={() => void data.refetch()}>
          Обновить сводку
        </Button>
        <SafeLink href="/app/settings/appearance">Настроить виджеты</SafeLink>
      </div>
    </section>
  );
}
function TaskList({
  tasks,
  top,
  changed,
  notify,
}: {
  tasks: Quest[];
  top: string[];
  changed: () => Promise<void>;
  notify: (v: string) => void;
}) {
  return tasks.length === 0 ? (
    <p className="ui-hint">На сегодня нет задач. Добавьте первый небольшой шаг.</p>
  ) : (
    <ul className="day-tasks">
      {[...tasks]
        .sort((a, b) => Number(top.includes(b.id)) - Number(top.includes(a.id)))
        .map((q) => (
          <li key={q.id}>
            <div className="day-task-copy">
              <SafeLink href={`/app/quests/${q.id}`}>
                {top.includes(q.id) && <Star size={14} aria-label="Важное на сегодня" />}
                {q.title}
              </SafeLink>{" "}
              <small>{q.estimated_minutes ? `${q.estimated_minutes} мин` : "В своём темпе"}</small>
            </div>
            {["todo", "in_progress", "postponed"].includes(q.status) ? (
              <CompleteTask q={q} changed={changed} notify={notify} />
            ) : (
              <span>
                {" "}
                ·{" "}
                {q.status === "completed"
                  ? "Выполнено"
                  : q.status === "failed"
                    ? "Не получилось"
                    : "Пропущено"}
              </span>
            )}
          </li>
        ))}
    </ul>
  );
}
function CompleteTask({
  q,
  changed,
  notify,
}: {
  q: Quest;
  changed: () => Promise<void>;
  notify: (v: string) => void;
}) {
  const rewardNotice = useRewardNotice();
  const [busy, setBusy] = useState(false),
    key = useRef(crypto.randomUUID());
  return (
    <Button
      variant="ghost"
      className="complete-task"
      aria-label={`Завершить ${q.title}`}
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        try {
          const saved = await actOnQuest(
            q,
            "complete",
            {
              reason: "",
              planned_date: "",
              return_to_backlog: false,
              shift_deadline: false,
              deadline: "",
            },
            key.current,
          );
          rewardNotice(saved.reward);
          key.current = crypto.randomUUID();
          await changed();
          notify(`Выполнено: ${q.title}`);
        } catch (e) {
          notify(e instanceof Error ? e.message : "Не удалось завершить задачу. Обновите сводку.");
        } finally {
          setBusy(false);
        }
      }}
    >
      <Check size={19} aria-hidden="true" />
    </Button>
  );
}
