"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Radar,
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  ResponsiveContainer,
  Tooltip,
} from "recharts";
import { DateTime } from "luxon";
import * as api from "@/lib/analytics";
import { SafeLink, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { useChallengeAction, useChallengeKey } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
const metrics = {
    tasks: "Завершённые задачи",
    focus_minutes: "Рабочие минуты фокуса",
    habit_norms: "Полные нормы привычек",
  },
  units: Record<string, string> = { tasks: "задач", minutes: "минут", periods: "периодов" };
export function AnalyticsBalance({
  account,
  timezone,
  range,
}: {
  account: string;
  timezone: string;
  range: api.AnalyticsRange;
}) {
  const cache = useQueryClient(),
    q = useQuery({
      queryKey: ["account", account, "analytics-balance", timezone, range],
      queryFn: ({ signal }) => api.getBalance(range, signal),
      refetchInterval: 10000,
    });
  const known = q.data?.axes.filter((a) => a.percent !== null) ?? [],
    rows = known.map((a) => ({
      name: a.name.length > 14 ? a.name.slice(0, 12) + "…" : a.name,
      percent: a.percent,
    }));
  return (
    <Card>
      <h2>Баланс по собственным ориентирам</h2>
      <p>
        Ориентиры относятся только к периоду {range.from} —{" "}
        {DateTime.fromISO(range.to).minus({ days: 1 }).toISODate()}. При смене периода они не
        масштабируются автоматически. Это выполнение ваших ориентиров по отдельным единицам, без
        оценки здоровья, отношений или ценности человека.
      </p>
      {q.isPending && <p role="status">Загружаем направления…</p>}
      {q.isError && <p role="alert">Не удалось загрузить направления. Обновите аналитику.</p>}
      {q.data?.stale && (
        <p role="status">Есть изменения: показатели направлений пересчитываются.</p>
      )}
      {known.length >= 3 ? (
        <>
          <p>
            Оси: min(факт / личный ориентир, 1) × 100%. Показаны только направления с известными
            значениями и положительными ориентирами.
          </p>
          <div
            className="analytics-chart"
            role="group"
            aria-label="Диаграмма выполнения личных ориентиров"
          >
            <ResponsiveContainer
              width="100%"
              height={320}
              minWidth={0}
              initialDimension={{ width: 600, height: 320 }}
            >
              <RadarChart data={rows} accessibilityLayer outerRadius="65%">
                <PolarGrid stroke="var(--border)" />
                <PolarAngleAxis dataKey="name" tick={{ fill: "var(--foreground)", fontSize: 11 }} />
                <PolarRadiusAxis
                  domain={[0, 100]}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                />
                <Radar
                  name="Выполнение ориентира, %"
                  dataKey="percent"
                  stroke="var(--primary)"
                  fill="var(--primary)"
                  fillOpacity={0.25}
                  isAnimationActive={false}
                />
                <Tooltip
                  contentStyle={{
                    background: "var(--card)",
                    color: "var(--foreground)",
                    border: "1px solid var(--border)",
                  }}
                />
              </RadarChart>
            </ResponsiveContainer>
          </div>
        </>
      ) : (
        <p>
          Для диаграммы нужно хотя бы три заданных ориентира с известным фактом. Таблица доступна
          для любого числа направлений.
        </p>
      )}
      {q.data && (
        <>
          <div
            className="analytics-table-scroll"
            role="region"
            aria-label="Личные ориентиры и фактические значения"
            tabIndex={0}
          >
            <table className="analytics-table">
              <caption>
                Отдельная единица каждого направления; отсутствие ориентира не заменяется нулевым
                баллом
              </caption>
              <thead>
                <tr>
                  <th>Направление</th>
                  <th>Показатель</th>
                  <th>Факт</th>
                  <th>Ориентир</th>
                  <th>Выполнение</th>
                </tr>
              </thead>
              <tbody>
                {q.data.axes.map((a) => (
                  <tr key={a.id}>
                    <th scope="row">
                      {a.name}
                      {a.archived ? " · В архиве" : ""}
                    </th>
                    <td>{metrics[a.metric]}</td>
                    <td>
                      {a.actual === null
                        ? "Пересчитывается"
                        : a.actual.toLocaleString("ru-RU", { maximumFractionDigits: 2 })}{" "}
                      {units[a.unit]}
                    </td>
                    <td>
                      {a.target === null || a.target <= 0
                        ? "Не задан"
                        : a.target.toLocaleString("ru-RU") + " " + units[a.unit]}
                    </td>
                    <td>{a.percent === null ? "Нет оценки" : a.percent.toFixed(1) + "%"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {q.data.axes.map((a) => (
            <TargetRow
              key={a.id + ":" + range.from + ":" + range.to}
              axis={a}
              range={range}
              saved={() => void cache.invalidateQueries({ queryKey: ["account", account] })}
            />
          ))}
          {!q.data.axes.length && (
            <p>
              Сначала добавьте свои направления.{" "}
              <SafeLink href="/app/settings/areas">Настроить направления</SafeLink>
            </p>
          )}
        </>
      )}
    </Card>
  );
}
function TargetRow({
  axis,
  range,
  saved,
}: {
  axis: api.BalanceAxis;
  range: api.AnalyticsRange;
  saved: () => void;
}) {
  const [base, setBase] = useState(axis),
    [metric, setMetric] = useState(axis.metric),
    [target, setTarget] = useState(axis.target?.toString() ?? ""),
    [dirty, setDirty] = useState(false),
    [ack, setAck] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(dirty);
  const changed = axis.version !== base.version;
  return (
    <details>
      <summary>{axis.name}: изменить ориентир и открыть записи</summary>
      <p>
        Это ваш выбор, а не рекомендуемая норма. 0 отключает ориентир. Показатели прошлых отчётов
        сохраняют свой снимок.
      </p>
      {changed && (
        <p role="status">
          Ориентир изменился в другом окне. Ваши правки сохранены локально; сначала обновите
          исходное значение.
        </p>
      )}
      <form
        className="form-grid"
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            if (!target.trim() || !Number.isFinite(Number(target)))
              throw Error("Укажите личный ориентир или 0 для отключения.");
            const body = { ...range, metric, target: Number(target), confirm: true };
            const value = await api.setBalanceTarget(
              axis.id,
              base.version,
              body,
              key({ ...body, version: base.version }),
            );
            setBase({ ...base, version: value.version, target: value.target, metric });
            setDirty(false);
            setAck(false);
            saved();
          });
        }}
      >
        <Select
          label={`Показатель: ${axis.name}`}
          value={metric}
          disabled={axis.archived}
          onChange={(e) => {
            setMetric(e.target.value as api.BalanceAxis["metric"]);
            setDirty(true);
            setAck(false);
          }}
        >
          {Object.entries(metrics).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <Input
          type="number"
          min={0}
          max={1000000}
          step={0.001}
          required
          label={`Ориентир: ${axis.name}`}
          value={target}
          disabled={axis.archived}
          placeholder="Пока не задан"
          onChange={(e) => {
            setTarget(e.target.value);
            setDirty(true);
            setAck(false);
          }}
        />
        <label>
          <input
            type="checkbox"
            checked={ack}
            disabled={axis.archived}
            onChange={(e) => setAck(e.target.checked)}
          />{" "}
          Подтверждаю ориентир для направления «{axis.name}» на выбранный период
        </label>
        <Button
          type="submit"
          disabled={!ack || !dirty || changed || axis.archived || action.pending}
        >
          Сохранить ориентир «{axis.name}»
        </Button>
        <Button
          variant="secondary"
          disabled={action.pending}
          onClick={() => {
            if (!dirty || window.confirm("Отменить локальные правки этого ориентира?")) {
              setBase(axis);
              setMetric(axis.metric);
              setTarget(axis.target?.toString() ?? "");
              setDirty(false);
              setAck(false);
            }
          }}
        >
          Обновить исходный ориентир
        </Button>
        {action.error && <p role="alert">{action.error}</p>}
      </form>
      <h3>Записи направления</h3>
      <ul>
        {axis.sources.map((s, i) => (
          <li key={s.id + ":" + i}>
            {s.date}: <SafeLink href={s.url}>{s.name}</SafeLink> · {s.value}{" "}
            {units[s.unit] ?? s.unit}
          </li>
        ))}
      </ul>
      {!axis.sources.length && <p>Подходящих записей в этом периоде нет.</p>}
    </details>
  );
}
