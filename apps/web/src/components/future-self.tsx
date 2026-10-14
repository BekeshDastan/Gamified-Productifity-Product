"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { requireAccount } from "@/lib/profile";
import { createApiClient } from "@/lib/api/client";
import { source } from "@/lib/analytics";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Select } from "./ui/field";
import { Button } from "./ui/button";
const text = z.string(),
  integer = z.number().int();
const schema = z.object({
  timezone: text,
  computed_at: text,
  horizon_weeks: integer,
  observed_weeks: integer,
  available: z.boolean(),
  average_weekly_hours: z.number().nullable(),
  projected_hours: z.number().nullable(),
  text,
  weeks: z.array(
    z.object({
      from: text,
      to: text,
      observed: z.boolean(),
      focus_seconds: integer.nullable(),
      sources: z.array(source),
    }),
  ),
});
async function getFuture(weeks: number, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics/future-self", {
    params: { query: { weeks } },
    signal,
  });
  return schema.parse(r.data?.data);
}
export function FutureSelf() {
  const [weeks, setWeeks] = useState(12),
    account = useAuth().session?.account.id,
    data = useQuery({
      queryKey: ["account", account, "future-self", weeks],
      queryFn: ({ signal }) => getFuture(weeks, signal),
      enabled: !!account,
    });
  if (!account) return null;
  return (
    <Card>
      <h2>Future Self: объём практики</h2>
      <p>
        Среднее подтверждённых рабочих часов за последние четыре полные недели, включая нулевые. Для
        расчёта нужны минимум две полные недели наблюдений. Текущая неделя исключена.
      </p>
      <Select
        label="Горизонт Future Self"
        value={weeks}
        onChange={(e) => setWeeks(Number(e.target.value))}
      >
        {[4, 12, 26, 52, 104].map((w) => (
          <option key={w} value={w}>
            {w} недель
          </option>
        ))}
      </Select>
      {data.isPending && <p role="status">Считаем подтверждённые часы…</p>}
      {data.isError && (
        <p role="alert">
          Расчёт недоступен. <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      {data.data && (
        <>
          <p>{data.data.text}</p>
          <p>
            Полных наблюдавшихся недель: {data.data.observed_weeks}. Зона: {data.data.timezone}.
          </p>
          {data.data.available && (
            <p>
              Средний темп:{" "}
              {data.data.average_weekly_hours?.toLocaleString("ru-RU", {
                maximumFractionDigits: 2,
              })}{" "}
              ч/неделю × {data.data.horizon_weeks} ={" "}
              {data.data.projected_hours?.toLocaleString("ru-RU", { maximumFractionDigits: 2 })} ч.
            </p>
          )}
          <details>
            <summary>Недели и источники Future Self</summary>
            {data.data.weeks.map((w) => (
              <section key={w.from}>
                <h3>
                  {w.from} — {w.to} (конец не включён)
                </h3>
                <p>
                  {w.focus_seconds === null
                    ? "Нет полной недели наблюдений"
                    : `${(w.focus_seconds / 3600).toLocaleString("ru-RU", { maximumFractionDigits: 2 })} подтверждённых часов`}
                </p>
                <ul>
                  {w.sources.map((s, i) => (
                    <li key={i}>
                      <SafeLink href={s.url}>Подтверждённая сессия</SafeLink> · {s.value} секунд ·{" "}
                      {s.date}
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </details>
        </>
      )}
    </Card>
  );
}
