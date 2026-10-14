"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { analyticsDay, balanceView } from "@/lib/analytics";
import { characterSchema } from "@/lib/character";
import type { components } from "@/lib/api/schema";
type S = components["schemas"];
const text = z.string(),
  integer = z.number().int(),
  bool = z.boolean(),
  range = { from: text, to: text };
export const comparison = z.object({
  metric: text,
  unit: text,
  current: z.number(),
  current_observed: bool,
  previous: z.number().nullable(),
  delta: z.number().nullable(),
  percent: z.number().nullable(),
});
export const reportSnapshot = z.object({
  kind: z.enum(["week", "month"]),
  starts_on: text,
  timezone: text,
  ...range,
  previous_range: z.object(range),
  version: text,
  score_version: text,
  source_revision: integer,
  computed_at: text,
  observed_from: text,
  current_observed: bool,
  previous_observed: bool,
  comparisons: z.array(comparison),
  obligations: z.object({
    planned: integer,
    completed: integer,
    habit_due: integer,
    habit_success: integer,
  }),
  frequent_reason: z
    .object({
      text,
      count: integer,
      sources: z.array(z.object({ id: text, version: integer, at: text, url: text })),
    })
    .nullable(),
  next_step: text,
  text,
  days: z.array(analyticsDay),
  previous_days: z.array(analyticsDay),
  balance: balanceView,
  records: characterSchema.shape.records,
});
export const report = z.object({
  id: z.uuid(),
  revision: integer,
  automatic: bool,
  created_at: text,
  snapshot: reportSnapshot,
});
export const reportPreview = z.object({ id: z.uuid(), expires_at: text, snapshot: reportSnapshot });
const reportBrief = z.object({
  ...range,
  id: z.uuid(),
  kind: z.enum(["week", "month"]),
  timezone: text,
  revision: integer,
  automatic: bool,
  created_at: text,
  text,
});
export type Report = z.infer<typeof report>;
export type ReportPreview = z.infer<typeof reportPreview>;
export type Snapshot = z.infer<typeof reportSnapshot>;
export async function previewReport(body: S["AnalyticsReportInput"]) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/analytics/reports/preview", { params: authParams(), body });
    return reportPreview.parse(r.data?.data);
  });
}
export async function saveReport(body: S["AnalyticsReportSave"]) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/analytics/reports", { params: authParams(), body });
    return report.parse(r.data?.data);
  });
}
export async function listReports(before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics/reports", {
    params: { query: { before } },
    signal,
  });
  return z.object({ items: z.array(reportBrief), next: text }).parse(r.data?.data);
}
export async function getReport(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/analytics/reports/{id}", {
    params: { path: { id } },
    signal,
  });
  return report.parse(r.data?.data);
}
