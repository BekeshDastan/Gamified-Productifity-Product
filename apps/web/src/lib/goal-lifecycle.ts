"use client";
import { z } from "zod";
import { createApiClient } from "./api/client";
import { authClient, authParams, withAuthLock } from "./auth/session";
import { requireAccount } from "./profile";
import { goalSchema, type Goal } from "./goals";
import { goalPlanSchema } from "./goal-plan";
import type { components } from "./api/schema";
export const goalActions = {
  resume: "Начать или продолжить",
  pause: "Приостановить",
  achieve: "Подтвердить достижение",
  "close-unachieved": "Завершить без достижения",
  cancel: "Отменить цель",
  review: "Пересмотреть",
  archive: "Убрать в архив",
  restore: "Восстановить из архива",
} as const;
export type GoalAction = keyof typeof goalActions;
export type GoalTransition = components["schemas"]["GoalTransition"];
export async function transitionGoal(
  goal: Goal,
  action: GoalAction,
  body: GoalTransition,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const paths = {
      resume: "/api/v1/goals/{id}/resume",
      pause: "/api/v1/goals/{id}/pause",
      achieve: "/api/v1/goals/{id}/achieve",
      "close-unachieved": "/api/v1/goals/{id}/close-unachieved",
      cancel: "/api/v1/goals/{id}/cancel",
      review: "/api/v1/goals/{id}/review",
      archive: "/api/v1/goals/{id}/archive",
      restore: "/api/v1/goals/{id}/restore",
    } as const;
    const r = await api.POST(paths[action], {
      params: {
        path: { id: goal.id },
        header: {
          ...authParams().header,
          "If-Match": '"' + goal.version + '"',
          "Idempotency-Key": key,
        },
      },
      body,
    });
    return goalSchema.parse(r.data?.data);
  });
}
export const goalVersionSchema = z.object({
  id: z.number().int(),
  goal_version: z.number().int(),
  kind: z.string(),
  reason: z.string(),
  snapshot: goalSchema.extend({ plan: goalPlanSchema }),
  created_at: z.iso.datetime({ offset: true }),
});
export async function goalHistory(id: string, before?: number, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/goals/{id}/history", {
    signal,
    params: { path: { id }, query: { before } },
  });
  return z.array(goalVersionSchema).parse(r.data?.data);
}
