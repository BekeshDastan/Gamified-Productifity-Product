"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { planInput, dayPlan } from "@/lib/planning";
import { calendarEvent } from "@/lib/calendar";
import type { components } from "@/lib/api/schema";
type S = components["schemas"];
const text = z.string(),
  int = z.number().int(),
  bool = z.boolean(),
  interval = z.object({ start: text, end: text });
const option = z.object({ id: text, allow_split: bool, five_minute_start: bool });
export const scheduleInput = z.object({
  from: text,
  to: text,
  tasks: z.array(option),
  breaks: z.array(interval),
  between_minutes: int,
});
const task = z.object({
  id: text,
  version: int,
  title: text,
  minutes: int,
  priority: text,
  created_at: text,
  deadline: text.nullable(),
  allow_split: bool,
  five_minute_start: bool,
  excluded_reason: text,
});
export const schedulePreview = z.object({
  id: text,
  input: scheduleInput,
  timezone: text,
  created_at: text,
  expires_at: text,
  input_versions: z.array(z.object({ kind: text, id: text, version: int })),
  hash: text,
  tasks: z.array(task),
  days: z.array(
    z.object({
      date: text,
      plan_version: int,
      input: planInput,
      timezone: text,
      blocks: z.array(calendarEvent),
    }),
  ),
  placements: z.array(
    z.object({
      task_id: text,
      task_version: int,
      title: text,
      start: text,
      end: text,
      break_end: text,
      minutes: int,
      partial_start: bool,
    }),
  ),
  unscheduled: z.array(
    z.object({
      task_id: text,
      reason: text,
      required_minutes: int,
      free_minutes: int,
      deficit_minutes: int,
    }),
  ),
  warnings: z.array(text),
});
export type ScheduleInput = z.infer<typeof scheduleInput>;
export type SchedulePreview = z.infer<typeof schedulePreview>;
export const scheduleResult = z.object({
  operation_id: text,
  preview_id: text,
  applied_at: text,
  blocks: z.array(calendarEvent),
  plans: z.array(dayPlan),
});
export const radar = z.object({
  timezone: text,
  computed_at: text,
  notice: text,
  buckets: z.array(
    z.object({
      id: text,
      label: text,
      before: text,
      required_minutes: int,
      cumulative_minutes: int,
      free_minutes: int,
      deficit_minutes: int,
      unknown_estimates: int,
      items: z.array(
        z.object({
          id: text,
          title: text,
          version: int,
          deadline: text,
          estimated_minutes: int.nullable(),
          url: text,
        }),
      ),
    }),
  ),
});
export async function getRadar(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/plans/deadline-radar", { signal });
  return radar.parse(r.data?.data);
}
export async function previewSchedule(body: S["ScheduleInput"]) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    const r = await a.POST("/api/v1/plans/schedule/preview", { params: authParams(), body });
    return schedulePreview.parse(r.data?.data);
  });
}
export async function applySchedule(body: S["ScheduleApply"]) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    const r = await a.POST("/api/v1/plans/schedule/apply", { params: authParams(), body });
    return scheduleResult.parse(r.data?.data);
  });
}
