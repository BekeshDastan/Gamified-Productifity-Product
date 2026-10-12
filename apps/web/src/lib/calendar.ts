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
});
export type CalendarInput = z.infer<typeof calendarInput>;
export type CalendarEvent = z.infer<typeof calendarEvent>;
export async function calendarRange(from: string, to: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/calendar", {
    params: { query: { from, to } },
    signal,
  });
  return z.object({ items: z.array(calendarEvent), timezone: z.string() }).parse(r.data?.data);
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
