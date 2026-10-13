"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const journalInput = z.object({
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
  title: z.string().trim().min(1).max(200),
  body: z.string().max(50000),
  tags: z.array(z.string().trim().min(1).max(50)).max(20),
  goal_id: z.uuid().nullable(),
  archived: z.boolean(),
});
export const journalEntry = journalInput.extend({
  id: z.uuid(),
  version: z.number().int(),
  html: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type JournalInput = z.infer<typeof journalInput>;
export type JournalEntry = z.infer<typeof journalEntry>;
export function emptyJournal(date: string): JournalInput {
  return { date, title: "", body: "", tags: [], goal_id: null, archived: false };
}
export async function listJournal(
  filters: { from: string; to: string; tag: string; archived: string; after: string },
  signal?: AbortSignal,
) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/journal/entries", {
    params: { query: filters },
    signal,
  });
  return z.object({ items: z.array(journalEntry), next: z.string() }).parse(r.data?.data);
}
export async function getJournalEntry(id: string) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/journal/entries/{id}", {
    params: { path: { id } },
  });
  return journalEntry.parse(r.data?.data);
}
export async function saveJournalEntry(
  input: JournalInput,
  current: JournalEntry | null,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      header = { ...authParams().header, "Idempotency-Key": key };
    const r = current
      ? await api.PUT("/api/v1/journal/entries/{id}", {
          params: {
            path: { id: current.id },
            header: { ...header, "If-Match": '"' + current.version + '"' },
          },
          body: input,
        })
      : await api.POST("/api/v1/journal/entries", { params: { header }, body: input });
    return journalEntry.parse(r.data?.data);
  });
}

import { dayPlan, type PlanInput } from "@/lib/planning";
export const routineRun = z.object({
  id: z.uuid(),
  date: z.string(),
  kind: z.string(),
  version: z.number().int(),
  payload: z.record(z.string(), z.unknown()),
  created_at: z.string(),
  updated_at: z.string(),
});
export const morning = z.object({ run: routineRun.nullable(), plan: dayPlan });
export async function getMorning(date: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/routine-runs/morning/{date}", {
    params: { path: { date } },
    signal,
  });
  return morning.parse(r.data?.data);
}
export async function saveMorning(
  date: string,
  current: z.infer<typeof morning>,
  input: PlanInput,
  reason: string,
  key: string,
  templateVersion = 0,
  checked: string[] = [],
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/routine-runs/morning/{date}", {
        params: {
          path: { date },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + (current.run?.version ?? 0) + '"',
          },
        },
        body: {
          input,
          plan_version: current.plan.version,
          reason,
          template_version: templateVersion,
          checked,
        },
      });
    return morning.parse(r.data?.data);
  });
}

export const reviewInput = z.object({
  template_version: z.number().int(),
  checked: z.array(z.string()).max(20),
  obstacle: z.enum(["none", "insufficient_time", "low_energy", "external", "unclear", "other"]),
  obstacle_note: z.string().max(2000),
  conclusion: z.string().trim().min(1).max(5000),
  energy: z.number().int().min(1).max(5).nullable(),
  mood: z.number().int().min(1).max(5).nullable(),
});
const dailyItem = z.object({
  id: z.uuid(),
  title: z.string(),
  status: z.string(),
  planned_date: z.string().nullable(),
  planned: z.boolean(),
  completed_today: z.boolean(),
});
export const dailyReview = z.object({
  run: routineRun.nullable(),
  timezone: z.string(),
  stats: z.object({
    planned: z.number(),
    completed: z.number(),
    completed_today: z.number(),
    postponed: z.number(),
    failed: z.number(),
    skipped: z.number(),
    more: z.boolean(),
    items: z.array(dailyItem),
  }),
});
export type Review = z.infer<typeof dailyReview>;
export async function getReview(date: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/reviews/day/{date}", {
    params: { path: { date } },
    signal,
  });
  return dailyReview.parse(r.data?.data);
}
export async function saveReview(
  date: string,
  current: Review,
  input: z.infer<typeof reviewInput>,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/reviews/day/{date}", {
        params: {
          path: { date },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + (current.run?.version ?? 0) + '"',
          },
        },
        body: input,
      });
    return dailyReview.parse(r.data?.data);
  });
}

const weeklyItem = z.object({
  id: z.uuid(),
  title: z.string(),
  version: z.number().int(),
  status: z.string(),
  planned_date: z.string().nullable(),
  deadline: z.string().nullable(),
  estimated_minutes: z.number(),
  priority: z.string(),
  reason: z.string(),
});
export const weekSnapshot = z.object({
  week: z.string(),
  next_week: z.string(),
  timezone: z.string(),
  items: z.array(weeklyItem),
  more: z.boolean(),
  reasons: z.array(z.object({ reason: z.string(), count: z.number() })),
  plans: z.array(dayPlan),
  review_version: z.number().int(),
});
export const weekPreview = z.object({
  id: z.uuid(),
  snapshot: weekSnapshot,
  expires_at: z.string(),
  applied: z.boolean(),
});
export type WeekPreview = z.infer<typeof weekPreview>;
export async function getWeekReview(date: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/reviews/week/{date}", {
    params: { path: { date } },
    signal,
  });
  return z
    .object({ snapshot: weekSnapshot, last_result: z.record(z.string(), z.unknown()).nullable() })
    .parse(r.data?.data);
}
export async function previewWeekReview(date: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/reviews/week/{date}/preview", {
        params: { path: { date }, header: { ...authParams().header, "Idempotency-Key": key } },
        body: {},
      });
    return weekPreview.parse(r.data?.data);
  });
}

import type { components } from "@/lib/api/schema";
export type WeekApplyInput = components["schemas"]["WeekApplyInput"];
export async function applyWeekReview(
  date: string,
  preview: WeekPreview,
  input: WeekApplyInput,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/reviews/week/{date}/apply-plan", {
        params: {
          path: { date },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + preview.snapshot.review_version + '"',
          },
        },
        body: input,
      });
    return z
      .object({
        preview_id: z.uuid(),
        week: z.string(),
        version: z.number(),
        changed_ids: z.array(z.uuid()),
        plan_ids: z.array(z.uuid()),
        applied_at: z.string(),
      })
      .parse(r.data?.data);
  });
}

export const routineTemplate = z.object({
  kind: z.enum(["morning", "evening"]),
  version: z.number().int(),
  steps: z.array(z.object({ id: z.string(), label: z.string() })).max(20),
});
export type RoutineTemplate = z.infer<typeof routineTemplate>;
export async function getRoutineTemplate(kind: "morning" | "evening", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/routine-runs/templates/{kind}", {
    params: { path: { kind } },
    signal,
  });
  return routineTemplate.parse(r.data?.data);
}
export async function saveRoutineTemplate(
  template: RoutineTemplate,
  steps: RoutineTemplate["steps"],
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/routine-runs/templates/{kind}", {
        params: {
          path: { kind: template.kind },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + template.version + '"',
          },
        },
        body: { steps },
      });
    return routineTemplate.parse(r.data?.data);
  });
}
export async function getRoutineHistory(before: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/routine-runs", {
    params: { query: { before } },
    signal,
  });
  return z
    .object({
      items: z.array(z.object({ id: z.string(), snapshot: routineRun })),
      next: z.string(),
    })
    .parse(r.data?.data);
}
