"use client";
import { z } from "zod";
import { createApiClient, responseError } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import type { Quest } from "@/lib/quests";
export const attachmentSchema = z.object({
  id: z.uuid(),
  quest_id: z.uuid(),
  name: z.string(),
  mime: z.string(),
  size: z.number().int(),
  verdict: z.enum(["pending", "clean", "rejected"]),
  version: z.number().int(),
  created_at: z.iso.datetime({ offset: true }),
});
export type Attachment = z.infer<typeof attachmentSchema>;
export async function questFiles(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/quests/{id}/attachments", {
    params: { path: { id } },
    signal,
  });
  return z.array(attachmentSchema).parse(r.data?.data);
}
export async function uploadQuestFile(q: Quest, file: File) {
  await requireAccount();
  return withAuthLock(async () => {
    await authClient();
    const form = new FormData();
    form.set("quest_id", q.id);
    form.set("version", '"' + q.version + '"');
    form.set("file", file);
    const response = await fetch("/api/v1/attachments", {
      method: "POST",
      headers: authParams().header,
      body: form,
      credentials: "same-origin",
      cache: "no-store",
      redirect: "error",
    });
    if (!response.ok) throw await responseError(response);
    const data: unknown = await response.json();
    return z.object({ data: attachmentSchema }).parse(data).data;
  });
}
export async function changeQuestFile(file: Attachment, remove: boolean) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const params = {
      path: { id: file.id },
      header: { ...authParams().header, "If-Match": '"' + file.version + '"' },
    };
    if (remove) {
      await api.DELETE("/api/v1/attachments/{id}", { params, body: {} });
      return;
    }
    const r = await api.POST("/api/v1/attachments/{id}/scan", { params, body: {} });
    return attachmentSchema.parse(r.data?.data);
  });
}
