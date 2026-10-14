"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { dayPlan, planItem } from "@/lib/planning";
import { calendarEvent } from "@/lib/calendar";
import { config } from "@/lib/challenges";
import { habitPeriod, habitSchema } from "@/lib/habits";
import type { components } from "@/lib/api/schema";
type S = components["schemas"];
const text = z.string(),
  num = z.number(),
  nil = text.nullable();
export const seasonInput = z.object({
  title: text.trim().min(1).max(200),
  theme: text.trim().min(1).max(200),
  desired_result: text.trim().min(1).max(2000),
  starts_on: z.iso.date(),
  ends_on: z.iso.date(),
  primary: z.boolean(),
});
export const season = seasonInput.extend({
  id: z.uuid(),
  timezone: text,
  state: z.enum(["planned", "active", "completed", "cancelled"]),
  version: num,
  activated_at: nil,
  closed_at: nil,
  final_snapshot: z.record(text, z.unknown()).nullable(),
  archived_at: nil,
  created_at: text,
});
const source = z.object({
  id: z.uuid(),
  name: text,
  metric: text,
  value: num.nullable(),
  target: num,
  unit: text,
  percent: num.nullable(),
  state: text,
  result: nil,
  version: num,
  stages_completed: num,
  stages_total: num,
  url: text.startsWith("/app/"),
  baseline: z.record(text, z.unknown()).nullable(),
});
export const overview = z.object({
  season,
  main_goal_id: nil,
  goals: z.array(source),
  challenges: z.array(source),
  bosses: z.array(
    z.object({
      boss_id: z.uuid(),
      goal_id: z.uuid(),
      name: text,
      month: text,
      timezone: text,
      state: text,
      progress: num,
      source_changed: z.boolean(),
    }),
  ),
});
const activity = z.object({
  from: nil,
  until: text,
  tasks: num,
  habit_norms: num,
  focus_seconds: num,
  reviews: num,
  xp_delta: num,
  coins_delta: num,
  level_at_close: num,
});
export const summary = z.object({
  version: text,
  kind: text,
  at: text,
  early: z.boolean(),
  reason: text,
  overview,
  activity,
  goals_achieved: num,
  goals_unavailable: num,
  challenges_achieved: num,
});
export const recoveryStep = z.object({
  id: text,
  title: text.trim().min(1).max(200),
  description: text.max(2000),
  estimated_minutes: num.int().min(1).max(15),
});
export const recoveryOffer = z.object({
  id: z.uuid(),
  catalog_version: text,
  challenge_rules: config,
  reward_xp: num,
  economy_version: text,
  rejoin: z.boolean(),
  visibility: z.literal("private"),
  source_policy: text,
  steps: z.array(recoveryStep),
  last_result_at: nil,
  inactive_since: text,
  offered_at: text,
  expires_at: text,
  state: z.enum(["offered", "accepted", "declined", "withdrawn", "expired"]),
  version: num,
  decided_at: nil,
  result: z
    .object({
      goal_id: nil,
      run_id: nil,
      quest_ids: z.array(z.uuid()),
      steps: z.array(recoveryStep),
    })
    .nullable(),
});
export const eligibility = z.object({
  timezone: text,
  eligible: z.boolean(),
  last_result_at: nil,
  inactive_since: text,
  cooldown_until: nil,
  offer: recoveryOffer.nullable(),
});
export const lowEnergy = z.object({
  date: text,
  minimum_minutes: num,
  id: z.uuid(),
  expires_at: text,
  timezone: text,
  original: dayPlan,
  candidates: z.array(
    planItem.extend({
      required: z.boolean(),
      proposed: z.boolean(),
      eligible: z.boolean(),
      reason: text,
    }),
  ),
  commitments: z.array(calendarEvent),
  source_hash: text,
  applied_at: nil,
  result: dayPlan.nullable(),
});
const comebackMember = z.object({
  state: text,
  result: nil,
  progress: z
    .object({
      current: num,
      target: num,
      unit: text,
      percent: num,
      target_met: z.boolean(),
      source_count: num,
      periods: z.array(z.unknown()),
      version: text,
    })
    .nullable(),
});
export const recoveryStatus = z.object({
  computed_at: text,
  complete: z.boolean(),
  habits: z.array(
    z.object({
      id: z.uuid(),
      title: text,
      url: text.startsWith("/app/"),
      stats: habitSchema.shape.stats,
      periods: z.array(habitPeriod),
      current_amount: num.nullable(),
      full_target: num.nullable(),
      minimum: num.nullable(),
      minimum_met: z.boolean().nullable(),
    }),
  ),
  comebacks: z.array(
    z.object({
      offer_id: z.uuid(),
      goal_id: nil,
      run_id: z.uuid(),
      url: text.startsWith("/app/"),
      member: comebackMember,
    }),
  ),
});
export type Season = z.infer<typeof season>;
export type Overview = z.infer<typeof overview>;
export type RecoveryOffer = z.infer<typeof recoveryOffer>;
export type Eligibility = z.infer<typeof eligibility>;
export type LowEnergy = z.infer<typeof lowEnergy>;
function headers(key: string, version?: number) {
  return { ...authParams().header, "Idempotency-Key": key, "If-Match": `"${version ?? 0}"` };
}
export async function listSeasons(before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/seasons", {
    params: { query: { before } },
    signal,
  });
  return z.object({ items: z.array(season), next: text }).parse(r.data?.data);
}
export async function getOverview(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/seasons/{id}/overview", {
    params: { path: { id } },
    signal,
  });
  return overview.parse(r.data?.data);
}
export async function getSummary(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/seasons/{id}/summary", {
    params: { path: { id } },
    signal,
  });
  return summary.parse(r.data?.data);
}
export async function getComeback(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/recovery/comeback", { signal });
  return eligibility.parse(r.data?.data);
}
export async function getRecoveryStatus(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/recovery/status", { signal });
  return recoveryStatus.parse(r.data?.data);
}
export async function getRecoveryHistory(before = "", signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/recovery/comeback/history", {
    params: { query: { before } },
    signal,
  });
  return z.object({ items: z.array(recoveryOffer), next: text }).parse(r.data?.data);
}
export async function createSeason(body: S["CreateSeason"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/seasons", { params: { header: headers(key) }, body });
    return season.parse(r.data?.data);
  });
}
export async function activateSeason(v: Season, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/seasons/{id}/activate", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body: { confirm: true },
      });
    return season.parse(r.data?.data);
  });
}
export async function linkSeason(v: Season, body: S["SeasonLinksInput"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/seasons/{id}/links", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body,
      });
    return overview.parse(r.data?.data);
  });
}
export async function linkMonthly(v: Season, body: S["SeasonMonthlyLinkInput"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/seasons/{id}/monthly-boss", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body,
      });
    return overview.parse(r.data?.data);
  });
}
export async function closeSeason(v: Season, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/seasons/{id}/close", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body: { confirm: true, reason },
      });
    return summary.parse(r.data?.data);
  });
}
export async function archiveSeason(v: Season, archived: boolean, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/seasons/{id}/archive", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body: { confirm: true, archived },
      });
    return season.parse(r.data?.data);
  });
}
export async function decideComeback(v: RecoveryOffer, body: S["RecoveryDecision"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/recovery/comeback/{id}/decide", {
        params: { path: { id: v.id }, header: headers(key, v.version) },
        body,
      });
    return recoveryOffer.parse(r.data?.data);
  });
}
export async function previewLowEnergy(body: S["LowEnergyInput"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/recovery/low-energy/preview", {
        params: { header: headers(key) },
        body,
      });
    return lowEnergy.parse(r.data?.data);
  });
}
export async function applyLowEnergy(id: string, body: S["LowEnergyApply"], key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/recovery/low-energy/{id}/apply", {
        params: { path: { id }, header: headers(key) },
        body,
      });
    return lowEnergy.parse(r.data?.data);
  });
}
