"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const statuses = {
  backlog: "Идеи",
  todo: "К выполнению",
  in_progress: "В работе",
  postponed: "Перенесены",
  completed: "Выполнены",
  failed: "Не получилось",
  skipped: "Пропущены",
} as const;
export const questInput = z.object({
  title: z.string().trim().min(1, "Укажите название").max(200),
  description: z.string().max(10000),
  area_id: z.uuid().nullable(),
  priority: z.enum(["low", "normal", "high"]),
  difficulty: z.number().int().min(1).max(5),
  deadline: z.iso.datetime({ offset: true }).nullable(),
  planned_date: z.iso.date().nullable(),
  estimated_minutes: z.number().int().min(0).max(525600),
  actual_minutes: z.number().int().min(0).max(525600),
  tags: z.array(z.uuid()).max(50),
  status: z.enum(["backlog", "todo", "in_progress", "postponed", "completed", "failed", "skipped"]),
});
export const questSchema = questInput.extend({
  id: z.uuid(),
  version: z.number().int().positive(),
  completed_at: z.iso.datetime({ offset: true }).nullable(),
  archived_at: z.iso.datetime({ offset: true }).nullable(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
export const questPage = z.object({
  items: z.array(questSchema),
  total: z.number().int(),
  page: z.number().int(),
  page_size: z.number().int(),
});
export type Quest = z.infer<typeof questSchema>;
export type QuestInput = z.infer<typeof questInput>;
export const emptyQuest: QuestInput = {
  title: "",
  description: "",
  area_id: null,
  priority: "normal",
  difficulty: 1,
  deadline: null,
  planned_date: null,
  estimated_minutes: 0,
  actual_minutes: 0,
  tags: [],
  status: "todo",
};
export async function listQuests(page = 1, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests", {
    params: { query: { page, page_size: 25 } },
    signal,
  });
  return questPage.parse(r.data?.data);
}
export async function getQuest(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/{id}", {
    params: { path: { id } },
    signal,
  });
  return questSchema.parse(r.data?.data);
}
export async function saveQuest(input: QuestInput, current?: Quest) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = current
      ? await api.PATCH("/api/v1/quests/{id}", {
          params: {
            path: { id: current.id },
            header: { ...authParams().header, "If-Match": '"' + current.version + '"' },
          },
          body: input,
        })
      : await api.POST("/api/v1/quests", { params: authParams(), body: input });
    return questSchema.parse(r.data?.data);
  });
}
export async function questLabels(signal?: AbortSignal) {
  await requireAccount();
  const api = createApiClient();
  const [areas, tags] = await Promise.all([
    api.GET("/api/v1/areas", { signal }),
    api.GET("/api/v1/tags", { signal }),
  ]);
  const schema = z.array(z.object({ id: z.uuid(), name: z.string(), archived: z.boolean() }));
  return { areas: schema.parse(areas.data?.data), tags: schema.parse(tags.data?.data) };
}
