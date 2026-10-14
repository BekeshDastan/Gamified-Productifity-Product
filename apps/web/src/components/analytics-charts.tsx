"use client";
import { useState } from "react";
import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { DateTime } from "luxon";
import type { AnalyticsDay } from "@/lib/analytics";
import { scoreStates } from "@/lib/analytics";
import { Button } from "./ui/button";
import { Select } from "./ui/field";
import { Card } from "./ui/card";
import { ChoiceChips, useMotionAllowed } from "./ui/experience";
const modes = {
  tasks: "Завершение и сроки",
  outcomes: "Провалы, пропуски, переносы",
  habits: "Полные нормы привычек",
  scores: "Оценка дня",
  time: "Оценка и фактическое время",
  focus: "Подтверждённая работа",
  xp: "Опыт",
  coins: "Монеты",
};
type Mode = keyof typeof modes;
export function AnalyticsChart({
  days,
  choose,
}: {
  days: AnalyticsDay[];
  choose: (date: string, filter?: string) => void;
}) {
  const motion = useMotionAllowed();
  const [mode, setMode] = useState<Mode>("tasks"),
    rows = days.map((d) => ({
      ...d,
      daily_score: d.scores.daily,
      estimate_minutes: d.comparable_tasks ? d.comparable_estimated_seconds / 60 : null,
      actual_minutes: d.comparable_tasks ? d.comparable_actual_seconds / 60 : null,
      focus_minutes: d.focus_seconds / 60,
    }));
  const metric =
    mode === "time" || mode === "focus"
      ? "минут"
      : mode === "scores"
        ? "баллов"
        : mode === "habits"
          ? "периодов"
          : mode === "xp"
            ? "XP"
            : mode === "coins"
              ? "монет"
              : "задач";
  const series =
    mode === "tasks"
      ? [
          { key: "on_time", name: "Вовремя", color: "var(--success)", filter: "on_time" },
          { key: "late", name: "Поздно", color: "var(--warning)", filter: "late" },
          { key: "no_deadline", name: "Без срока", color: "var(--primary)", filter: "no_deadline" },
        ]
      : mode === "outcomes"
        ? [
            { key: "failed", name: "Провал", color: "var(--destructive)", filter: "fail" },
            { key: "skipped", name: "Пропуск", color: "var(--info)", filter: "skip" },
            {
              key: "postponed",
              name: "Перенесённые задачи",
              color: "var(--warning)",
              filter: "postponement",
            },
          ]
        : mode === "habits"
          ? [
              {
                key: "habit_successes",
                name: "Полная норма",
                color: "var(--success)",
                filter: "habit",
              },
              {
                key: "habit_missed",
                name: "Закрытые неполные нормы",
                color: "var(--warning)",
                filter: "habit",
              },
            ]
          : mode === "time"
            ? [
                {
                  key: "estimate_minutes",
                  name: "Оценка сопоставимых задач",
                  color: "var(--primary)",
                  filter: "time",
                },
                {
                  key: "actual_minutes",
                  name: "Факт тех же задач",
                  color: "var(--info)",
                  filter: "time",
                },
              ]
            : mode === "focus"
              ? [
                  {
                    key: "focus_minutes",
                    name: "Подтверждённая работа",
                    color: "var(--primary)",
                    filter: "focus",
                  },
                ]
              : mode === "xp"
                ? [
                    {
                      key: "xp_delta",
                      name: "Изменение опыта",
                      color: "var(--primary)",
                      filter: "xp",
                    },
                  ]
                : mode === "coins"
                  ? [
                      {
                        key: "coins_delta",
                        name: "Изменение монет",
                        color: "var(--warning)",
                        filter: "coins",
                      },
                    ]
                  : [];
  return (
    <Card className="analytics-chart-card">
      <h2>Динамика по дням</h2>
      <ChoiceChips
        label="Быстрый выбор показателя"
        value={mode}
        options={Object.entries(modes).map(([value, label]) => ({ value: value as Mode, label }))}
        onChange={setMode}
      />
      <details className="quiet-details">
        <summary>Показатель и пояснение графика</summary>
        <Select
          label="Показатель графика"
          value={mode}
          onChange={(e) => setMode(e.target.value as Mode)}
        >
          {Object.entries(modes).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <p>Единица: {metric}. Нажмите на день графика или откройте его источники в таблице ниже.</p>
        {mode === "tasks" && (
          <p>
            Вовремя + поздно + без срока = все завершённые листовые задачи. Просрочка и провал
            считаются отдельно.
          </p>
        )}
        {mode === "time" && (
          <p>
            Сравниваются только завершённые задачи с известной оценкой и фактом. Длительность
            относится к завершённой задаче; рабочий фокус по календарным дням показан отдельно.
          </p>
        )}
        {mode === "scores" && (
          <p>
            Разрыв линии означает отдых или отсутствие оценки. Открытый день предварителен;
            невыполненный план может иметь реальный нулевой балл.
          </p>
        )}
      </details>
      <div className="analytics-chart" role="group" aria-label={`График: ${modes[mode]}`}>
        <ResponsiveContainer
          width="100%"
          height={300}
          minWidth={0}
          initialDimension={{ width: 600, height: 300 }}
        >
          <ComposedChart
            data={rows}
            accessibilityLayer
            onClick={(state) => {
              if (typeof state.activeLabel === "string") choose(state.activeLabel);
            }}
            margin={{ top: 12, right: 12, bottom: 12, left: 0 }}
          >
            <CartesianGrid stroke="var(--border)" strokeDasharray="3 3" />
            <XAxis
              dataKey="date"
              tickFormatter={(v) => DateTime.fromISO(String(v)).toFormat("dd.LL")}
              tick={{ fill: "var(--muted-foreground)", fontSize: 12 }}
              minTickGap={24}
            />
            <YAxis
              allowDecimals={mode === "time" || mode === "focus"}
              domain={mode === "scores" ? [0, 100] : ["auto", "auto"]}
              tick={{ fill: "var(--muted-foreground)", fontSize: 12 }}
              width={45}
            />
            <Tooltip
              contentStyle={{
                background: "var(--card)",
                color: "var(--foreground)",
                border: "1px solid var(--border)",
              }}
              labelFormatter={(value) => String(value)}
              formatter={(value) =>
                value == null
                  ? "Нет данных"
                  : typeof value === "number"
                    ? value.toLocaleString("ru-RU", { maximumFractionDigits: 2 })
                    : String(value)
              }
            />
            <Legend wrapperStyle={{ color: "var(--foreground)", fontSize: 12 }} />
            {mode === "scores" ? (
              <Line
                dataKey="daily_score"
                name="Оценка собственного плана"
                stroke="var(--primary)"
                strokeWidth={2}
                type="linear"
                connectNulls={false}
                isAnimationActive={motion}
                animationDuration={600}
                dot={{ r: 3 }}
              />
            ) : (
              series.map((s) => (
                <Bar
                  key={s.key}
                  dataKey={s.key}
                  name={s.name}
                  fill={s.color}
                  isAnimationActive={motion}
                  animationDuration={600}
                  maxBarSize={32}
                  onClick={(item, _index, event) => {
                    event.stopPropagation();
                    const date = item.payload?.date;
                    if (typeof date === "string") choose(date, s.filter);
                  }}
                />
              ))
            )}
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <details className="quiet-details">
        <summary>Таблица-альтернатива графика</summary>
        <div
          className="analytics-table-scroll"
          role="region"
          aria-label="Показатели по дням"
          tabIndex={0}
        >
          <table className="analytics-table">
            <caption>Те же значения, что на графике «{modes[mode]}»</caption>
            <thead>
              <tr>
                <th scope="col">Дата</th>
                {mode === "scores" ? (
                  <>
                    <th scope="col">Оценка</th>
                    <th scope="col">Состояние</th>
                  </>
                ) : (
                  series.map((s) => (
                    <th key={s.key} scope="col">
                      {s.name}, {metric}
                    </th>
                  ))
                )}
                <th scope="col">Записи</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((d) => (
                <tr key={d.date}>
                  <th scope="row">{d.date}</th>
                  {mode === "scores" ? (
                    <>
                      <td>{d.scores.daily ?? "—"}</td>
                      <td>{scoreStates[d.scores.state] ?? "Нет оценки"}</td>
                    </>
                  ) : (
                    series.map((s) => <td key={s.key}>{displayValue(d, s.key)}</td>)
                  )}
                  <td>
                    <Button
                      size="small"
                      variant="secondary"
                      onClick={() =>
                        choose(
                          d.date,
                          mode === "time"
                            ? "time"
                            : mode === "tasks"
                              ? "task"
                              : mode === "habits"
                                ? "habit"
                                : mode === "scores"
                                  ? "planned"
                                  : mode === "outcomes"
                                    ? ""
                                    : mode,
                        )
                      }
                    >
                      Источники {d.date}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </Card>
  );
}
function displayValue(row: Record<string, unknown>, key: string) {
  const value = row[key];
  return typeof value === "number"
    ? value.toLocaleString("ru-RU", { maximumFractionDigits: 2 })
    : "Нет замера";
}
