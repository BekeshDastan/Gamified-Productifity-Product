import { z } from "zod";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { responseError } from "@/lib/api/client";
import { requireAccount } from "@/lib/profile";
import { dashboardSchema, receiptSchema, socialInputs, type SocialCommand } from "./social-schema";

const envelope = z.object({
  data: dashboardSchema,
  meta: z.object({ server_time: z.iso.datetime({ offset: true }), request_id: z.string() }),
});
export async function loadSocial(signal: AbortSignal) {
  await requireAccount();
  signal.throwIfAborted();
  const response = await fetch("/api/v1/social/dashboard", {
    credentials: "same-origin",
    cache: "no-store",
    redirect: "error",
    signal,
  });
  if (!response.ok) throw await responseError(response);
  return envelope.parse(await response.json());
}
export async function mutateSocial(command: SocialCommand, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    await authClient();
    const body = socialInputs[command.section].parse(command.input);
    const response = await fetch(`/api/v1/social/${command.section}`, {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      redirect: "error",
      headers: {
        ...authParams().header,
        "Content-Type": "application/json",
        "Idempotency-Key": key,
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw await responseError(response);
    return z.object({ data: receiptSchema }).parse(await response.json()).data;
  });
}
