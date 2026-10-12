"use client";
import { z } from "zod";
import { createApiClient } from "./api/client";
import { authClient, authParams, withAuthLock } from "./auth/session";
import { requireAccount } from "./profile";
import { goalInput, goalSchema } from "./goals";
export const milestoneDraft = z.object({
  title: z.string().trim().min(1).max(200),
  required: z.boolean(),
  weight: z.number().positive().max(1000),
  completed: z.boolean(),
  reason: z.string(),
});
export const taskDraft = z.object({
  title: z.string().trim().min(1).max(200),
  description: z.string(),
  day_offset: z.number().int().min(0),
  milestone: z.number().int().min(-1),
  required: z.boolean(),
  estimated_minutes: z.number().int(),
  difficulty: z.number().int(),
});
export const templateDraft = z.object({
  template_id: z.string(),
  input: goalInput,
  duration_days: z.number().int().min(1).max(3660),
  milestones: z.array(milestoneDraft).max(20),
  tasks: z.array(taskDraft).max(100),
  confirmed: z.boolean(),
});
const template = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  input: goalInput,
  milestones: z.array(milestoneDraft),
  tasks: z.array(taskDraft),
});
const preview = z.object({
  input: goalInput,
  milestones: z.array(milestoneDraft),
  tasks: z.array(taskDraft.extend({ planned_date: z.iso.date() })),
  notice: z.string(),
});
export type GoalTemplate = z.infer<typeof template>;
export type TemplateDraft = z.infer<typeof templateDraft>;
export type TemplatePreview = z.infer<typeof preview>;
export async function goalTemplates(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/templates/goals", { signal });
  return z.array(template).parse(r.data?.data);
}
export async function previewTemplate(input: TemplateDraft) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/goals/preview", {
      params: authParams(),
      body: input,
    });
    return preview.parse(r.data?.data);
  });
}
export async function applyTemplate(input: TemplateDraft, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/goals/apply", {
      params: { header: { ...authParams().header, "Idempotency-Key": key } },
      body: input,
    });
    return goalSchema.parse(r.data?.data);
  });
}
