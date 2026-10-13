"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const journalInput = z.object({
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
  title: z.string().trim().min(1).max(200),
  body: z.string().max(50000),
  tags: z.array(z.string().trim().min(1).max(50)).max(20),
  goal_id: z.uuid().nullable(),
  archived: z.boolean(),
});
export const journalEntry = journalInput.extend({
  id: z.uuid(),
  version: z.number().int(),
  html: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type JournalInput = z.infer<typeof journalInput>;
export type JournalEntry = z.infer<typeof journalEntry>;
export function emptyJournal(date: string): JournalInput {
  return { date, title: "", body: "", tags: [], goal_id: null, archived: false };
}
export async function listJournal(
  filters: { from: string; to: string; tag: string; archived: string; after: string },
  signal?: AbortSignal,
) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/journal/entries", {
    params: { query: filters },
    signal,
  });
  return z.object({ items: z.array(journalEntry), next: z.string() }).parse(r.data?.data);
}
export async function getJournalEntry(id: string) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/journal/entries/{id}", {
    params: { path: { id } },
  });
  return journalEntry.parse(r.data?.data);
}
export async function saveJournalEntry(
  input: JournalInput,
  current: JournalEntry | null,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      header = { ...authParams().header, "Idempotency-Key": key };
    const r = current
      ? await api.PUT("/api/v1/journal/entries/{id}", {
          params: {
            path: { id: current.id },
            header: { ...header, "If-Match": '"' + current.version + '"' },
          },
          body: input,
        })
      : await api.POST("/api/v1/journal/entries", { params: { header }, body: input });
    return journalEntry.parse(r.data?.data);
  });
}
