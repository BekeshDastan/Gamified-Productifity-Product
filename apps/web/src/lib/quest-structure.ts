"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { questSchema, type Quest } from "@/lib/quests";
export const structureSchema = z.object({
  container: z.boolean(),
  required_leaves: z.number().int(),
  completed_leaves: z.number().int(),
  children: z.array(questSchema),
  dependencies: z.array(questSchema),
});
export async function questStructure(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/{id}/structure", {
    params: { path: { id } },
    signal,
  });
  return structureSchema.parse(r.data?.data);
}
export async function saveDependencies(q: Quest, ids: string[], reason: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.PUT("/api/v1/quests/{id}/dependencies", {
      params: {
        path: { id: q.id },
        header: { ...authParams().header, "If-Match": '"' + q.version + '"' },
      },
      body: { ids, reason },
    });
    return questSchema.parse(r.data?.data);
  });
}
