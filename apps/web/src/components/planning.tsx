"use client";
import { Suspense, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings, type Preferences } from "@/lib/settings";
import { getPlan, getPlanWeek, confirmPlan, type DayPlan } from "@/lib/planning";
import { ApiError } from "@/lib/api/client";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { PlanEditor, PlanBlocks } from "./plan-editor";
import { PlanRoutines } from "./plan-routines";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Card } from "./ui/card";
export function Planning() {
  const auth = useAuth(),
    settings = useSettings();
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!auth.session)
    return (
      <p>
        Для планирования{" "}
        <SafeLink href="/auth/login?next=/app/planning">войдите в аккаунт</SafeLink>.
      </p>
    );
  if (!settings.data)
    return (
      <p role={settings.isError ? "alert" : "status"}>
        {settings.isError ? "Не удалось загрузить настройки." : "Загружаем настройки…"}
        <Button onClick={() => void settings.refetch()}>Повторить</Button>
      </p>
    );
  return (
    <Suspense fallback={<p role="status">Загружаем план…</p>}>
      <Workspace
        key={auth.session.account.id}
        account={auth.session.account.id}
        preferences={settings.data}
      />
    </Suspense>
  );
}
function Workspace({ account, preferences }: { account: string; preferences: Preferences }) {
  const search = useSearchParams(),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  const [date, setDate] = useState(() => {
    const requested = search.get("date");
    return requested && /^\d{4}-\d{2}-\d{2}$/.test(requested) && DateTime.fromISO(requested).isValid
      ? requested
      : DateTime.now().setZone(preferences.timezone).toISODate()!;
  });
  const [mode, setMode] = useState("day"),
    [editing, setEditing] = useState(false),
    [notice, setNotice] = useState("");
  const dt = DateTime.fromISO(date),
    from = dt
      .minus({ days: preferences.week_start === 7 ? dt.weekday % 7 : dt.weekday - 1 })
      .toISODate()!;
  const data = useQuery({
    queryKey: ["account", account, "day-plan", date],
    queryFn: ({ signal }) => getPlan(date, signal),
    refetchInterval: (q) => (q.state.data?.availability?.materializing ? 500 : false),
  });
  const week = useQuery({
    queryKey: ["account", account, "plan-week", from],
    enabled: mode === "week",
    queryFn: ({ signal }) => getPlanWeek(from, signal),
  });
  function select(next: string) {
    if (!next || !DateTime.fromISO(next).isValid || !discard()) return;
    setDate(next);
    setEditing(false);
    setNotice("");
  }
  function changed() {
    setEditing(false);
    setNotice("План сохранён.");
    void cache.invalidateQueries({ queryKey: ["account", account] });
  }
  const p = data.data;
  let initial = p;
  if (p && !p.id) {
    const day = dt.weekday;
    const windows = preferences.work_intervals
      .filter((w) => w.day === day)
      .map(({ start, end }) => ({ start, end }));
    const rest = preferences.rest_days.includes(day);
    initial = {
      ...p,
      windows: rest ? [] : windows,
      rest,
      available_minutes: rest
        ? 0
        : windows.reduce(
            (sum, w) =>
              sum +
              DateTime.fromFormat(w.end, "HH:mm").diff(
                DateTime.fromFormat(w.start, "HH:mm"),
                "minutes",
              ).minutes,
            0,
          ),
    };
  }
  const showEditor = editing || (!!p && !p.id);
  return (
    <div className="planning-workspace">
      <div className="plan-controls">
        <Input
          label="Дата плана"
          type="date"
          value={date}
          onChange={(e) => select(e.target.value)}
        />
        <Select
          label="Вид плана"
          value={mode}
          onChange={(e) => {
            if (discard()) {
              setMode(e.target.value);
              setEditing(false);
            }
          }}
        >
          <option value="day">День</option>
          <option value="week">Неделя</option>
        </Select>
        <Button
          variant="ghost"
          onClick={() => select(DateTime.now().setZone(preferences.timezone).toISODate()!)}
        >
          Сегодня
        </Button>
      </div>
      {notice && <p role="status">{notice}</p>}
      {mode === "week" && (
        <Card>
          <h2>Неделя с {from}</h2>
          <p className="ui-hint">
            Выберите день для редактирования. Недельный обзор показывает также будущие повторения
            календаря.
          </p>
          {week.isPending && <p role="status">Загружаем неделю…</p>}
          {week.isError && (
            <p role="alert">
              Не удалось загрузить неделю.
              <Button onClick={() => void week.refetch()}>Повторить</Button>
            </p>
          )}
          <div className="plan-week">
            {week.data?.map((day) => (
              <div key={day.date}>
                <Button
                  variant={day.date === date ? "primary" : "secondary"}
                  onClick={() => select(day.date)}
                >
                  {DateTime.fromISO(day.date).setLocale("ru").toFormat("ccc, dd.LL")}
                </Button>
                <p>
                  {day.rest ? "Отдых" : day.confirmed_at ? "Подтверждён" : "Черновик"} ·{" "}
                  {day.items.length} задач
                </p>
                <p className="ui-hint">Дефицит: {day.availability?.deficit_minutes ?? 0} мин</p>
              </div>
            ))}
          </div>
        </Card>
      )}
      {data.isPending && <p role="status">Загружаем день…</p>}
      {data.isError && (
        <p role="alert">
          {data.error instanceof ApiError ? data.error.message : "Не удалось загрузить план."}
          <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      {p && initial && (
        <>
          <Card>
            <h2>
              {p.date} ·{" "}
              {p.rest ? "Отдых" : p.confirmed_at ? "Подтверждённый день" : "Черновик дня"}
            </h2>
            <p className="ui-hint">
              Пояс плана: {p.timezone}. Сводка относится к последнему сохранению. Черновик не
              считается обязательством.
            </p>
            <div className="button-row">
              <Button
                variant="secondary"
                onClick={() => {
                  if (discard()) setEditing(!showEditor);
                }}
              >
                {showEditor ? "Закрыть редактор" : "Изменить план"}
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  if (discard()) {
                    setEditing(false);
                    void data.refetch();
                  }
                }}
              >
                Загрузить свежую версию
              </Button>
              <SafeLink href="/app/quests">Задачи</SafeLink>
              <SafeLink href="/app/calendar">Календарь</SafeLink>
            </div>
          </Card>
          {showEditor ? (
            <PlanEditor key={p.date + ":" + p.version} plan={initial} onChanged={changed} />
          ) : (
            <>
              <PlanCapacity plan={p} />
              <ConfirmPanel key={p.date + ":" + p.version} plan={p} onChanged={changed} />
              <PlanBlocks key={p.date + ":" + p.version} plan={p} onChanged={changed} />
              <PlanRoutines date={date} onChanged={changed} />
            </>
          )}
          {p.snapshot && (
            <Card>
              <h2>Исходный подтверждённый план</h2>
              <p>
                {p.snapshot.input.rest
                  ? "Отдых без оценки"
                  : p.progress.score === null
                    ? "Нет задач для оценки"
                    : `Выполнение исходного плана: ${p.progress.score} из 100 · ${p.progress.completed} из ${p.progress.committed} задач`}
              </p>
              <p className="ui-hint">
                Веса приоритетов: низкий 1, обычный 2, высокий 3. Учитывается выполнение в дату
                этого плана, в его поясе. Более позднее выполнение и новые задачи показаны отдельно
                и не заменяют исходные обязательства.
              </p>
              <details>
                <summary>Посмотреть исходный снимок</summary>
                <p>
                  Лимит: {p.snapshot.input.available_minutes} мин. Главных задач:{" "}
                  {p.snapshot.input.top_ids.length}.
                </p>
                <ul>
                  {p.snapshot.items.map((q) => (
                    <li key={q.id}>
                      {q.title} · {q.estimated_minutes} мин · вес {q.weight}
                    </li>
                  ))}
                </ul>
                <p>Блоков на момент подтверждения: {p.snapshot.blocks.length}</p>
              </details>
            </Card>
          )}
          <Card>
            <h2>История плана</h2>
            {!p.changes.length ? (
              <p className="ui-hint">Подтверждений и дополнений пока нет.</p>
            ) : (
              <ol>
                {p.changes.map((c, i) => (
                  <li key={i}>
                    {DateTime.fromISO(c.at).setZone(p.timezone).toFormat("dd.LL HH:mm")} ·{" "}
                    {c.kind === "confirmed"
                      ? "Подтверждение"
                      : c.kind === "template"
                        ? "Шаблон"
                        : "Дополнение"}
                    <p>{c.reason}</p>
                  </li>
                ))}
              </ol>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
function ConfirmPanel({ plan, onChanged }: { plan: DayPlan; onChanged: () => void }) {
  const [agree, setAgree] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const key = useRef(crypto.randomUUID());
  if (plan.confirmed_at) return null;
  async function confirm() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await confirmPlan(plan, key.current);
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Не удалось подтвердить план.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>Подтвердить день</h2>
      <p>
        Будут сохранены исходные {plan.items.length} задач, их оценки, приоритеты и блоки.
        Последующие изменения станут дополнениями.
      </p>
      <label className="check-row">
        <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} />Я
        просмотрел задачи, ёмкость и пересечения
      </label>
      <Button
        disabled={!agree || busy || !plan.id || plan.availability?.materializing}
        onClick={() => void confirm()}
      >
        Подтвердить план
      </Button>
      {error && <p role="alert">{error}</p>}
    </Card>
  );
}
export function PlanCapacity({ plan }: { plan: DayPlan }) {
  const a = plan.availability;
  if (!a) return null;
  const task = (id: string) => plan.items.find((q) => q.id === id)?.title ?? id;
  const time = (v: string) => DateTime.fromISO(v).setZone(plan.timezone).toFormat("dd.LL HH:mm");
  return (
    <Card>
      <h2>Ёмкость и занятость</h2>
      <p>
        Лимит дня: {plan.available_minutes} мин · Оценка задач: {a.planned_minutes} мин
      </p>
      <p>
        Уже назначено задачам: {a.scheduled_minutes} мин · Осталось работы: {a.remaining_minutes}{" "}
        мин
      </p>
      <p>
        Свободно в окнах: {a.free_minutes} мин · Ёмкость для оставшейся работы:{" "}
        {a.remaining_capacity} мин
      </p>
      <p>Дефицит времени: {a.deficit_minutes} мин</p>
      {!plan.windows.length && (
        <p className="ui-hint">
          Рабочие окна не заданы: интервалы рассчитаны в пределах всего местного дня. Лимит минут
          продолжает действовать.
        </p>
      )}
      {a.materializing && <p role="status">Догружаем повторения календаря…</p>}
      <h3>Не вместились по оценке</h3>
      {a.unfit_ids.length ? (
        <ul>
          {a.unfit_ids.map((id) => (
            <li key={id}>
              <SafeLink href={`/app/quests?id=${id}`}>{task(id)}</SafeLink>
            </li>
          ))}
        </ul>
      ) : (
        <p className="ui-hint">Известные оценки помещаются в доступную ёмкость.</p>
      )}
      {!!a.unknown_ids.length && (
        <p>
          Без оценки времени: {a.unknown_ids.map(task).join(", ")}. Уточните длительность в задаче.
        </p>
      )}
      <h3>Пересечения блоков</h3>
      {a.conflicts.length ? (
        <ul>
          {a.conflicts.map((c, i) => (
            <li key={i}>
              {a.blocks.find((b) => b.id === c.first)?.title ?? "Блок"} и{" "}
              {a.blocks.find((b) => b.id === c.second)?.title ?? "Блок"} · {time(c.start)}–
              {time(c.end)}
            </li>
          ))}
        </ul>
      ) : (
        <p className="ui-hint">Пересечений нет.</p>
      )}
      {a.more_conflicts && <p>Показаны первые 200 пересечений.</p>}
      <details>
        <summary>Свободные интервалы ({a.free_slots.length})</summary>
        <ul>
          {a.free_slots.map((slot, i) => (
            <li key={i}>
              {time(slot.start)}–{time(slot.end)}
            </li>
          ))}
        </ul>
      </details>
      <details>
        <summary>Блоки дня ({a.blocks.length})</summary>
        <ul>
          {a.blocks.map((b) => (
            <li key={b.id}>
              {b.title} · {time(b.start)}–{time(b.end)}
            </li>
          ))}
        </ul>
      </details>
    </Card>
  );
}
