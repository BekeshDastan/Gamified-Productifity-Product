"use client";
import { z } from "zod";
import { createApiClient } from "./api/client";
import { requireAccount } from "./profile";
import { dayPlan } from "./planning";
import { questSchema } from "./quests";
import { calendarEvent } from "./calendar";
export const dashboardSchema = z.object({
  date: z.string(),
  timezone: z.string(),
  plan: dayPlan,
  tasks: z.array(questSchema),
  deadlines: z.array(questSchema),
  events: z.array(calendarEvent),
  more_tasks: z.boolean(),
  more_deadlines: z.boolean(),
  more_events: z.boolean(),
  updated_at: z.string(),
});
export async function getDashboard(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/dashboard", { signal });
  return dashboardSchema.parse(r.data?.data);
}
