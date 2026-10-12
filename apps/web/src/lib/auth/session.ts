import { z } from "zod";
import { ApiError, createApiClient } from "@/lib/api/client";

export const sessionSchema = z.object({
  account: z.object({
    id: z.uuid(),
    email: z.email(),
    role: z.enum(["user", "admin"]),
    email_verified: z.boolean(),
    created_at: z.iso.datetime({ offset: true }),
  }),
  session_id: z.uuid(),
  expires_at: z.iso.datetime({ offset: true }),
});
export type Session = z.infer<typeof sessionSchema>;
let csrf: string | undefined;
let queue: Promise<unknown> = Promise.resolve();
let refresh: Promise<Session | null> | undefined;

// Serialize cookie changes across tabs where Web Locks is available. The
// server's five-second conflict rule remains the authority in every browser.
export function withAuthLock<T>(work: () => Promise<T>): Promise<T> {
  const result = queue
    .catch(() => undefined)
    .then(async () => {
      if (navigator.locks) return await navigator.locks.request("ascend-auth", work);
      return await work();
    });
  queue = result.catch(() => undefined);
  return result;
}
export async function authClient() {
  const client = createApiClient({ csrfToken: () => csrf });
  const response = await client.GET("/api/v1/auth/csrf");
  const parsed = z
    .object({ token: z.string().regex(/^[A-Za-z0-9_-]{43}$/) })
    .safeParse(response.data?.data);
  if (!parsed.success)
    throw new ApiError(502, "INVALID_RESPONSE", "Не удалось подготовить форму. Обновите страницу.");
  csrf = parsed.data.token;
  return client;
}
export function authParams() {
  if (!csrf) throw new ApiError(401, "CSRF_UNAVAILABLE", "Обновите страницу.");
  return { header: { "X-CSRF-Token": csrf, Origin: window.location.origin } };
}
export function parseSession(value: unknown): Session {
  const result = sessionSchema.safeParse(value);
  if (!result.success) throw new ApiError(502, "INVALID_RESPONSE", "Сервер вернул неполный ответ.");
  return result.data;
}
async function readSession(): Promise<Session | null> {
  try {
    const result = await createApiClient().GET("/api/v1/auth/session");
    return parseSession(result.data?.data);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return null;
    throw error;
  }
}
export async function loadSession(): Promise<Session | null> {
  const current = await readSession();
  if (current) return current;
  refresh ??= withAuthLock(async () => {
    const updated = await readSession();
    if (updated) return updated;
    try {
      const client = await authClient();
      const result = await client.POST("/api/v1/auth/refresh", { params: authParams(), body: {} });
      return parseSession(result.data?.data);
    } catch (error) {
      if (error instanceof ApiError && error.code === "REFRESH_IN_PROGRESS") {
        // Retry the protected read, never reuse the rotated token again.
        for (const delay of [100, 250, 500]) {
          await new Promise((resolve) => setTimeout(resolve, delay));
          const shared = await readSession();
          if (shared) return shared;
        }
        return null;
      }
      if (error instanceof ApiError && error.status === 401) return null;
      throw error;
    }
  }).finally(() => {
    refresh = undefined;
  });
  return refresh;
}

export function safeRedirect(value: string | null): string {
  if (
    !value ||
    !/^\/app(?:\/|\?|$)/.test(value) ||
    /[\\\u0000-\u0020]/.test(value) ||
    value.includes("%") ||
    value.includes("#")
  )
    return "/app";
  const url = new URL(value, "https://ascend.invalid");
  if (url.origin !== "https://ascend.invalid" || !/^\/app(?:\/|$)/.test(url.pathname))
    return "/app";
  return url.pathname + url.search;
}

export function authError(error: unknown, login = false): never {
  if (!(error instanceof ApiError)) throw error;
  const message =
    error.code === "TOKEN_INVALID"
      ? "Ссылка недействительна или её срок истёк. Запросите новую."
      : error.code === "EMAIL_TAKEN"
        ? "Этот адрес уже зарегистрирован. Войдите или восстановите пароль."
        : login && error.status === 401
          ? "Неверный email или пароль."
          : error.code === "CSRF_REJECTED"
            ? "Обновите страницу и повторите действие."
            : error.status === 429
              ? `Слишком много попыток. Повторите через ${error.retryAfterSeconds ?? 60} сек.`
              : error.message;
  throw new ApiError(
    error.status,
    error.code,
    message,
    error.requestId,
    error.fields,
    error.currentVersion,
    error.retryAfterSeconds,
  );
}
