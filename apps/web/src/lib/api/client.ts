import createClient from "openapi-fetch";
import type { paths } from "./schema";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
    public readonly fields: Record<string, string> = {},
    public readonly currentVersion?: number,
    public readonly retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
const record = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
function safeId(value: unknown) {
  return typeof value === "string" && /^[A-Za-z0-9._-]{1,64}$/.test(value) ? value : undefined;
}
function retryAfter(value: string | null) {
  if (!value) return undefined;
  const seconds = /^\d+$/.test(value)
    ? Number(value)
    : Math.ceil((Date.parse(value) - Date.now()) / 1000);
  return Number.isFinite(seconds) ? Math.min(3600, Math.max(0, seconds)) : undefined;
}
export async function responseError(response: Response): Promise<ApiError> {
  let body: unknown;
  try {
    body = await response.clone().json();
  } catch {
    /* HTML/proxy errors must not enter the UI. */
  }
  const error = record(body) && record(body.error) ? body.error : {};
  const fields: Record<string, string> = {};
  if (record(error.fields))
    for (const [key, value] of Object.entries(error.fields)) {
      if (/^[a-zA-Z0-9_.]{1,100}$/.test(key) && typeof value === "string")
        fields[key] = value.slice(0, 500);
    }
  const message =
    response.status === 401
      ? "Сессия завершена. Войдите снова."
      : response.status === 409
        ? "Запись изменилась. Обновите данные перед сохранением."
        : response.status === 429
          ? "Слишком много попыток. Попробуйте позже."
          : response.status === 422
            ? "Проверьте заполненные поля."
            : "Не удалось выполнить запрос. Попробуйте снова.";
  return new ApiError(
    response.status,
    typeof error.code === "string" && /^[A-Z_]{1,80}$/.test(error.code) ? error.code : "HTTP_ERROR",
    message,
    safeId(error.request_id) ?? safeId(response.headers.get("X-Request-ID")),
    fields,
    typeof error.current_version === "number" &&
      Number.isSafeInteger(error.current_version) &&
      error.current_version > 0
      ? error.current_version
      : undefined,
    retryAfter(response.headers.get("Retry-After")),
  );
}

// Browser boundary: cookies only go to this origin; all business logic stays in Go.
export function createApiClient(
  options: { fetch?: typeof fetch; csrfToken?: () => string | undefined } = {},
) {
  if (typeof window === "undefined")
    throw new Error("Use a request-scoped server API client for SSR");
  const origin = window.location.origin;
  const client = createClient<paths>({
    baseUrl: origin,
    credentials: "same-origin",
    cache: "no-store",
    redirect: "error",
    fetch: options.fetch ?? globalThis.fetch,
  });
  client.use({
    onRequest({ request }) {
      if (new URL(request.url).origin !== origin)
        throw new ApiError(0, "INVALID_ORIGIN", "Адрес запроса недопустим.");
      request.headers.set("X-Request-ID", crypto.randomUUID());
      request.headers.set("Accept", "application/json");
      if (!["GET", "HEAD", "OPTIONS"].includes(request.method)) {
        const csrf = options.csrfToken?.();
        if (!csrf)
          throw new ApiError(401, "CSRF_UNAVAILABLE", "Обновите страницу перед сохранением.");
        request.headers.set("X-CSRF-Token", csrf);
      }
      return request;
    },
    async onResponse({ response }) {
      if (!response.ok) throw await responseError(response);
      if (response.status !== 204) {
        let body: unknown;
        try {
          body = await response.clone().json();
        } catch {
          throw new ApiError(
            502,
            "INVALID_RESPONSE",
            "Сервер вернул неполный ответ.",
            safeId(response.headers.get("X-Request-ID")),
          );
        }
        if (
          !record(body) ||
          !("data" in body) ||
          !record(body.meta) ||
          !safeId(body.meta.request_id) ||
          typeof body.meta.server_time !== "string" ||
          !Number.isFinite(Date.parse(body.meta.server_time))
        )
          throw new ApiError(502, "INVALID_RESPONSE", "Сервер вернул неполный ответ.");
      }
    },
    onError({ error }) {
      if (error instanceof Error && error.name === "AbortError") return error;
      return error instanceof ApiError
        ? error
        : new ApiError(0, "NETWORK_ERROR", "Нет связи с сервером. Проверьте подключение.");
    },
  });
  return client;
}
export function mutationHeaders(version?: number, idempotencyKey = crypto.randomUUID()) {
  if (version !== undefined && (!Number.isSafeInteger(version) || version < 1))
    throw new Error("A positive record version is required");
  if (!/^[A-Za-z0-9_-]{16,128}$/.test(idempotencyKey)) throw new Error("Invalid idempotency key");
  return {
    "Idempotency-Key": idempotencyKey,
    ...(version === undefined ? {} : { "If-Match": `"${version}"` }),
  };
}
