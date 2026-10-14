"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { dayPlan, planInput } from "@/lib/planning";
import type { components } from "@/lib/api/schema";
type S = components["schemas"];
const text = z.string(),
  integer = z.number().int(),
  number = z.number(),
  bool = z.boolean();
export const totals = z.object({
  planned: integer,
  planned_completed: integer,
  planned_failed: integer,
  planned_skipped: integer,
  planned_pending: integer,
  on_time: integer,
  late: integer,
  no_deadline: integer,
  failed: integer,
  skipped: integer,
  postponed: integer,
  postponement_events: integer,
  habit_obligations: integer,
  habit_successes: integer,
  habit_missed: integer,
  habit_excluded: integer,
  goals_achieved: integer,
  goals_not_achieved: integer,
  goals_cancelled: integer,
});
const timeTotals = z.object({
  estimated_tasks: integer,
  actual_known_tasks: integer,
  comparable_tasks: integer,
  comparable_estimated_seconds: integer,
  comparable_actual_seconds: integer,
});
export const source = z.object({
  date: text,
  qualifier: text,
  kind: text,
  id: text,
  name: text,
  url: text.startsWith("/app/"),
  at: text,
  value: number,
  unit: text,
  area_id: text.nullable(),
  version: integer,
});
export const taskTime = z.object({
  id: text,
  title: text,
  url: text.startsWith("/app/"),
  estimated_minutes: integer.nullable(),
  actual_seconds: integer.nullable(),
  focus_seconds: integer,
  manual_minutes: integer.nullable(),
  mode: text,
  confirmed_sessions: integer,
  completed_at: text,
  template_id: text.nullable(),
  template_name: text.nullable(),
  template_version: integer.nullable(),
  area_id: text.nullable(),
});
export const rollingScore = z.object({
  from: text,
  to: text,
  score: integer.nullable(),
  active_days: integer,
  task_obligations: integer,
  habit_obligations: integer,
  dates: z.array(text),
  state: text,
});
export const scores = z.object({
  version: text,
  state: text,
  closed: bool,
  t: number.nullable(),
  h: number.nullable(),
  r: number.nullable(),
  daily: integer.nullable(),
  discipline_day: number.nullable(),
  discipline: rollingScore,
});
export const analyticsDay = totals.extend({
  ...timeTotals.shape,
  date: text,
  completed: integer,
  focus_seconds: integer,
  xp_delta: integer,
  coins_delta: integer,
  sources: z.array(source),
  plan: z
    .object({
      confirmed: bool,
      timezone: text,
      version: integer,
      input: planInput,
      snapshot: dayPlan.shape.snapshot,
    })
    .nullable(),
  task_weight_total: integer,
  task_weight_completed: integer,
  habits_complete: bool,
  habits: z.array(
    z.object({
      id: text,
      period_id: text,
      name: text,
      unit: text,
      url: text.startsWith("/app/"),
      timezone: text,
      start: text,
      end: text,
      amount: number,
      target: number,
      excluded: bool,
      closed: bool,
      area_id: text.nullable(),
    }),
  ),
  scores,
  task_times: z.array(taskTime),
});
export const analyticsWeek = totals.extend({
  ...timeTotals.shape,
  start: text,
  end: text,
  days: integer,
  completed: integer,
  focus_seconds: integer,
  xp_delta: integer,
  coins_delta: integer,
});
export const analyticsView = z.object({
  from: text,
  to: text,
  timezone: text,
  projection_version: text,
  source_revision: integer,
  projected_revision: integer,
  computed_at: text.nullable(),
  stale: bool,
  days: z.array(analyticsDay),
  weeks: z.array(analyticsWeek),
  definitions: z.array(
    z.object({ id: text, label: text, numerator: text, denominator: text, basis: text }),
  ),
});
export const templateSample = z.object({
  id: text,
  name: text,
  version: integer,
  count: integer,
  average_estimated_minutes: number.nullable(),
  average_actual_seconds: number.nullable(),
  factor: number.nullable(),
  sufficient: bool,
  quest_ids: z.array(text),
});
export type AnalyticsDay = z.infer<typeof analyticsDay>;
export type AnalyticsView = z.infer<typeof analyticsView>;
export type AnalyticsRange = S["AnalyticsRange"];
export async function getAnalytics(range: AnalyticsRange, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics", { params: { query: range }, signal });
  return analyticsView.parse(r.data?.data);
}
export async function rebuildAnalytics(range: AnalyticsRange) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/analytics/rebuild", { params: authParams(), body: range });
    return analyticsView.parse(r.data?.data);
  });
}
export async function getTemplateSamples(range: AnalyticsRange, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics/templates", {
    params: { query: range },
    signal,
  });
  return z
    .object({ ...rangeSchema, timezone: text, computed_at: text, items: z.array(templateSample) })
    .parse(r.data?.data);
}
const rangeSchema = { from: text, to: text };
export const scoreStates: Record<string, string> = {
  scored: "Завершённый день",
  in_progress: "День ещё идёт",
  rest: "Отдых",
  no_plan: "Нет плана",
  future: "Будущий день",
  recalculating: "Пересчитываем привычки",
  insufficient_data: "Недостаточно данных",
  unavailable_timezone: "Часовой пояс недоступен",
  invalid_date: "Дата недоступна",
};
export const balanceAxis = z.object({
  id: text,
  name: text,
  archived: bool,
  metric: z.enum(["tasks", "focus_minutes", "habit_norms"]),
  unit: text,
  target: number.nullable(),
  actual: number.nullable(),
  percent: number.nullable(),
  version: integer,
  sources: z.array(source),
});
export const balanceView = z.object({
  ...rangeSchema,
  timezone: text,
  computed_at: text.nullable(),
  stale: bool,
  source_revision: integer,
  projected_revision: integer,
  axes: z.array(balanceAxis),
});
export type BalanceAxis = z.infer<typeof balanceAxis>;
export async function getBalance(range: AnalyticsRange, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics/balance", {
    params: { query: range },
    signal,
  });
  return balanceView.parse(r.data?.data);
}
export async function setBalanceTarget(
  area_id: string,
  version: number,
  body: S["AnalyticsTargetInput"],
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/analytics/targets/{area_id}", {
        params: {
          path: { area_id },
          header: { ...authParams().header, "If-Match": `"${version}"`, "Idempotency-Key": key },
        },
        body,
      });
    return z
      .object({
        ...rangeSchema,
        area_id: text,
        metric: text,
        target: number,
        version: integer,
        updated_at: text,
      })
      .parse(r.data?.data);
  });
}
export const sourceKinds: Record<string, string> = {
  task: "Завершение задачи",
  planned: "Обязательство плана",
  fail: "Провал",
  skip: "Пропуск",
  postponement: "Перенос",
  habit: "Норма привычки",
  focus: "Рабочий фокус",
  xp: "Изменение опыта",
  coins: "Изменение монет",
  goal_metric: "Запись метрики цели",
  goal_achieved: "Цель достигнута",
  goal_not_achieved: "Цель закрыта без результата",
  goal_cancelled: "Цель отменена",
};
