"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { goalSchema, type Goal } from "./goals";
import type { components } from "./api/schema";
export type MilestoneInput = components["schemas"]["GoalMilestoneInput"];
export type LinkInput = components["schemas"]["GoalLinkInput"];
export const milestoneSchema = z.object({
  id: z.uuid(),
  title: z.string(),
  required: z.boolean(),
  weight: z.number(),
  completed: z.boolean(),
  version: z.number().int(),
  reason: z.string(),
});
export const goalPlanSchema = z.object({
  milestones: z.array(milestoneSchema),
  links: z.array(
    z.object({
      quest_id: z.uuid(),
      title: z.string(),
      status: z.string(),
      milestone_id: z.uuid().nullable(),
      required: z.boolean(),
      weight: z.number(),
    }),
  ),
});
export const assignmentSchema = z.object({
  goal_id: z.uuid().nullable(),
  goal_title: z.string(),
  goal_version: z.number().int(),
});
export type Milestone = z.infer<typeof milestoneSchema>;
export async function getGoalPlan(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/goals/{id}/plan", {
    signal,
    params: { path: { id } },
  });
  return goalPlanSchema.parse(r.data?.data);
}
export async function questGoal(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/{id}/goal", {
    signal,
    params: { path: { id } },
  });
  return assignmentSchema.parse(r.data?.data);
}
export async function saveMilestone(
  g: Goal,
  input: MilestoneInput,
  current?: Milestone,
  remove = false,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const header = { ...authParams().header, "If-Match": '"' + g.version + '"' };
    const r = current
      ? remove
        ? await api.DELETE("/api/v1/goals/{id}/milestones/{mid}", {
            params: { path: { id: g.id, mid: current.id }, header },
            body: input,
          })
        : await api.PATCH("/api/v1/goals/{id}/milestones/{mid}", {
            params: { path: { id: g.id, mid: current.id }, header },
            body: input,
          })
      : await api.POST("/api/v1/goals/{id}/milestones", {
          params: { path: { id: g.id }, header },
          body: input,
        });
    return goalSchema.parse(r.data?.data);
  });
}
export async function saveGoalLink(g: Goal, qid: string, input: LinkInput, remove = false) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const params = {
      path: { id: g.id, qid },
      header: { ...authParams().header, "If-Match": '"' + g.version + '"' },
    };
    const r = remove
      ? await api.DELETE("/api/v1/goals/{id}/links/{qid}", { params, body: input })
      : await api.PUT("/api/v1/goals/{id}/links/{qid}", { params, body: input });
    return goalSchema.parse(r.data?.data);
  });
}
