"use client";
import { z } from "zod";
import { createApiClient } from "./api/client";
import { authClient, authParams, withAuthLock } from "./auth/session";
import { requireAccount } from "./profile";
import { planWindow, planInput, planItem, planAvailability } from "./planning";
export const routineAction = z.object({
  key: z.string().min(1).max(64),
  title: z.string().trim().min(1).max(200),
  minutes: z.number().int().min(0).max(1440),
  priority: z.enum(["low", "normal", "high"]),
  start: z.string(),
  end: z.string(),
});
export const routineDay = z.object({
  day: z.number().int().min(1).max(7),
  available_minutes: z.number().int().min(0).max(1440),
  windows: z.array(planWindow).max(8),
  rest: z.boolean(),
  actions: z.array(routineAction).max(20),
});
export const routineInput = z.object({
  title: z.string().trim().min(1).max(200),
  period: z.enum(["day", "week"]),
  days: z.array(routineDay).min(1).max(7),
});
export const routineSchema = routineInput.extend({ id: z.string(), version: z.number().int() });
export const routinePreview = z.object({
  template: routineSchema,
  start_date: z.string(),
  hash: z.string(),
  days: z.array(
    z.object({
      date: z.string(),
      timezone: z.string(),
      items: z.array(planItem),
      plan_version: z.number().int(),
      already_applied: z.boolean(),
      past: z.boolean(),
      confirmed: z.boolean(),
      input: planInput,
      actions: z.array(routineAction),
      availability: planAvailability,
    }),
  ),
});
export type Routine = z.infer<typeof routineSchema>;
export type RoutinePreview = z.infer<typeof routinePreview>;
export async function listRoutines(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/templates/routines", { signal });
  return z.array(routineSchema).parse(r.data?.data);
}
export async function saveRoutine(
  current: Routine,
  input: z.infer<typeof routineInput>,
  copy: boolean,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const header = {
      ...authParams().header,
      "Idempotency-Key": key,
      "If-Match": '"' + current.version + '"',
    };
    const r = copy
      ? await api.POST("/api/v1/templates/routines", { params: { header }, body: input })
      : await api.PATCH("/api/v1/templates/routines/{id}", {
          params: { path: { id: current.id }, header },
          body: input,
        });
    return routineSchema.parse(r.data?.data);
  });
}
export async function previewRoutine(id: string, start_date: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/routines/{id}/preview", {
      params: { path: { id }, ...authParams() },
      body: { start_date },
    });
    return routinePreview.parse(r.data?.data);
  });
}
export async function applyRoutine(
  preview: RoutinePreview,
  reason: string,
  accept_conflicts: boolean,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/routines/{id}/apply", {
      params: {
        path: { id: preview.template.id },
        header: { ...authParams().header, "Idempotency-Key": key },
      },
      body: {
        start_date: preview.start_date,
        preview_hash: preview.hash,
        reason,
        accept_conflicts,
      },
    });
    return routinePreview.parse(r.data?.data);
  });
}
