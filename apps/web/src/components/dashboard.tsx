"use client";
import { useRef, useState } from "react";
import { z } from "zod";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { getDashboard } from "@/lib/dashboard";
import { actOnQuest, emptyQuest, saveQuest, type Quest } from "@/lib/quests";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { DashboardHabits } from "./habits";
import { ProgressionSummary, useRewardNotice } from "./progression";
import { Input } from "./ui/field";
import { Form } from "./ui/form";
const quick = z.object({ title: z.string().trim().min(1, "Укажите название").max(200) });
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
  if (!id) return null;
  if (!data.data || !settings.data)
    return (
      <Card>
        <p role={data.isError ? "alert" : "status"}>
          {data.isError ? "Не удалось загрузить сводку." : "Загружаем ваш день…"}
        </p>
        <Button onClick={() => void data.refetch()}>Повторить</Button>
      </Card>
    );
  const d = data.data,
    p = settings.data;
  const changed = () => {
    void cache.invalidateQueries({ queryKey: ["account", id] });
  };
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
        <h2>Сегодня · {d.date}</h2>
        <p>
          {d.plan.confirmed_at
            ? d.plan.rest
              ? "День отдыха"
              : `План: ${d.plan.progress.completed} из ${d.plan.progress.committed} · ${d.plan.progress.score === null ? "без оценки" : `${d.plan.progress.score}%`}`
            : "План пока не подтверждён"}
        </p>
        <SafeLink href={`/app/planning?date=${d.date}`}>Открыть план дня</SafeLink>
        <QuickTask date={d.date} changed={changed} />
        <TaskList tasks={d.tasks} top={d.plan.top_ids} changed={changed} notify={setNotice} />
        {d.more_tasks && <p>Показаны первые 100 задач. Полный список доступен в разделе задач.</p>}
        <h3>Сроки и просрочки</h3>
        {d.deadlines.length === 0 ? (
          <p className="ui-hint">Нет открытых задач со сроком в ближайшие семь дней или раньше.</p>
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
    focus: (
      <>
        <h2>Фокус</h2>
        <p className="ui-hint">Выберите одну главную задачу и выделите ей время в плане дня.</p>
        <SafeLink href="/app/planning">Выделить время</SafeLink>
      </>
    ),
    progress: (
      <>
        <ProgressionSummary compact />
        <h2>Прогресс дня</h2>
        {d.plan.confirmed_at ? (
          <p>
            {d.plan.rest
              ? "Отдых без оценки."
              : d.plan.progress.score === null
                ? "Нет задач для расчёта."
                : `Выполнено ${d.plan.progress.completed_weight} из ${d.plan.progress.total_weight} весовых единиц исходного плана.`}
          </p>
        ) : (
          <p className="ui-hint">Подтвердите план, чтобы сохранить исходный объём дня.</p>
        )}
        <SafeLink href="/app/goals">Мои цели</SafeLink>
      </>
    ),
  };
  return (
    <section aria-label="Сводка дня">
      <div className="form-actions">
        <Button onClick={() => void data.refetch()}>Обновить сводку</Button>
        <SafeLink href="/app/settings/appearance">Настроить виджеты</SafeLink>
      </div>
      {notice && <p role="status">{notice}</p>}
      <div className="dashboard-grid">
        {p.personalization.widget_order
          .filter((w) => !p.personalization.hidden_widgets?.includes(w))
          .map((w) => (
            <Card key={w} className="dashboard-widget">
              {widgets[w]}
            </Card>
          ))}
      </div>
    </section>
  );
}
function QuickTask({ date, changed }: { date: string; changed: () => void }) {
  const flow = useValidatedForm({
    schema: quick,
    defaultValues: { title: "" },
    fields: ["title"],
    save: async (v) => {
      await saveQuest({ ...emptyQuest, title: v.title, planned_date: date });
      changed();
      return { title: "" };
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Быстро добавить задачу"
        {...flow.form.register("title")}
        error={flow.form.formState.errors.title?.message}
      />
      <Button type="submit">Добавить на сегодня</Button>
    </Form>
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
  changed: () => void;
  notify: (v: string) => void;
}) {
  return tasks.length === 0 ? (
    <p className="ui-hint">На сегодня нет задач. Добавьте первый небольшой шаг.</p>
  ) : (
    <ul>
      {[...tasks]
        .sort((a, b) => Number(top.includes(b.id)) - Number(top.includes(a.id)))
        .map((q) => (
          <li key={q.id}>
            <SafeLink href={`/app/quests/${q.id}`}>
              {top.includes(q.id) ? "★ " : ""}
              {q.title}
            </SafeLink>{" "}
            · {q.estimated_minutes ? `${q.estimated_minutes} мин` : "без оценки"}
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
  changed: () => void;
  notify: (v: string) => void;
}) {
  const rewardNotice = useRewardNotice();
  const [busy, setBusy] = useState(false),
    key = useRef(crypto.randomUUID());
  return (
    <Button
      variant="ghost"
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
          notify(`Выполнено: ${q.title}`);
          changed();
        } catch (e) {
          notify(e instanceof Error ? e.message : "Не удалось завершить задачу. Обновите сводку.");
        } finally {
          setBusy(false);
        }
      }}
    >
      Завершить {q.title}
    </Button>
  );
}
