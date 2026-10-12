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
export const filterSchema = z.object({
  search: z.string().max(200),
  status: z.string(),
  area_id: z.string(),
  priority: z.string(),
  planned_from: z.string(),
  planned_to: z.string(),
  overdue: z.boolean(),
  archived: z.boolean(),
  sort: z.enum(["newest", "oldest", "title", "planned", "priority"]),
});
export type QuestFilters = z.infer<typeof filterSchema>;
export const emptyFilters: QuestFilters = {
  search: "",
  status: "",
  area_id: "",
  priority: "",
  planned_from: "",
  planned_to: "",
  overdue: false,
  archived: false,
  sort: "newest",
};
export const viewSchema = z.object({
  id: z.uuid(),
  version: z.number().int(),
  name: z.string(),
  filters: filterSchema,
});
export async function listQuests(
  page = 1,
  signal?: AbortSignal,
  filters: QuestFilters = emptyFilters,
) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests", {
    params: { query: { page, page_size: 25, ...filters } },
    signal,
  });
  return questPage.parse(r.data?.data);
}
export async function getQuestViews(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/views", { signal });
  return z.array(viewSchema).parse(r.data?.data);
}
export async function saveQuestView(name: string, filters: QuestFilters) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/quests/views", {
      params: authParams(),
      body: { name, filters },
    });
    return viewSchema.parse(r.data?.data);
  });
}
export async function deleteQuestView(view: z.infer<typeof viewSchema>) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    await api.DELETE("/api/v1/quests/views/{id}", {
      params: {
        path: { id: view.id },
        header: { ...authParams().header, "If-Match": '"' + view.version + '"' },
      },
    });
  });
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
export const transitionSchema = z.object({
  reason: z.string().max(1000),
  planned_date: z.string(),
  return_to_backlog: z.boolean(),
  shift_deadline: z.boolean(),
  deadline: z.string(),
});
export type TransitionFields = z.infer<typeof transitionSchema>;
export const actionPaths = {
  start: "/api/v1/quests/{id}/start",
  ready: "/api/v1/quests/{id}/ready",
  complete: "/api/v1/quests/{id}/complete",
  reopen: "/api/v1/quests/{id}/reopen",
  postpone: "/api/v1/quests/{id}/postpone",
  skip: "/api/v1/quests/{id}/skip",
  fail: "/api/v1/quests/{id}/fail",
} as const;
export type QuestAction = keyof typeof actionPaths;
export async function actOnQuest(
  q: Quest,
  action: QuestAction,
  inFields: TransitionFields,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST(actionPaths[action], {
      params: {
        path: { id: q.id },
        header: {
          ...authParams().header,
          "If-Match": '"' + q.version + '"',
          "Idempotency-Key": key,
        },
      },
      body: {
        reason: inFields.reason,
        ...(action === "postpone"
          ? {
              planned_date: inFields.planned_date || null,
              return_to_backlog: inFields.return_to_backlog,
              shift_deadline: inFields.shift_deadline,
              deadline: inFields.deadline || null,
            }
          : {}),
      },
    });
    return questSchema.parse(r.data?.data);
  });
}
export const questEventSchema = z.object({
  id: z.number().int(),
  kind: z.string(),
  reason: z.string(),
  before: questSchema.nullable(),
  after: questSchema,
  created_at: z.iso.datetime({ offset: true }),
});
export async function questHistory(id: string, before?: number, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/{id}/history", {
    params: { path: { id }, query: { before } },
    signal,
  });
  return z.array(questEventSchema).parse(r.data?.data);
}
