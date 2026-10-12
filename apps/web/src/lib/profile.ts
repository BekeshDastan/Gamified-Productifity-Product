import { z } from "zod";
import { ApiError, createApiClient } from "@/lib/api/client";
import { loadSession } from "@/lib/auth/session";

export const avatars = ["mountain", "compass", "sprout", "sun", "moon", "star"] as const;
export const profileInput = z.object({
  name: z
    .string()
    .max(80, "До 80 символов")
    .refine((v) => !/[\u0000-\u001f\u007f]/.test(v), "Без управляющих символов"),
  nickname: z
    .string()
    .refine((v) => v === "" || /^[a-z0-9_]{3,24}$/.test(v), "От 3 до 24 символов: a–z, 0–9 и _"),
  avatar: z.enum(avatars),
});
export const profileSchema = profileInput.extend({
  owner_id: z.uuid(),
  version: z.number().int().positive(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
export type Profile = z.infer<typeof profileSchema>;
export async function requireAccount() {
  if (!(await loadSession())) throw new ApiError(401, "UNAUTHORIZED", "Войдите снова.");
}
export async function getProfile(signal?: AbortSignal) {
  await requireAccount();
  const result = await createApiClient().GET("/api/v1/me", { signal });
  return profileSchema.parse(result.data?.data);
}
