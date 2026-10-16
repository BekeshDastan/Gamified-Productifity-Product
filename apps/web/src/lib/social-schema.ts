import { z } from "zod";
const id = z.uuid();
const number = z.number().finite();
const integer = z.number().int().nonnegative();
const timestamp = z.iso.datetime({ offset: true });
export const friendSchema = z.object({
  id,
  user_id: id,
  nickname: z.string(),
  state: z.enum(["pending", "accepted"]),
  incoming: z.boolean(),
  version: integer,
});
export const friendsSchema = z.object({
  code: id,
  verified: z.boolean(),
  items: z.array(friendSchema),
});
export const grantSchema = z.object({
  owner: id,
  recipient: id,
  metric: z.enum(["regularity", "evening", "challenge"]),
  scope: z.string(),
  value: number,
  unit: z.string(),
});
export const standingSchema = z.object({
  user_id: id,
  nickname: z.string(),
  amount: integer,
  rank: integer,
  earned: z.boolean(),
  confirmation: z.enum(["recorded", "self_report", "partner_confirmed"]),
});
export const roomSchema = z.object({
  id,
  owner: id,
  name: z.string(),
  mode: z.enum(["coop", "pvp"]),
  metric: z.enum(["tasks", "focus", "reviews", "manual"]),
  unit: z.string(),
  target: integer,
  daily_cap: integer,
  starts_at: timestamp,
  ends_at: timestamp,
  rules_hash: z.string(),
  finalized_at: timestamp.nullable(),
  state: z.enum(["invited", "joined", "left", "rejected"]),
  share: z.boolean(),
  leaderboard: z.boolean(),
  version: integer,
  total: integer,
  members: z.array(standingSchema),
  checkin: z.object({ id, amount: integer, version: integer }).nullable(),
});
export const reviewSchema = z.object({
  id,
  checkin_id: id,
  owner: id,
  reviewer: id,
  room_id: id,
  room_name: z.string(),
  amount: integer,
  unit: z.string(),
  state: z.enum(["pending", "accepted", "rejected", "disputed", "withdrawn"]),
  version: integer,
  file: z.boolean(),
  matches_version: z.boolean(),
});
export const fileSchema = z.object({ id, name: z.string() });
export const communitySchema = z.object({
  partner: id.nullable(),
  nudges: z.boolean(),
  version: integer,
  feed: z.array(
    z.object({ id, owner: id, nickname: z.string(), title: z.string(), created_at: timestamp }),
  ),
  publishable: z.array(z.object({ key: z.string(), version: z.string(), title: z.string() })),
  inbox: z.array(
    z.object({
      id,
      template: z.enum(["small_step", "rest", "celebrate"]),
      nickname: z.string(),
      created_at: timestamp,
    }),
  ),
});
export const moderationSchema = z.object({
  blocks: z.array(z.object({ id, nickname: z.string() })),
  reports: z.array(
    z.object({ id, target: id, post_id: id.nullable(), reason: z.string(), resolved: z.boolean() }),
  ),
  admin: z.boolean(),
});
export const dashboardSchema = z.object({
  owner: id,
  nickname: z.string(),
  friends: friendsSchema,
  grants: z.array(grantSchema),
  rooms: z.array(roomSchema),
  reviews: z.array(reviewSchema),
  files: z.array(fileSchema),
  community: communitySchema,
  moderation: moderationSchema,
});
export const receiptSchema = z.object({ id, ok: z.boolean() });
export const socialInputs = {
  friends: z.object({
    action: z.enum(["request", "rotate", "accept", "reject", "remove"]),
    code: id.optional(),
    id: id.optional(),
    version: integer.optional(),
  }),
  grants: z.object({
    recipient: id,
    metric: z.enum(["regularity", "evening", "challenge"]),
    scope: z.string(),
    allow: z.boolean(),
  }),
  rooms: z.object({
    action: z.enum([
      "create",
      "invite",
      "join",
      "leave",
      "reject",
      "sharing",
      "checkin",
      "finalize",
    ]),
    id: id.optional(),
    name: z.string().max(80).optional(),
    mode: z.enum(["coop", "pvp"]).optional(),
    metric: z.enum(["tasks", "focus", "reviews", "manual"]).optional(),
    unit: z.enum(["tasks", "minutes", "reviews", "count", "pages"]).optional(),
    target: integer.optional(),
    daily_cap: integer.optional(),
    days: z.number().int().min(1).max(30).optional(),
    recipient: id.optional(),
    rules_hash: z.string().optional(),
    consent: z.boolean().optional(),
    leaderboard: z.boolean().optional(),
    version: integer.optional(),
    amount: integer.optional(),
  }),
  reviews: z.object({
    action: z.enum(["request", "accepted", "rejected", "disputed", "withdrawn"]),
    id,
    recipient: id.optional(),
    attachment_id: z.union([id, z.literal("")]).optional(),
    consent: z.boolean().optional(),
    version: integer.optional(),
  }),
  community: z.object({
    action: z.enum(["preferences", "nudge", "publish"]),
    recipient: z.union([id, z.literal("")]).optional(),
    enabled: z.boolean().optional(),
    version: integer.optional(),
    template: z.enum(["small_step", "rest", "celebrate"]).optional(),
    source_key: z.string().optional(),
    source_version: z.string().optional(),
    consent: z.boolean().optional(),
  }),
  moderation: z.object({
    action: z.enum(["block", "unblock", "hide", "report", "resolve"]),
    id: id.optional(),
    target: id.optional(),
    reason: z.enum(["spam", "abuse", "privacy", "other"]).optional(),
  }),
};
export type SocialCommand = {
  [K in keyof typeof socialInputs]: { section: K; input: z.infer<(typeof socialInputs)[K]> };
}[keyof typeof socialInputs];
export type SocialDashboard = z.infer<typeof dashboardSchema>;
export type SocialRoom = z.infer<typeof roomSchema>;
