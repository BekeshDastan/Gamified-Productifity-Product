"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { DateTime } from "luxon";
import { z } from "zod";
import * as api from "@/lib/analytics-reports";
import type { AnalyticsRange } from "@/lib/analytics";
import { SafeLink, useConfirmDiscard, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { useChallengeAction } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
export function AnalyticsReports(props: {
  account: string;
  timezone: string;
  openRange: (value: AnalyticsRange) => void;
}) {
  const params = useSearchParams(),
    requested = params.get("report_id") ?? "",
    valid = !requested || z.uuid().safeParse(requested).success;
  return (
    <>
      {!valid && <p role="alert">Ссылка на отчёт неверна. Выберите отчёт из истории.</p>}
      <ReportsWorkspace key={requested} {...props} initialID={valid ? requested : ""} />
    </>
  );
}
function ReportsWorkspace({
  account,
  timezone,
  initialID,
  openRange,
}: {
  account: string;
  timezone: string;
  initialID: string;
  openRange: (value: AnalyticsRange) => void;
}) {
  const now = DateTime.now().setZone(timezone),
    [kind, setKind] = useState<"week" | "month">("week"),
    [start, setStart] = useState(now.startOf("week").minus({ weeks: 1 }).toISODate()!),
    [preview, setPreview] = useState<api.ReportPreview | null>(null),
    [operationID, setOperationID] = useState(""),
    [ack, setAck] = useState(false),
    [id, setID] = useState(initialID),
    [before, setBefore] = useState(""),
    action = useChallengeAction(),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  useUnsavedChanges(!!preview);
  const list = useQuery({
      queryKey: ["account", account, "analytics-reports", before],
      queryFn: ({ signal }) => api.listReports(before, signal),
      refetchInterval: 30000,
    }),
    detail = useQuery({
      queryKey: ["account", account, "analytics-report", id],
      queryFn: ({ signal }) => api.getReport(id, signal),
      enabled: !!id,
    });
  function resetPreview() {
    if (!discard()) return false;
    setPreview(null);
    setAck(false);
    return true;
  }
  return (
    <Card>
      <h2>Недельные и месячные отчёты</h2>
      <p>
        Отчёт составляется по числам и готовому текстовому шаблону. Фоновый обработчик сохраняет
        итоги полных наблюдавшихся периодов. Для ручного снимка сначала просмотрите записи; заметки
        дневника и свободные комментарии не входят в отчёт.
      </p>
      <Select
        label="Период отчёта"
        value={kind}
        onChange={(e) => {
          if (resetPreview()) {
            const value = e.target.value as "week" | "month";
            setKind(value);
            setStart(
              value === "week"
                ? now.startOf("week").minus({ weeks: 1 }).toISODate()!
                : now.startOf("month").minus({ months: 1 }).toISODate()!,
            );
          }
        }}
      >
        <option value="week">Неделя</option>
        <option value="month">Месяц</option>
      </Select>
      <Input
        type="date"
        label="Начало периода отчёта"
        value={start}
        onChange={(e) => {
          if (resetPreview()) setStart(e.target.value);
        }}
      />
      <p>
        Неделя начинается в понедельник, месяц — с первого числа. Сохраняется только завершённый
        период. Часовой пояс: {timezone}.
      </p>
      <Button
        variant="secondary"
        disabled={action.pending}
        onClick={() =>
          void action.run(async () => {
            if (!resetPreview()) return;
            setPreview(await api.previewReport({ kind, starts_on: start, timezone }));
            setOperationID(crypto.randomUUID());
            setAck(false);
          })
        }
      >
        Показать предпросмотр отчёта
      </Button>
      {preview && (
        <section>
          <h3>Предпросмотр отчёта</h3>
          <p>
            Действует до{" "}
            {DateTime.fromISO(preview.expires_at)
              .setZone(preview.snapshot.timezone)
              .toFormat("HH:mm:ss")}
            .
          </p>
          <SnapshotView value={preview.snapshot} openRange={openRange} />
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
            Подтверждаю сохранение неизменяемого снимка отчёта
          </label>
          <div className="button-row">
            <Button
              disabled={!ack || action.pending}
              onClick={() =>
                void action.run(async () => {
                  setAck(false);
                  const value = await api.saveReport({
                    operation_id: operationID,
                    preview_id: preview.id,
                    confirm: true,
                  });
                  setPreview(null);
                  setID(value.id);
                  cache.setQueryData(["account", account, "analytics-report", value.id], value);
                  void cache.invalidateQueries({
                    queryKey: ["account", account, "analytics-reports"],
                  });
                })
              }
            >
              Сохранить снимок отчёта
            </Button>
            <Button
              variant="secondary"
              disabled={action.pending}
              onClick={() => {
                if (resetPreview()) setOperationID("");
              }}
            >
              Отменить предпросмотр
            </Button>
          </div>
        </section>
      )}
      {action.error && (
        <p role="alert">
          {action.error} При изменении записей или истечении срока создайте новый предпросмотр.
        </p>
      )}
      <h3>История отчётов</h3>
      {list.isPending && <p role="status">Загружаем отчёты…</p>}
      {list.isError && (
        <p role="alert">
          Не удалось загрузить историю.{" "}
          <Button variant="secondary" onClick={() => void list.refetch()}>
            Повторить
          </Button>
        </p>
      )}
      {list.data?.items.map((r) => (
        <div key={r.id}>
          <Button
            variant="secondary"
            onClick={() => {
              if (resetPreview()) setID(r.id);
            }}
          >
            {r.kind === "week" ? "Неделя" : "Месяц"} {r.from} · снимок {r.revision}
          </Button>
          <p>
            {r.automatic ? "Сформирован автоматически" : "Сохранён вами"} · {r.timezone} ·{" "}
            {DateTime.fromISO(r.created_at).setZone(timezone).toFormat("dd.LL.yyyy HH:mm")}
          </p>
        </div>
      ))}
      {list.data && !list.data.items.length && <p>Сохранённых отчётов пока нет.</p>}
      {list.data?.next && (
        <Button
          variant="secondary"
          onClick={() => {
            if (resetPreview()) setBefore(list.data!.next);
          }}
        >
          Предыдущие отчёты
        </Button>
      )}
      {before && (
        <Button
          variant="secondary"
          onClick={() => {
            if (resetPreview()) setBefore("");
          }}
        >
          Новые отчёты
        </Button>
      )}
      {detail.isPending && id && <p role="status">Загружаем снимок…</p>}
      {detail.isError && (
        <p role="alert">Отчёт не найден или недоступен. Выберите другой отчёт из истории.</p>
      )}
      {detail.data && (
        <section>
          <h3>Сохранённый отчёт</h3>
          <p>
            Снимок {detail.data.revision}. Последующие коррекции не переписывают его: для
            актуального результата создайте новый предпросмотр.
          </p>
          <SnapshotView value={detail.data.snapshot} openRange={openRange} />
          <SafeLink href={`/app/analytics?report_id=${detail.data.id}`}>
            Ссылка на этот отчёт
          </SafeLink>
        </section>
      )}
    </Card>
  );
}
const names: Record<string, string> = {
  tasks: "Завершённые задачи",
  habit_norms: "Полные нормы привычек",
  focus: "Подтверждённая работа",
  goals: "Достигнутые цели",
  xp: "Изменение опыта",
  coins: "Изменение монет",
};
function SnapshotView({
  value: v,
  openRange,
}: {
  value: api.Snapshot;
  openRange: (range: AnalyticsRange) => void;
}) {
  return (
    <>
      <p>{v.text}</p>
      <p>
        Снимок данных:{" "}
        {DateTime.fromISO(v.computed_at).setZone(v.timezone).toFormat("dd.LL.yyyy HH:mm:ss")} (
        {v.timezone}). Наблюдения аккаунта начались{" "}
        {DateTime.fromISO(v.observed_from).setZone(v.timezone).toFormat("dd.LL.yyyy HH:mm")}.
      </p>
      {(!v.current_observed || !v.previous_observed) && (
        <p>
          Неполные наблюдения не заменяются нулевым результатом. Числа текущего периода — только
          отмеченные записи; относительное сравнение недоступно.
        </p>
      )}
      <div
        className="analytics-table-scroll"
        role="region"
        aria-label="Сравнение равных периодов отчёта"
        tabIndex={0}
      >
        <table className="analytics-table">
          <caption>
            Текущий период {v.from} — {v.to}; предыдущий равной длины {v.previous_range.from} —{" "}
            {v.previous_range.to}. Конец каждого диапазона не включён.
          </caption>
          <thead>
            <tr>
              <th>Показатель</th>
              <th>Текущие записи</th>
              <th>Предыдущий период</th>
              <th>Разница</th>
              <th>Изменение, %</th>
            </tr>
          </thead>
          <tbody>
            {v.comparisons.map((c) => (
              <tr key={c.metric}>
                <th scope="row">{names[c.metric] ?? c.metric}</th>
                <td>
                  {format(c.current, c.unit)}
                  {!c.current_observed ? " · неполные наблюдения" : ""}
                </td>
                <td>
                  {c.previous === null ? "Нет полных наблюдений" : format(c.previous, c.unit)}
                </td>
                <td>
                  {c.delta === null ? "—" : `${c.delta > 0 ? "+" : ""}${format(c.delta, c.unit)}`}
                </td>
                <td>
                  {c.percent === null
                    ? c.previous === 0 && c.current_observed
                      ? "Только абсолютная разница"
                      : "—"
                    : c.percent.toLocaleString("ru-RU", { maximumFractionDigits: 1 }) + "%"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="button-row">
        <Button variant="secondary" onClick={() => openRange({ from: v.from, to: v.to })}>
          Открыть текущие записи периода
        </Button>
        <Button variant="secondary" onClick={() => openRange(v.previous_range)}>
          Открыть текущие записи предыдущего периода
        </Button>
      </div>
      <p>
        Эти переходы открывают актуальную аналитику. Исходные значения сохранённого отчёта остаются
        ниже.
      </p>
      <details>
        <summary>Записи, сохранённые в снимке</summary>
        {v.days.map((d) => (
          <details key={d.date}>
            <summary>
              {d.date}: отмечено задач {d.completed}, фокус {d.focus_seconds} сек
            </summary>
            <ul>
              {d.sources.map((s, i) => (
                <li key={s.kind + ":" + s.id + ":" + i}>
                  <SafeLink href={s.url}>
                    {s.kind === "xp" || s.kind === "coins" ? "История наград" : s.name}
                  </SafeLink>{" "}
                  · {format(s.value, s.unit)}
                </li>
              ))}
            </ul>
            {!d.sources.length && (
              <p>Записей этого дня нет. Оценка плана: {d.scores.daily ?? "отсутствует"}.</p>
            )}
          </details>
        ))}
      </details>
      <details>
        <summary>Личные ориентиры в этом снимке</summary>
        <ul>
          {v.balance.axes.map((a) => (
            <li key={a.id}>
              {a.name}: {a.actual === null ? "Нет данных" : format(a.actual, a.unit)} /{" "}
              {a.target === null || a.target <= 0 ? "Ориентир не задан" : format(a.target, a.unit)}{" "}
              · {a.percent === null ? "Нет оценки" : a.percent.toFixed(1) + "%"}
            </li>
          ))}
        </ul>
      </details>
      <details>
        <summary>Личные рекорды на момент снимка</summary>
        {v.records.map((r) => (
          <div key={r.id}>
            <h4>{r.name}</h4>
            <p>
              {r.available && r.value !== null
                ? `${r.value} ${r.unit}`
                : "Пока недостаточно данных"}
            </p>
            <p>{r.explanation}</p>
            {r.sources.map((s, i) => (
              <p key={i}>
                <SafeLink href={s.url}>{s.title}</SafeLink>
              </p>
            ))}
          </div>
        ))}
      </details>
    </>
  );
}
function format(value: number, unit: string) {
  const labels: Record<string, string> = {
    tasks: "задач",
    periods: "периодов",
    seconds: "минут",
    minutes: "минут",
    goals: "целей",
    xp: "XP",
    coins: "монет",
  };
  return `${(unit === "seconds" ? value / 60 : value).toLocaleString("ru-RU", { maximumFractionDigits: 2 })} ${labels[unit] ?? unit}`;
}
