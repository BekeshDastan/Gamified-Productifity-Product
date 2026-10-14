"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { rewardSchema } from "@/lib/progression";
import type { components } from "@/lib/api/schema";
type S = components["schemas"];
export type ChallengeRun = S["ChallengeRun"];
export type ChallengeMember = S["ChallengeMember"];
export type ChallengeCreate = S["ChallengeCreate"];
export const config = z.object({
  metric: z.enum(["tasks", "habits", "focus", "reviews", "stages", "manual"]),
  unit: z.enum(["tasks", "periods", "minutes", "reviews", "stages", "count", "pages"]),
  kind: z.enum(["cumulative", "periodic"]),
  period: z.enum(["day", "week"]),
  target: z.number().int().positive(),
  period_target: z.number().int().positive(),
  days: z.number().int().min(1).max(365),
  daily_cap: z.number().int().positive(),
  allowed_misses: z.number().int().nonnegative(),
});
const template = config.extend({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  audience: z.string(),
  area_hint: z.string(),
  conditions: z.string(),
  safe_norm: z.string(),
});
const questTemplate = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  audience: z.string(),
  area_hint: z.string(),
  estimated_minutes: z.number(),
  difficulty: z.number(),
  metric: z.literal("tasks"),
  unit: z.literal("tasks"),
  target: z.literal(1),
  conditions: z.string(),
  safe_norm: z.string(),
});
export type ChallengeTemplate = z.infer<typeof template>;
const runSchema = z.object({
  id: z.uuid(),
  rules: config.extend({
    name: z.string(),
    starts_on: z.string(),
    ends_on: z.string(),
    area_id: z.string().nullable(),
    goal_id: z.string().nullable(),
    habit_id: z.string().nullable(),
    rejoin: z.boolean(),
    timezone: z.string(),
    visibility: z.literal("private"),
    confirmation: z.enum(["automatic", "self_report"]),
    scoring_version: z.string(),
    source_policy: z.string(),
    economy_version: z.string(),
    reward_xp: z.number(),
  }),
  rules_hash: z.string(),
  catalog_id: z.string().nullable(),
  catalog_version: z.string(),
  state: z.enum(["open", "closed"]),
  version: z.number(),
  starts_at: z.string(),
  ends_at: z.string(),
  created_at: z.string(),
  finalized_at: z.string().nullable(),
  final_snapshot: z.record(z.string(), z.unknown()).nullable(),
});
const progress = z.object({
  version: z.string(),
  current: z.number(),
  target: z.number(),
  unit: z.string(),
  percent: z.number(),
  target_met: z.boolean(),
  source_count: z.number(),
  periods: z.array(
    z.object({
      start: z.string(),
      end: z.string(),
      current: z.number(),
      target: z.number(),
      state: z.enum(["open", "met", "future", "missed"]),
    }),
  ),
});
const member = z.object({
  reward: rewardSchema.optional(),
  id: z.uuid(),
  run_id: z.uuid(),
  rules_hash: z.string(),
  state: z.enum(["joined", "left", "finished"]),
  version: z.number(),
  first_joined_at: z.string(),
  left_at: z.string().nullable(),
  leave_reason: z.string(),
  updated_at: z.string(),
  progress: progress.nullable(),
  computed_at: z.string().nullable(),
  result: z.enum(["achieved", "not_achieved", "left"]).nullable(),
  final_snapshot: z.record(z.string(), z.unknown()).nullable(),
  closed_at: z.string().nullable(),
});
export const challengeFinalSnapshot = z.object({
  kind: z.literal("result"),
  result: z.enum(["achieved", "not_achieved", "left"]),
  at: z.string(),
  reason: z.string(),
  rules_hash: z.string(),
  progress: progress.nullable(),
});
export async function closeChallenge(run: ChallengeRun, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs/{id}/close", {
        params: { path: { id: run.id }, header: headers(key, run.version) },
        body: { confirm: true, reason },
      });
    return z
      .object({ run: runSchema, member: member.nullable(), reward: rewardSchema.nullable() })
      .parse(r.data?.data);
  });
}
export async function challengeFinalSources(id: string, before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}/final-sources", {
    params: { path: { id }, query: { before } },
    signal,
  });
  return z
    .object({
      items: z.array(source),
      next: z.string(),
      total: z.number(),
      unit: z.string(),
      timezone: z.string(),
    })
    .parse(r.data?.data);
}
const source = z.object({
  kind: z.enum(["task", "habit", "focus", "review", "stage", "manual"]),
  key: z.uuid(),
  title: z.string(),
  url: z.string().startsWith("/app/"),
  amount: z.number(),
  at: z.string(),
  version: z.number(),
  confidence: z.enum(["recorded", "self_report"]),
});
const checkin = z.object({
  id: z.uuid(),
  participant_id: z.uuid(),
  date: z.string(),
  unit: z.string(),
  initial_amount: z.number(),
  amount: z.number(),
  note: z.string(),
  version: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
  confirmation: z.literal("self_report"),
});
export type Checkin = z.infer<typeof checkin>;
export async function challengeCatalog(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/catalog", { signal });
  return z
    .object({ version: z.string(), challenges: z.array(template), quests: z.array(questTemplate) })
    .parse(r.data?.data);
}
export async function challengeRuns(after = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs", {
    params: { query: { before: after } },
    signal,
  });
  return z.object({ items: z.array(runSchema), next: z.string() }).parse(r.data?.data);
}
export async function challengeRun(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}", {
    params: { path: { id } },
    signal,
  });
  return runSchema.parse(r.data?.data);
}
type MutationHeaders = ReturnType<typeof authParams>["header"] & { "Idempotency-Key": string };
function headers(key: string, version: number): MutationHeaders & { "If-Match": string };
function headers(key: string): MutationHeaders;
function headers(key: string, version?: number) {
  return {
    ...authParams().header,
    "Idempotency-Key": key,
    ...(version === undefined ? {} : { "If-Match": `"${version}"` }),
  };
}
export async function createChallenge(input: ChallengeCreate, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs", {
        params: { header: headers(key) },
        body: input,
      });
    return runSchema.parse(r.data?.data);
  });
}
export async function challengeMembership(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}/membership", {
    params: { path: { id } },
    signal,
  });
  return z.object({ member: member.nullable() }).parse(r.data?.data);
}
export async function challengeProgress(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}/progress", {
    params: { path: { id } },
    signal,
  });
  return member.parse(r.data?.data);
}
export async function joinChallenge(run: ChallengeRun, version: number, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs/{id}/join", {
        params: { path: { id: run.id }, header: headers(key) },
        body: { rules_hash: run.rules_hash, accept_rules: true, membership_version: version },
      });
    return member.parse(r.data?.data);
  });
}
export async function leaveChallenge(id: string, version: number, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs/{id}/leave", {
        params: { path: { id }, header: headers(key, version) },
        body: { reason },
      });
    return member.parse(r.data?.data);
  });
}
export async function cancelChallenge(id: string, version: number, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs/{id}/cancel", {
        params: { path: { id }, header: headers(key, version) },
        body: { reason },
      });
    return runSchema.parse(r.data?.data);
  });
}
export async function challengeSources(id: string, before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}/sources", {
    params: { path: { id }, query: { before } },
    signal,
  });
  return z
    .object({
      items: z.array(source),
      next: z.string(),
      total: z.number(),
      unit: z.string(),
      timezone: z.string(),
    })
    .parse(r.data?.data);
}
export async function challengeCheckins(id: string, before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/runs/{id}/check-ins", {
    params: { path: { id }, query: { before } },
    signal,
  });
  return z.object({ items: z.array(checkin), next: z.string() }).parse(r.data?.data);
}
export async function saveCheckin(id: string, input: S["ChallengeCheckinInput"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/runs/{id}/check-ins", {
        params: { path: { id }, header: headers(key) },
        body: input,
      });
    return checkin.parse(r.data?.data);
  });
}
export async function correctCheckin(
  id: string,
  current: Checkin,
  input: S["ChallengeCheckinCorrection"],
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PATCH("/api/v1/challenges/runs/{id}/check-ins/{checkin}", {
        params: { path: { id, checkin: current.id }, header: headers(key, current.version) },
        body: input,
      });
    return checkin.parse(r.data?.data);
  });
}
const evidence = z.object({
  attachment_id: z.string(),
  name: z.string(),
  url: z.string(),
  visibility: z.enum(["private", "participants"]),
  consent_at: z.string().nullable(),
  version: z.number(),
  available: z.boolean(),
  confirmation: z.literal("self_report"),
});
export async function challengeEvidence(id: string, checkin: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET(
    "/api/v1/challenges/runs/{id}/check-ins/{checkin}/evidence",
    { params: { path: { id, checkin } }, signal },
  );
  return z.array(evidence).parse(r.data?.data);
}
export async function setChallengeEvidence(
  id: string,
  checkin: string,
  file: string,
  version: number,
  input: S["ChallengeEvidenceInput"],
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/challenges/runs/{id}/check-ins/{checkin}/evidence/{file}", {
        params: { path: { id, checkin, file }, header: headers(key, version) },
        body: input,
      });
    return evidence.parse(r.data?.data);
  });
}
export async function questSuggestions(
  period: "daily" | "weekly",
  available_minutes: number,
  area_id: string,
  signal?: AbortSignal,
) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/challenges/suggestions", {
    params: { query: { period, available_minutes, ...(area_id ? { area_id } : {}) } },
    signal,
  });
  return z
    .object({
      period: z.enum(["daily", "weekly"]),
      starts_on: z.string(),
      timezone: z.string(),
      available_minutes: z.number(),
      area_id: z.string().nullable(),
      items: z.array(
        z.object({ template: questTemplate, quest_id: z.string().nullable(), reason: z.string() }),
      ),
    })
    .parse(r.data?.data);
}
export async function acceptQuestSuggestion(input: S["AcceptQuestSuggestion"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/challenges/suggestions/accept", {
        params: { header: headers(key) },
        body: input,
      });
    return z.object({ quest_id: z.uuid() }).parse(r.data?.data);
  });
}
