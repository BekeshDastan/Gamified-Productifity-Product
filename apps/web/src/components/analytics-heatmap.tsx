"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DateTime } from "luxon";
import * as api from "@/lib/analytics";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { Card } from "./ui/card";
export function AnalyticsHeatmap({
  account,
  timezone,
  choose,
}: {
  account: string;
  timezone: string;
  choose: (date: string) => void;
}) {
  const currentYear = DateTime.now().setZone(timezone).year,
    [year, setYear] = useState(currentYear),
    [enabled, setEnabled] = useState(false),
    from = DateTime.fromObject({ year, month: 1, day: 1 }),
    to = from.plus({ years: 1 }),
    valid = year >= 1 && year <= 9997;
  const q = useQuery({
    queryKey: ["account", account, "analytics-year", timezone, year],
    queryFn: ({ signal }) =>
      api.getAnalytics({ from: from.toISODate()!, to: to.toISODate()! }, signal),
    enabled: enabled && valid,
    refetchInterval: enabled ? 10000 : false,
  });
  const offset = from.weekday - 1;
  return (
    <Card>
      <h2>Год в календаре</h2>
      <p>
        Цвет показывает оценку собственного плана. Отдых, нулевая оценка и отсутствие плана
        различаются. Нажмите день, чтобы открыть реальные записи; будущие дни не получают оценку.
      </p>
      <Input
        type="number"
        label="Год тепловой карты"
        value={year}
        min={1}
        max={9997}
        onChange={(e) => setYear(Number(e.target.value))}
      />
      <Button variant="secondary" disabled={!valid} onClick={() => setEnabled(!enabled)}>
        {enabled ? "Скрыть карту года" : "Показать карту года"}
      </Button>
      {enabled && q.isPending && <p role="status">Собираем год…</p>}
      {enabled && q.isError && <p role="alert">Карта не загрузилась. Повторите открытие.</p>}
      {enabled && q.data && (
        <>
          <p role="status">
            {q.data.stale
              ? "Карта пересчитывается, показана предыдущая версия."
              : "Карта актуальна."}{" "}
            Столбцы — недели, строки — понедельник–воскресенье. Слева январь, справа декабрь.
          </p>
          <div
            className="analytics-heatmap-scroll"
            role="region"
            aria-label={`Карта дней за ${year} год`}
            tabIndex={0}
          >
            <div className="analytics-year-grid">
              {Array.from({ length: offset }, (_, i) => (
                <span key={"gap" + i} aria-hidden="true" />
              ))}
              {q.data.days.map((d) => (
                <button
                  key={d.date}
                  type="button"
                  className={`analytics-heat-cell ${cellClass(d)}`}
                  title={dayLabel(d)}
                  aria-label={dayLabel(d)}
                  onClick={() => choose(d.date)}
                />
              ))}
            </div>
          </div>
          <ul className="analytics-heat-legend" aria-label="Легенда карты">
            {[
              ["empty", "Нет плана или оценки"],
              ["future", "Будущий день"],
              ["rest", "Отдых"],
              ["zero", "Нулевой балл"],
              ["level-1", "1–24"],
              ["level-2", "25–49"],
              ["level-3", "50–74"],
              ["level-4", "75–100"],
            ].map(([style, label]) => (
              <li key={style}>
                <span className={`analytics-heat-cell ${style}`} aria-hidden="true" />
                {label}
              </li>
            ))}
          </ul>
          <details>
            <summary>Таблица-альтернатива годовой карты</summary>
            <div
              className="analytics-table-scroll"
              role="region"
              aria-label="Дни выбранного года"
              tabIndex={0}
            >
              <table className="analytics-table">
                <caption>Все значения карты {year} года</caption>
                <thead>
                  <tr>
                    <th>Дата</th>
                    <th>Оценка плана</th>
                    <th>Состояние</th>
                    <th>Отмечено задач</th>
                    <th>Фокус, сек</th>
                    <th>Записи</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.days.map((d) => (
                    <tr key={d.date}>
                      <th scope="row">{d.date}</th>
                      <td>{d.scores.daily ?? "—"}</td>
                      <td>{api.scoreStates[d.scores.state] ?? "Нет оценки"}</td>
                      <td>{d.completed}</td>
                      <td>{d.focus_seconds}</td>
                      <td>
                        <Button size="small" variant="secondary" onClick={() => choose(d.date)}>
                          Открыть {d.date}
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </details>
        </>
      )}
    </Card>
  );
}
function cellClass(d: api.AnalyticsDay) {
  if (d.scores.state === "rest") return "rest";
  if (d.scores.state === "future") return "future";
  if (d.scores.daily === null) return "empty";
  if (d.scores.daily === 0) return "zero";
  return `level-${Math.min(4, Math.floor(d.scores.daily / 25) + 1)}`;
}
function dayLabel(d: api.AnalyticsDay) {
  return `${d.date}: ${api.scoreStates[d.scores.state] ?? "Нет оценки"}; оценка ${d.scores.daily ?? "отсутствует"}; отмечено задач ${d.completed}; фокус ${d.focus_seconds} секунд`;
}
