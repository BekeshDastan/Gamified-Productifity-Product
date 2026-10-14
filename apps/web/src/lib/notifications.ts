"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import type { components } from "@/lib/api/schema";
const text = z.string(),
  integer = z.number().int(),
  time = z.iso.datetime({ offset: true });
export const notificationSchema = z.object({
  id: z.uuid(),
  kind: text,
  source_kind: text,
  source_id: text,
  source_revision: integer,
  event_key: text,
  url: text,
  reminder_id: z.uuid().nullable(),
  title: text,
  message: text,
  created_at: time,
  read_at: time.nullable(),
  version: integer,
  active: z.boolean(),
  muted: z.boolean(),
});
const pageSchema = z.object({
  items: z.array(notificationSchema),
  next: text,
  unread: integer,
  computed_at: time,
});
export type NotificationItem = z.infer<typeof notificationSchema>;
export type Preferences = components["schemas"]["NotificationPreferences"];
export type PreferencesInput = components["schemas"]["NotificationPreferencesInput"];
export type Reminder = components["schemas"]["Reminder"];
export type Delivery = components["schemas"]["NotificationDelivery"];
export async function inbox(before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/notifications", {
    params: { query: { before } },
    signal,
  });
  return pageSchema.parse(r.data?.data);
}
export async function preferences(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/notifications/preferences", { signal });
  if (!r.data) throw Error("Не удалось загрузить настройки.");
  return r.data.data;
}
const headers = (version = 0) => ({
  ...authParams().header,
  "Idempotency-Key": crypto.randomUUID(),
  "If-Match": `"${version}"`,
});
export async function savePreferences(body: PreferencesInput, version: number) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    const r = await a.PUT("/api/v1/notifications/preferences", {
      params: { header: headers(version) },
      body,
    });
    if (!r.data) throw Error("Не удалось сохранить настройки.");
    return r.data.data;
  });
}
export async function mark(n: NotificationItem, read: boolean) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.PATCH("/api/v1/notifications/{id}/read", {
      params: { path: { id: n.id }, header: headers(n.version) },
      body: { read },
    });
  });
}
export async function markAll(seen_before: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.POST("/api/v1/notifications/read-all", {
      params: { header: headers() },
      body: { seen_before },
    });
  });
}
export async function reminders(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/reminders", { signal });
  if (!r.data) throw Error("Не удалось загрузить напоминания.");
  return r.data.data.items;
}
export async function snooze(r: Reminder, minutes: number, urgent_override: boolean) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.POST("/api/v1/reminders/{id}/snooze", {
      params: { path: { id: r.id }, header: headers(r.version) },
      body: { minutes, urgent_override, confirm: true },
    });
  });
}
export async function deliveries(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/notifications/{id}/deliveries", {
    params: { path: { id } },
    signal,
  });
  if (!r.data) throw Error("Не удалось загрузить отправки.");
  return r.data.data.items;
}
export async function retry(d: Delivery, ack: boolean) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.POST("/api/v1/notification-deliveries/{id}/retry", {
      params: { path: { id: d.id }, header: headers(d.version) },
      body: { confirm: true, acknowledge_duplicates: ack },
    });
  });
}
export async function pushDevices(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/notifications/push", { signal });
  if (!r.data) throw Error("Не удалось загрузить устройства.");
  return r.data.data;
}
export async function revokeDevice(id: string, version: number) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.POST("/api/v1/notifications/push/{id}/revoke", {
      params: { path: { id }, header: headers(version) },
      body: { confirm: true },
    });
  });
}
