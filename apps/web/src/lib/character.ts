"use client";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { progressionBalance } from "./progression";
export const slots = ["background", "base", "clothes", "head", "accessory", "frame"] as const;
export const slotNames = {
  base: "Основа",
  clothes: "Одежда",
  head: "Головной убор",
  accessory: "Аксессуар",
  background: "Фон",
  frame: "Рамка",
};
const slot = z.enum(slots),
  integer = z.number().int();
export const characterSchema = z.object({
  recalculating: z.boolean(),
  version: integer.positive(),
  catalog_version: z.string(),
  equipment: z.object({
    base: z.string(),
    clothes: z.string(),
    head: z.string(),
    accessory: z.string(),
    background: z.string(),
    frame: z.string(),
  }),
  title_id: z.string(),
  title: z.string(),
  titles: z.array(
    z.object({ id: z.string(), name: z.string(), requirement: z.string(), earned: z.boolean() }),
  ),
  records: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      value: integer.nullable(),
      unit: z.string(),
      available: z.boolean(),
      explanation: z.string(),
      sources: z.array(z.object({ title: z.string(), url: z.string().regex(/^\/app\//) })),
    }),
  ),
  level: progressionBalance.shape.level,
  inventory: z.array(
    z.object({
      id: z.string(),
      slot,
      name: z.string(),
      rarity: z.enum(["common", "uncommon", "rare"]),
      starter: z.boolean(),
      level: integer.positive(),
      achievement: z.string(),
      url: z.string().regex(/^\/api\/v1\/cosmetics\/v1\/[a-z0-9-]+\.svg$/),
      owned: z.boolean(),
      source: z.string(),
      revoked: z.boolean(),
    }),
  ),
  areas: z.array(
    z.object({
      id: z.uuid(),
      name: z.string(),
      xp: integer.nonnegative(),
      level: progressionBalance.shape.level,
      successful_periods: integer.nonnegative(),
      archived: z.boolean(),
      skills: z.array(
        z.object({
          id: z.string(),
          name: z.string(),
          metric: z.enum(["xp", "periods"]),
          threshold: integer.positive(),
          depends_on: z.array(z.string()),
          current: integer,
          unlocked: z.boolean(),
          explanation: z.string(),
        }),
      ),
    }),
  ),
});
export type CharacterProfile = z.infer<typeof characterSchema>;
export async function getCharacter(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/character", { signal });
  return characterSchema.parse(r.data?.data);
}
export function useCharacter() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "character"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getCharacter(signal),
    refetchInterval: 15000,
    staleTime: 0,
  });
}
export async function equipCharacter(
  p: CharacterProfile,
  slot: (typeof slots)[number],
  id: string,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.PATCH("/api/v1/character/equipment", {
      params: {
        ...authParams(),
        header: {
          ...authParams().header,
          "If-Match": '"' + p.version + '"',
          "Idempotency-Key": key,
        },
      },
      body: { slot, item_id: id },
    });
    return characterSchema.parse(r.data?.data);
  });
}

export async function selectCharacterTitle(p: CharacterProfile, id: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.PATCH("/api/v1/character/title", {
      params: {
        ...authParams(),
        header: {
          ...authParams().header,
          "If-Match": '"' + p.version + '"',
          "Idempotency-Key": key,
        },
      },
      body: { title_id: id },
    });
    return characterSchema.parse(r.data?.data);
  });
}
