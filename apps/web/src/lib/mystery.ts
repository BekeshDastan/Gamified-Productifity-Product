"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { characterSchema } from "./character";
const item = characterSchema.shape.inventory.element
  .omit({ owned: true, source: true, revoked: true })
  .extend({ weight: z.number().int().positive() });
export const mysteryGrant = z.object({
  id: z.uuid(),
  source: z.string(),
  catalog_version: z.string(),
  weights: z.array(item),
  item_id: z.string().nullable(),
  compensation: z.number().int().nonnegative().nullable(),
  created_at: z.string(),
  opened_at: z.string().nullable(),
});
export const mysteryCatalog = z.object({
  enabled: z.boolean(),
  version: z.string(),
  compensation: z.number().int().nonnegative(),
  items: z.array(item),
  grants: z.array(mysteryGrant),
});
export async function getMystery(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/rewards/mystery", { signal });
  return mysteryCatalog.parse(r.data?.data);
}
export async function openMystery(id: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/rewards/mystery/{id}/open", {
        params: { path: { id }, header: { ...authParams().header, "Idempotency-Key": key } },
        body: {},
      });
    return mysteryGrant.parse(r.data?.data);
  });
}
