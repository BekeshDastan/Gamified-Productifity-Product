"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { questInput, questSchema, type Quest } from "@/lib/quests";
import type { components } from "@/lib/api/schema";
export type QuestBatch = components["schemas"]["QuestBatch"];
export const templateSchema = z.object({
  id: z.uuid(),
  name: z.string(),
  input: questInput,
  version: z.number().int(),
});
export type QuestTemplate = z.infer<typeof templateSchema>;
export type TemplateDates = { planned_date: string | null; deadline: string | null };
export const previewSchema = z.object({ input: questInput, version: z.number().int() });
export async function getTemplates(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/templates/quests", { signal });
  return z.array(templateSchema).parse(r.data?.data);
}
export async function saveTemplate(name: string, q: Quest) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/quests", {
      params: authParams(),
      body: { name, quest_id: q.id },
    });
    return templateSchema.parse(r.data?.data);
  });
}
export async function renameTemplate(t: QuestTemplate, name: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.PATCH("/api/v1/templates/quests/{id}", {
      params: {
        path: { id: t.id },
        header: { ...authParams().header, "If-Match": '"' + t.version + '"' },
      },
      body: { name, input: t.input },
    });
    return templateSchema.parse(r.data?.data);
  });
}
export async function deleteTemplate(t: QuestTemplate) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    await api.DELETE("/api/v1/templates/quests/{id}", {
      params: {
        path: { id: t.id },
        header: { ...authParams().header, "If-Match": '"' + t.version + '"' },
      },
      body: {},
    });
  });
}
export async function previewTemplate(id: string, dates: TemplateDates) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/quests/{id}/preview", {
      params: { path: { id }, ...authParams() },
      body: dates,
    });
    return previewSchema.parse(r.data?.data);
  });
}
export async function applyTemplate(
  id: string,
  dates: TemplateDates,
  version: number,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/templates/quests/{id}/apply", {
      params: {
        path: { id },
        header: { ...authParams().header, "If-Match": '"' + version + '"', "Idempotency-Key": key },
      },
      body: dates,
    });
    return questSchema.parse(r.data?.data);
  });
}
export async function questBatch(batch: QuestBatch, preview: boolean, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = preview
      ? await api.POST("/api/v1/quests/batch/preview", { params: authParams(), body: batch })
      : await api.POST("/api/v1/quests/batch", {
          params: { header: { ...authParams().header, "Idempotency-Key": key } },
          body: batch,
        });
    return z.array(questSchema).parse(r.data?.data);
  });
}
export async function archiveQuest(q: Quest, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST(
      q.archived_at ? "/api/v1/quests/{id}/restore" : "/api/v1/quests/{id}/archive",
      {
        params: {
          path: { id: q.id },
          header: {
            ...authParams().header,
            "If-Match": '"' + q.version + '"',
            "Idempotency-Key": key,
          },
        },
        body: {},
      },
    );
    return questSchema.parse(r.data?.data);
  });
}
