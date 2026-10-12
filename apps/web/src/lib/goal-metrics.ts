"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { goalSchema, type Goal } from "./goals";
import type { components } from "./api/schema";
export const entrySchema = z.object({
  id: z.number().int(),
  value: z.number(),
  unit: z.string(),
  occurred_on: z.iso.date(),
  note: z.string(),
  version: z.number().int(),
  created_at: z.iso.datetime({ offset: true }),
  reason: z.string(),
});
export type MetricEntry = z.infer<typeof entrySchema>;
export type MetricInput = components["schemas"]["GoalMetricInput"];
export async function metricEntries(
  id: string,
  before?: number,
  entry?: number,
  signal?: AbortSignal,
) {
  await requireAccount();
  const api = createApiClient();
  const r = entry
    ? await api.GET("/api/v1/goals/{id}/entries/{entry}/history", {
        signal,
        params: { path: { id, entry }, query: { before } },
      })
    : await api.GET("/api/v1/goals/{id}/entries", {
        signal,
        params: { path: { id }, query: { before } },
      });
  return z.array(entrySchema).parse(r.data?.data);
}
export async function saveMetric(goal: Goal, input: MetricInput, key: string, entry?: MetricEntry) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const header = {
      ...authParams().header,
      "If-Match": '"' + goal.version + '"',
      "Idempotency-Key": key,
    };
    const r = entry
      ? await api.PATCH("/api/v1/goals/{id}/entries/{entry}", {
          params: { path: { id: goal.id, entry: entry.id }, header },
          body: input,
        })
      : await api.POST("/api/v1/goals/{id}/entries", {
          params: { path: { id: goal.id }, header },
          body: input,
        });
    return goalSchema.parse(r.data?.data);
  });
}
