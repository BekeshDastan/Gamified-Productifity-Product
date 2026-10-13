"use client";
import { z } from "zod";
export const rewardSchema = z.object({
  entitlement_id: z.string(),
  ledger_id: z.string(),
  source_kind: z.string(),
  source_key: z.string(),
  economy_version: z.string(),
  kind: z.string(),
  requested_xp: z.number().int(),
  awarded_xp: z.number().int(),
  awarded_coins: z.number().int(),
  xp_delta: z.number().int(),
  coins_delta: z.number().int(),
  total_xp: z.number().int(),
  available_coins: z.number().int(),
  coin_correction: z.number().int(),
  level_before: z.number().int(),
  level_after: z.number().int(),
  reason: z.string(),
});
export type Reward = z.infer<typeof rewardSchema>;
