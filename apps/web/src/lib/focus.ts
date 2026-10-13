"use client";
import { z } from "zod";
import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
const integer = z.number().int().nonnegative(),
  instant = z.iso.datetime({ offset: true });
export const focusStart = z.object({
  operation_id: z.uuid(),
  mode: z.enum(["free", "pomodoro", "quick"]),
  title: z.string().max(200),
  quest_id: z.uuid().nullable(),
  goal_id: z.uuid().nullable(),
  area_id: z.uuid().nullable(),
  work_minutes: integer.min(5).max(120),
  break_minutes: integer.min(1).max(60),
  sound_enabled: z.boolean(),
});
export type FocusStart = z.infer<typeof focusStart>;
export const focusSession = z.object({
  id: z.uuid(),
  state: z.enum(["running", "paused", "awaiting_confirmation", "finished", "cancelled"]),
  mode: z.enum(["free", "pomodoro", "quick"]),
  phase: z.enum(["work", "break", "work_complete", "break_complete", "review"]),
  title: z.string(),
  quest_id: z.uuid().nullable(),
  goal_id: z.uuid().nullable(),
  area_id: z.uuid().nullable(),
  timezone: z.string(),
  work_minutes: integer,
  break_minutes: integer,
  sound_enabled: z.boolean(),
  cycle: integer.positive(),
  version: integer.positive(),
  started_at: instant,
  updated_at: instant,
  ended_at: instant.nullable(),
  phase_deadline: instant.nullable(),
  phase_remaining_seconds: integer.nullable(),
  result: z.string(),
  comment: z.string(),
  active_seconds: integer,
  break_seconds: integer,
  segments: z.array(
    z.object({
      id: integer,
      kind: z.enum(["work", "break"]),
      started_at: instant,
      ended_at: instant.nullable(),
      seconds: integer,
    }),
  ),
});
export type FocusSession = z.infer<typeof focusSession>;
const current = z.object({ server_now: instant, session: focusSession.nullable() });
export async function getFocus(signal?: AbortSignal) {
  await requireAccount();
  const before = performance.now(),
    deviceBefore = Date.now();
  const r = await createApiClient().GET("/api/v1/focus/current", { signal }),
    received = performance.now(),
    deviceAfter = Date.now(),
    data = current.parse(r.data?.data),
    serverMs = Date.parse(data.server_now);
  return {
    ...data,
    sync: {
      serverMs,
      receivedMono: received,
      uncertaintyMs: (received - before) / 2,
      skewMs: serverMs - (deviceBefore + deviceAfter) / 2,
    },
  };
}
export function useFocus() {
  const auth = useAuth(),
    cache = useQueryClient(),
    id = auth.session?.account.id;
  useEffect(() => {
    if (!id) return;
    const refresh = () => {
      if (document.visibilityState === "visible")
        void cache.invalidateQueries({ queryKey: ["account", id, "focus"] });
    };
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("online", refresh);
    const channel =
      typeof BroadcastChannel !== "undefined" ? new BroadcastChannel("ascend-focus-" + id) : null;
    if (channel) channel.onmessage = refresh;
    return () => {
      document.removeEventListener("visibilitychange", refresh);
      window.removeEventListener("online", refresh);
      channel?.close();
    };
  }, [id, cache]);
  return useQuery({
    queryKey: ["account", id, "focus"],
    enabled: !!id,
    queryFn: ({ signal }) => getFocus(signal),
    refetchInterval: 15000,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
    staleTime: 0,
  });
}
export function broadcastFocus(accountId: string) {
  if (typeof BroadcastChannel === "undefined") return;
  const c = new BroadcastChannel("ascend-focus-" + accountId);
  c.postMessage("changed");
  c.close();
}
export async function startFocus(input: FocusStart, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/focus/start", {
        params: { header: { ...authParams().header, "Idempotency-Key": key } },
        body: input,
      });
    return focusSession.parse(r.data?.data);
  });
}
export type FocusAction = "pause" | "resume" | "finish" | "cancel" | "break" | "next" | "extend";
export async function actOnFocus(
  f: FocusSession,
  action: FocusAction,
  comment: string,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/focus/sessions/{id}/actions", {
        params: {
          path: { id: f.id },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + f.version + '"',
          },
        },
        body: { action, comment },
      });
    return focusSession.parse(r.data?.data);
  });
}
export async function confirmFocus(
  f: FocusSession,
  result: "progress" | "no_progress" | "rest",
  comment: string,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/focus/sessions/{id}/confirm", {
        params: {
          path: { id: f.id },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + f.version + '"',
          },
        },
        body: { result, comment },
      });
    return focusSession.parse(r.data?.data);
  });
}
export const questFocusTime = z.object({
  quest_id: z.uuid(),
  mode: z.enum(["from_focus", "manual_override"]),
  manual_minutes: integer,
  focus_seconds: integer,
  effective_seconds: integer,
  confirmed_sessions: integer,
  overridden: z.boolean(),
});
export async function getQuestFocusTime(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/focus/quests/{id}/time", {
    params: { path: { id } },
    signal,
  });
  return questFocusTime.parse(r.data?.data);
}
export const focusHistory = z.object({
  items: z.array(focusSession),
  next_cursor: z.string().nullable(),
  summary: z.object({
    sessions: integer,
    work_seconds: integer,
    confirmed_seconds: integer,
    pending_seconds: integer,
    cancelled_seconds: integer,
    rest_seconds: integer,
  }),
  as_of: instant,
  from: z.iso.date(),
  to: z.iso.date(),
});
export type FocusHistoryFilters = {
  from: string;
  to: string;
  quest_id?: string;
  area_id?: string;
  state?: string;
};
export async function getFocusHistory(
  filters: FocusHistoryFilters,
  before?: string,
  signal?: AbortSignal,
) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/focus/history", {
    params: { query: { ...filters, before } },
    signal,
  });
  return focusHistory.parse(r.data?.data);
}
export async function getFocusSession(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/focus/sessions/{id}", {
    params: { path: { id } },
    signal,
  });
  const data = current.parse(r.data?.data);
  if (!data.session) throw Error("Missing session");
  return data.session;
}
