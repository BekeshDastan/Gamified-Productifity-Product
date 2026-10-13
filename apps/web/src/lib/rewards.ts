"use client";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { progressionBalance } from "./progression";
import { characterSchema } from "./character";
export const customRewardInput = z.object({
  title: z.string().trim().min(1).max(120),
  description: z.string().max(2000),
  cost: z.number().int().min(0).max(1000000),
  available: z.boolean(),
  archived: z.boolean(),
  price_confirmed: z.boolean(),
});
export const rewardDefinition = customRewardInput
  .omit({ price_confirmed: true })
  .extend({ id: z.uuid(), version: z.number().int().positive() });
export type RewardDefinition = z.infer<typeof rewardDefinition>;
export const redemption = z.object({
  id: z.uuid(),
  kind: z.enum(["custom", "cosmetic"]),
  target_id: z.string(),
  title: z.string(),
  description: z.string(),
  cost: z.number().int().nonnegative(),
  definition_version: z.number().int(),
  status: z.enum(["pending", "claimed", "cancelled"]),
  version: z.number().int().positive(),
  created_at: z.string(),
  reason: z.string(),
});
export type Redemption = z.infer<typeof redemption>;
export const redemptionPage = z.object({
  items: z.array(redemption),
  next_cursor: z.string().nullable(),
});
export const rewardCatalog = z.object({
  version: z.string(),
  balance: progressionBalance,
  cosmetics: z.array(
    characterSchema.shape.inventory.element.extend({ cost: z.number().int().nonnegative() }),
  ),
  custom: z.array(rewardDefinition),
  redemptions: redemptionPage,
});
export type PurchaseInput = {
  operation_id: string;
  kind: "custom" | "cosmetic";
  target_id: string;
  expected_cost: number;
  expected_version: number;
};
export async function getRewardCatalog(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/rewards/catalog", { signal });
  return rewardCatalog.parse(r.data?.data);
}
export function useRewardCatalog() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "rewards"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getRewardCatalog(signal),
    staleTime: 0,
    refetchInterval: 15000,
  });
}
export async function saveCustomReward(
  existing: RewardDefinition | null,
  input: z.infer<typeof customRewardInput>,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      header = {
        ...authParams().header,
        "Idempotency-Key": key,
        "If-Match": '"' + (existing?.version ?? 0) + '"',
      };
    const r = existing
      ? await api.PATCH("/api/v1/rewards/custom/{id}", {
          params: { header, path: { id: existing.id } },
          body: input,
        })
      : await api.POST("/api/v1/rewards/custom", { params: { header }, body: input });
    return rewardDefinition.parse(r.data?.data);
  });
}
export async function purchaseReward(input: PurchaseInput) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/rewards/purchase", {
        params: { header: { ...authParams().header, "Idempotency-Key": input.operation_id } },
        body: input,
      });
    return redemption.parse(r.data?.data);
  });
}
export async function changeRedemption(p: Redemption, action: "claim" | "cancel", key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST(
        action === "claim"
          ? "/api/v1/rewards/redemptions/{id}/claim"
          : "/api/v1/rewards/redemptions/{id}/cancel",
        {
          params: {
            path: { id: p.id },
            header: {
              ...authParams().header,
              "Idempotency-Key": key,
              "If-Match": '"' + p.version + '"',
            },
          },
          body: { reason: "Подтверждено владельцем" },
        },
      );
    return redemption.parse(r.data?.data);
  });
}
export async function getRedemptions(before?: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/rewards/redemptions", {
    params: { query: { before } },
    signal,
  });
  return redemptionPage.parse(r.data?.data);
}
