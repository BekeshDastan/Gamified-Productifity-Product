"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const calendarInput = z.object({
  title: z.string().trim().min(1, "Укажите название").max(200),
  area_id: z.uuid().nullable(),
  quest_id: z.uuid().nullable(),
  all_day: z.boolean(),
  start: z.string().min(1),
  end: z.string().min(1),
  timezone: z.string().min(1),
});
export const calendarEvent = calendarInput.extend({
  id: z.string(),
  version: z.number().int(),
  quest_version: z.number().int(),
  source: z.string(),
  status: z.string(),
  cancelled: z.boolean(),
  series_id: z.uuid().nullable(),
  occurrence_key: z.string().nullable(),
  series_version: z.number().int(),
  reason: z.string(),
});
export type CalendarInput = z.infer<typeof calendarInput>;
export type CalendarEvent = z.infer<typeof calendarEvent>;
export async function calendarRange(from: string, to: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/calendar", {
    params: { query: { from, to } },
    signal,
  });
  return z
    .object({ items: z.array(calendarEvent), timezone: z.string(), materializing: z.boolean() })
    .parse(r.data?.data);
}
export async function createEvent(input: CalendarInput, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/calendar/events", {
      params: { header: { ...authParams().header, "Idempotency-Key": key } },
      body: input,
    });
    return calendarEvent.parse(r.data?.data);
  });
}
export const recurrenceRule = z.object({
  frequency: z.enum(["daily", "weekly", "monthly"]),
  interval: z.number().int().min(1).max(52),
  weekdays: z.array(z.number().int().min(1).max(7)),
  until: z.string().nullable(),
  count: z.number().int().min(0).max(10000),
});
export const seriesInput = z.object({
  event: calendarInput,
  rule: recurrenceRule,
  kind: z.enum(["event", "task"]),
});
export const calendarSeries = seriesInput.extend({
  id: z.uuid(),
  version: z.number().int(),
  ends_before: z.string().nullable(),
});
export type SeriesInput = z.infer<typeof seriesInput>;
export type CalendarSeries = z.infer<typeof calendarSeries>;
export async function getSeries(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/calendar/series/{id}", {
    params: { path: { id } },
    signal,
  });
  return calendarSeries.parse(r.data?.data);
}
export async function changeSeries(
  current: CalendarSeries,
  body: {
    action: "split" | "end";
    occurrence_key: string;
    replacement: SeriesInput | null;
    until: string | null;
    reason: string;
  },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.PATCH("/api/v1/calendar/series/{id}", {
      params: {
        path: { id: current.id },
        header: {
          ...authParams().header,
          "If-Match": '"' + current.version + '"',
          "Idempotency-Key": key,
        },
      },
      body,
    });
    return calendarSeries.parse(r.data?.data);
  });
}
export async function createSeries(input: SeriesInput, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/calendar/series", {
      params: { header: { ...authParams().header, "Idempotency-Key": key } },
      body: input,
    });
    return calendarSeries.parse(r.data?.data);
  });
}
export async function editCalendar(
  current: CalendarEvent,
  input: CalendarInput,
  reason: string,
  key: string,
  cancelled = false,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const header = {
      ...authParams().header,
      "If-Match": '"' + current.version + '"',
      "Idempotency-Key": key,
    };
    if (current.source === "quest") {
      await api.PATCH("/api/v1/calendar/quests/{id}", {
        params: { path: { id: current.quest_id! }, header },
        body: { planned_date: input.start.slice(0, 10), reason },
      });
      return;
    }
    await api.PATCH("/api/v1/calendar/events/{id}", {
      params: { path: { id: current.id }, header },
      body: { ...input, reason, quest_version: current.quest_version, cancelled },
    });
  });
}
