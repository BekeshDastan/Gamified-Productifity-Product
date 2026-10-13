"use client";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { requireAccount } from "@/lib/profile";
export const achievementSchema = z.object({
  id: z.string(),
  version: z.string(),
  name: z.string(),
  description: z.string(),
  metric: z.string(),
  threshold: z.number().int().positive(),
  source_url: z.string().regex(/^\/app\//),
  current: z.number().int().nonnegative(),
  available: z.boolean(),
  active: z.boolean(),
  revoked: z.boolean(),
  awarded_at: z.iso.datetime({ offset: true }).nullable(),
});
export async function getAchievements(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/achievements", { signal });
  return z.array(achievementSchema).parse(r.data?.data);
}
export function useAchievements() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "achievements"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getAchievements(signal),
    staleTime: 0,
    refetchInterval: 15000,
  });
}
