"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
const text = z.string(),
  integer = z.number().int();
export const rule = z.object({
  id: text,
  version: integer,
  priority: integer,
  conditions: z.array(z.object({ metric: text, op: text, value: z.number() })),
  template_key: text,
  action_type: text,
  cooldown_seconds: integer,
  enabled: z.boolean(),
});
export const setting = z.object({ rule_id: text, disabled: z.boolean(), version: integer });
export const evidence = z.object({ kind: text, id: text, version: integer, url: text });
export const card = z.object({
  id: text,
  version: integer,
  created_at: text,
  rule,
  fact: z.object({
    rule_id: text,
    subject: text,
    numbers: z.record(text, z.number()),
    sources: z.array(evidence),
  }),
  message: text,
  action_url: text,
});
export const navigatorPage = z.object({
  items: z.array(card),
  settings: z.array(setting),
  rules: z.array(rule),
  computed_at: text,
  complete: z.boolean(),
});
export type Card = z.infer<typeof card>;
export async function getNavigator(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/navigator", { signal });
  return navigatorPage.parse(r.data?.data);
}
function mutationParams(id: string, version: number) {
  return {
    path: { id },
    header: {
      ...authParams().header,
      "If-Match": `"${version}"`,
      "Idempotency-Key": crypto.randomUUID(),
    },
  };
}
export async function setRule(id: string, disabled: boolean, version: number) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    const r = await a.PUT("/api/v1/navigator/rules/{id}", {
      params: mutationParams(id, version),
      body: { rule_id: id, disabled, version },
    });
    return setting.parse(r.data?.data);
  });
}
export async function decideCard(value: Card, action: "hide" | "snooze", hours = 24) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    const r = await a.POST("/api/v1/navigator/cards/{id}/decision", {
      params: mutationParams(value.id, value.version),
      body: { action, hours },
    });
    return z.object({ id: text, version: integer }).parse(r.data?.data);
  });
}
export async function trackView(id: string, visible: boolean) {
  await requireAccount();
  return withAuthLock(async () => {
    const a = await authClient();
    await a.POST("/api/v1/navigator/task-view/{id}", {
      params: { ...authParams(), path: { id } },
      body: { visible },
    });
  });
}
