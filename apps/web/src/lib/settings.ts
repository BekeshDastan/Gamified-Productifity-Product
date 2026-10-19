"use client";
import { z } from "zod";
import { DateTime } from "luxon";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { requireAccount } from "@/lib/profile";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";

const time = z.string().regex(/^([01][0-9]|2[0-3]):[0-5][0-9]$/);
export const preferencesSchema = z.object({
  personalization: z.object({
    hidden_widgets: z
      .array(z.enum(["today", "calendar", "habits", "focus", "progress"]))
      .max(5)
      .optional(),
    theme: z.enum(["system", "light", "dark"]),
    motion: z.enum(["system", "reduce"]),
    sound: z.boolean(),
    game_mode: z.enum(["calm", "standard"]),
    widget_order: z
      .array(z.enum(["today", "calendar", "habits", "focus", "progress"]))
      .length(5)
      .refine((value) => new Set(value).size === 5, "Виджеты не должны повторяться"),
  }),
  privacy: z.object({
    profile_visibility: z.enum(["private", "friends"]),
    share_activity: z.boolean(),
    share_achievements: z.boolean(),
    discoverable: z.boolean(),
  }),
  timezone: z
    .string()
    .max(100)
    .refine((zone) => {
      try {
        new Intl.DateTimeFormat("ru", { timeZone: zone });
        return zone === "UTC" || zone.includes("/");
      } catch {
        return false;
      }
    }, "Выберите IANA-часовой пояс"),
  language: z.enum(["ru", "en"]),
  week_start: z.union([z.literal(1), z.literal(7)]),
  time_format: z.enum(["24h", "12h"]),
  work_intervals: z
    .array(z.object({ day: z.number().int().min(1).max(7), start: time, end: time }))
    .max(28),
  rest_days: z.array(z.number().int().min(1).max(7)).max(7),
});
export const settingsSchema = preferencesSchema.extend({
  version: z.number().int().positive(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
export type Settings = z.infer<typeof settingsSchema>;
export type Preferences = z.infer<typeof preferencesSchema>;
export async function getSettings(signal?: AbortSignal) {
  await requireAccount();
  const result = await createApiClient().GET("/api/v1/me/settings", { signal });
  return settingsSchema.parse(result.data?.data);
}
// Fresh server time and current account zone keep "today" consistent with the
// dashboard even when the device clock is wrong or the tab crossed midnight.
export async function getAccountToday() {
  await requireAccount();
  const result = await createApiClient().GET("/api/v1/me/settings");
  const settings = settingsSchema.parse(result.data?.data);
  const instant = z.iso.datetime({ offset: true }).parse(result.data?.meta.server_time);
  return DateTime.fromISO(instant).setZone(settings.timezone).toISODate()!;
}
export function useSettings() {
  const auth = useAuth();
  const id = auth.session?.account.id;
  return useQuery({
    queryKey: ["account", id, "settings"],
    enabled: !!id,
    queryFn: ({ signal }) => getSettings(signal),
  });
}
export async function saveSettings(input: Preferences, version: number) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const response = await api.PATCH("/api/v1/me/settings", {
      params: { header: { ...authParams().header, "If-Match": `"${version}"` } },
      body: input,
    });
    return settingsSchema.parse(response.data?.data);
  });
}
export function formatMoment(
  value: string,
  settings: Pick<Preferences, "language" | "timezone" | "time_format">,
) {
  return new Intl.DateTimeFormat(settings.language === "en" ? "en-US" : "ru-RU", {
    timeZone: settings.timezone,
    dateStyle: "medium",
    timeStyle: "short",
    hour12: settings.time_format === "12h",
  }).format(new Date(value));
}
