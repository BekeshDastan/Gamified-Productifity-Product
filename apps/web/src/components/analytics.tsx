"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DateTime } from "luxon";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import * as api from "@/lib/analytics";
import { useChallengeAction } from "./challenges";
import { AnalyticsChart } from "./analytics-charts";
import { AnalyticsHeatmap } from "./analytics-heatmap";
import { AnalyticsBalance } from "./analytics-balance";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Card } from "./ui/card";
export function Analytics() {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p role="status">Загружаем аналитику…</p>;
  return (
    <Workspace
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
    />
  );
}
function Workspace({ account, timezone }: { account: string; timezone: string }) {
  const discard = useConfirmDiscard();
  const today = DateTime.now().setZone(timezone).toISODate()!,
    initial = {
      from: DateTime.fromISO(today).minus({ days: 29 }).toISODate()!,
      to: DateTime.fromISO(today).plus({ days: 1 }).toISODate()!,
    },
    [range, setRange] = useState<api.AnalyticsRange>(initial),
    [from, setFrom] = useState(initial.from),
    [to, setTo] = useState(today),
    [date, setDate] = useState(today),
    [filter, setFilter] = useState(""),
    action = useChallengeAction(),
    cache = useQueryClient();
  const key = ["account", account, "analytics", timezone, range],
    q = useQuery({
      queryKey: key,
      queryFn: ({ signal }) => api.getAnalytics(range, signal),
      refetchInterval: 10000,
    }),
    samples = useQuery({
      queryKey: ["account", account, "analytics-template-samples", timezone, range],
      queryFn: ({ signal }) => api.getTemplateSamples(range, signal),
    });
  function applyRange(value: api.AnalyticsRange) {
    if (!discard()) return false;
    setRange(value);
    setFrom(value.from);
    setTo(DateTime.fromISO(value.to).minus({ days: 1 }).toISODate()!);
    setFilter("");
    return true;
  }
  function choose(day: string, kind = "") {
    setDate(day);
    setFilter(kind);
    requestAnimationFrame(() => document.getElementById("analytics-sources")?.focus());
  }
  const selected = q.data?.days.find((d) => d.date === date) ?? q.data?.days.at(-1);
  return (
    <div className="journal-layout analytics-workspace">
      <Card>
        <h2>Период наблюдения</h2>
        <div className="button-row">
          {[7, 30, 90, 365].map((n) => (
            <Button
              key={n}
              variant="secondary"
              onClick={() =>
                applyRange({
                  from: DateTime.fromISO(today)
                    .minus({ days: n - 1 })
                    .toISODate()!,
                  to: DateTime.fromISO(today).plus({ days: 1 }).toISODate()!,
                })
              }
            >
              {n === 365 ? "Год" : `${n} дней`}
            </Button>
          ))}
        </div>
        <form
          className="form-grid"
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              const a = DateTime.fromISO(from),
                b = DateTime.fromISO(to);
              if (!a.isValid || !b.isValid || b < a || b.diff(a, "days").days > 365)
                throw Error("Выберите от 1 до 366 календарных дней.");
              applyRange({ from, to: b.plus({ days: 1 }).toISODate()! });
            });
          }}
        >
          <Input
            type="date"
            label="Начало аналитики"
            value={from}
            required
            onChange={(e) => setFrom(e.target.value)}
          />
          <Input
            type="date"
            label="Конец аналитики включительно"
            value={to}
            required
            onChange={(e) => setTo(e.target.value)}
          />
          <Button type="submit">Показать период</Button>
        </form>
        <Button
          variant="secondary"
          disabled={action.pending}
          onClick={() =>
            void action.run(async () => {
              cache.setQueryData(key, await api.rebuildAnalytics(range));
              void cache.invalidateQueries({
                queryKey: ["account", account, "analytics-template-samples"],
              });
              void cache.invalidateQueries({ queryKey: ["account", account, "analytics-year"] });
            })
          }
        >
          Пересчитать из записей
        </Button>
        {action.error && <p role="alert">{action.error}</p>}
        {q.isPending ? (
          <p role="status">Собираем записи…</p>
        ) : q.isError ? (
          <p role="alert">Не удалось загрузить аналитику. Повторите пересчёт.</p>
        ) : (
          q.data && (
            <p role="status">
              {q.data.stale
                ? "Есть изменения: сводка пересчитывается. Пока показан предыдущий результат."
                : "Сводка актуальна."}{" "}
              {q.data.computed_at
                ? `Пересчитано ${DateTime.fromISO(q.data.computed_at).setZone(timezone).toFormat("dd.LL.yyyy HH:mm:ss")}.`
                : "Пересчёт ещё не выполнен."}{" "}
              Часовой пояс: {q.data.timezone}.
            </p>
          )
        )}
      </Card>
      {q.data && (
        <>
          <AnalyticsChart days={q.data.days} choose={choose} />
          <Card>
            <h2>Недельные итоги</h2>
            <p>
              Недели начинаются в понедельник. Крайние недели могут содержать только часть
              выбранного диапазона.
            </p>
            <div
              className="analytics-table-scroll"
              role="region"
              aria-label="Недельные итоги аналитики"
              tabIndex={0}
            >
              <table className="analytics-table">
                <caption>Суммы дневных записей с отдельными единицами</caption>
                <thead>
                  <tr>
                    <th>Неделя</th>
                    <th>Дней</th>
                    <th>Завершено задач</th>
                    <th>Обязательства плана</th>
                    <th>Выполнено в свой день</th>
                    <th>Полные нормы</th>
                    <th>Фокус, минут</th>
                    <th>XP</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.weeks.map((w) => (
                    <tr key={w.start}>
                      <th scope="row">{w.start}</th>
                      <td>{w.days}</td>
                      <td>{w.completed}</td>
                      <td>{w.planned}</td>
                      <td>{w.planned_completed}</td>
                      <td>
                        {w.habit_successes} / {w.habit_obligations}
                      </td>
                      <td>
                        {(w.focus_seconds / 60).toLocaleString("ru-RU", {
                          maximumFractionDigits: 2,
                        })}
                      </td>
                      <td>{w.xp_delta}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
          {selected && (
            <DaySources
              day={selected}
              timezone={timezone}
              filter={filter}
              setFilter={setFilter}
              chooseRange={applyRange}
            />
          )}
          <Card>
            <h2>Как считаются показатели</h2>
            {q.data.definitions.map((d) => (
              <details key={d.id}>
                <summary>{d.label}</summary>
                <p>Учитывается: {d.numerator}</p>
                <p>Знаменатель: {d.denominator}</p>
                <p>{d.basis}</p>
              </details>
            ))}
          </Card>
        </>
      )}
      <Card>
        <h2>Оценки повторяющихся задач</h2>
        <p>
          Выборка включает только записанные копии шаблона с известной оценкой и фактом в выбранном
          периоде. Для совета нужно минимум пять завершений. Старые задачи не объединяются по
          совпадению названий.
        </p>
        {samples.isPending && <p role="status">Загружаем выборки…</p>}
        {samples.isError && <p role="alert">Выборки не загрузились. Повторите пересчёт.</p>}
        {samples.data?.items.map((s) => (
          <details key={s.id + ":" + s.version}>
            <summary>
              {s.name}: {s.count} замеров{s.sufficient ? "" : " · пока мало данных"}
            </summary>
            <p>
              Средняя оценка: {s.average_estimated_minutes?.toFixed(1) ?? "Нет данных"} мин; средний
              факт:{" "}
              {s.average_actual_seconds === null
                ? "Нет данных"
                : (s.average_actual_seconds / 60).toFixed(1) + " мин"}
              .
            </p>
            <ul>
              {s.quest_ids.map((id) => (
                <li key={id}>
                  <SafeLink href={`/app/quests/${id}`}>Задача из выборки {id.slice(0, 8)}</SafeLink>
                </li>
              ))}
            </ul>
          </details>
        ))}
        {samples.data && !samples.data.items.length && (
          <p>Записанных копий шаблонов в периоде пока нет.</p>
        )}
      </Card>
      <AnalyticsBalance account={account} timezone={timezone} range={range} />
      <AnalyticsHeatmap
        account={account}
        timezone={timezone}
        choose={(day) => {
          if (applyRange({ from: day, to: DateTime.fromISO(day).plus({ days: 1 }).toISODate()! }))
            setDate(day);
        }}
      />
    </div>
  );
}
function DaySources({
  day: d,
  timezone,
  filter,
  setFilter,
  chooseRange,
}: {
  day: api.AnalyticsDay;
  timezone: string;
  filter: string;
  setFilter: (v: string) => void;
  chooseRange: (range: api.AnalyticsRange) => void;
}) {
  const rows = d.sources.filter(
      (s) => !filter || filter === "time" || s.kind === filter || s.qualifier === filter,
    ),
    labels: Record<string, string> = {
      on_time: "Вовремя",
      late: "Поздно",
      no_deadline: "Без срока",
    };
  return (
    <Card>
      <h2 id="analytics-sources" tabIndex={-1}>
        Источники за {d.date}
      </h2>
      <p>
        {api.scoreStates[d.scores.state] ?? "Нет оценки"} · оценка дня: {d.scores.daily ?? "—"}.
      </p>
      <p>
        Компоненты собственного плана: задачи T — {fraction(d.scores.t)}, нормы H —{" "}
        {fraction(d.scores.h)}, выполнимость объёма R — {fraction(d.scores.r)}. Неактивные
        компоненты исключены из формулы.
      </p>
      <p>
        Регулярность: {d.scores.discipline.score ?? "Недостаточно данных"}; завершённых активных
        дней: {d.scores.discipline.active_days}; обязательств по задачам:{" "}
        {d.scores.discipline.task_obligations}; периодов привычек:{" "}
        {d.scores.discipline.habit_obligations}.
      </p>
      <p>
        Это выполнение собственных планов. Балл не оценивает человека и не используется для
        публичного рейтинга.
      </p>
      <Button
        variant="secondary"
        onClick={() => chooseRange({ from: d.scores.discipline.from, to: d.scores.discipline.to })}
      >
        Открыть выборку регулярности
      </Button>
      {d.plan?.confirmed && (
        <details>
          <summary>Исходный подтверждённый план</summary>
          <p>
            Часовой пояс исходного дня: {d.plan.timezone}. Доступно минут при подтверждении:{" "}
            {d.plan.input.available_minutes}. Обязательств: {d.planned}; выполнено в свой день:{" "}
            {d.planned_completed}.
          </p>
          <ul>
            {d.plan.snapshot?.items.map((q) => (
              <li key={q.id}>
                <SafeLink href={`/app/quests/${q.id}`}>{q.title}</SafeLink> · вес {q.weight} ·
                оценка {q.estimated_minutes || "Неизвестно"} мин
              </li>
            ))}
          </ul>
          <SafeLink href={`/app/planning?date=${d.date}`}>Открыть план и дополнения</SafeLink>
        </details>
      )}
      <Select label="Фильтр источников" value={filter} onChange={(e) => setFilter(e.target.value)}>
        <option value="">Все записи</option>
        {Object.entries(api.sourceKinds).map(([value, label]) => (
          <option key={value} value={value}>
            {label}
          </option>
        ))}
        {Object.entries(labels).map(([value, label]) => (
          <option key={value} value={value}>
            {label}
          </option>
        ))}
        <option value="time">Замеры времени</option>
      </Select>
      {filter !== "time" && (
        <div
          className="analytics-table-scroll"
          role="region"
          aria-label="Исходные записи дня"
          tabIndex={0}
        >
          <table className="analytics-table">
            <caption>Записи, из которых построен выбранный день</caption>
            <thead>
              <tr>
                <th>Вид</th>
                <th>Источник</th>
                <th>Количество</th>
                <th>Время или граница периода</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s, i) => (
                <tr key={s.kind + ":" + s.id + ":" + i}>
                  <td>
                    {api.sourceKinds[s.kind] ?? "Запись"}
                    {s.qualifier && labels[s.qualifier] ? ` · ${labels[s.qualifier]}` : ""}
                  </td>
                  <td>
                    <SafeLink href={s.url}>
                      {s.kind === "xp" || s.kind === "coins" ? "История наград" : s.name}
                    </SafeLink>
                    <small>Запись {s.id}</small>
                  </td>
                  <td>
                    {s.value} {unit(s.unit)}
                  </td>
                  <td>{DateTime.fromISO(s.at).setZone(timezone).toFormat("dd.LL HH:mm:ss")}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {!rows.length && (
            <p>Записей этого вида нет. Это отсутствие записей, без оценки качества дня.</p>
          )}
        </div>
      )}
      {(filter === "time" || !filter) && (
        <>
          <h3>Длительность завершённых задач</h3>
          <p>
            Сопоставимых пар: {d.comparable_tasks}. Замеров факта: {d.actual_known_tasks}. Ручное
            время заменяет связанный фокус, а не прибавляется к нему.
          </p>
          <div
            className="analytics-table-scroll"
            role="region"
            aria-label="Замеры завершённых задач"
            tabIndex={0}
          >
            <table className="analytics-table">
              <caption>Неизвестный факт отличается от явно записанного нуля</caption>
              <thead>
                <tr>
                  <th>Задача</th>
                  <th>Оценка, мин</th>
                  <th>Факт, мин</th>
                  <th>Источник факта</th>
                  <th>Фокус, секунд</th>
                </tr>
              </thead>
              <tbody>
                {d.task_times.map((t) => (
                  <tr key={t.id}>
                    <td>
                      <SafeLink href={t.url}>{t.title}</SafeLink>
                    </td>
                    <td>{t.estimated_minutes ?? "Нет оценки"}</td>
                    <td>
                      {t.actual_seconds === null
                        ? "Нет замера"
                        : (t.actual_seconds / 60).toLocaleString("ru-RU", {
                            maximumFractionDigits: 2,
                          })}
                    </td>
                    <td>
                      {t.mode === "manual_override"
                        ? "Ручной замер"
                        : t.actual_seconds === null
                          ? "Не записан"
                          : "Подтверждённый фокус"}
                    </td>
                    <td>{t.focus_seconds}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
      <details>
        <summary>Нормы привычек этого дня</summary>
        <ul>
          {d.habits.map((h) => (
            <li key={h.period_id}>
              <SafeLink href={h.url}>{h.name}</SafeLink>: {h.amount} / {h.target} {h.unit} ·{" "}
              {h.excluded ? "Исключённый период" : h.closed ? "Период закрыт" : "Период ещё открыт"}{" "}
              · зона {h.timezone}
            </li>
          ))}
        </ul>
      </details>
    </Card>
  );
}
function fraction(v: number | null) {
  return v === null ? "Нет компонента" : `${(v * 100).toFixed(1)}%`;
}
function unit(value: string) {
  return (
    (
      {
        tasks: "задач",
        seconds: "секунд",
        events: "событий",
        goals: "целей",
        xp: "XP",
        coins: "монет",
      } as Record<string, string>
    )[value] ?? value
  );
}
