import { render, screen, waitFor } from "@testing-library/react";
import { useQueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient, mutationHeaders, responseError } from "../src/lib/api/client";
import {
  AccountQueryProvider,
  accountQueryKey,
  createQueryClient,
} from "../src/lib/api/query-provider";
describe("central API boundary", () => {
  it.each([401, 409, 429, 503])(
    "throws on HTTP %s and retains safe diagnostics",
    async (status) => {
      const client = createApiClient({
        fetch: vi
          .fn()
          .mockResolvedValue(
            new Response(
              JSON.stringify({
                error: { code: "VERSION_CONFLICT", request_id: "req-12", current_version: 3 },
              }),
              { status, headers: { "Retry-After": "15" } },
            ),
          ),
      });
      await expect(client.GET("/health/ready")).rejects.toMatchObject({
        status,
        requestId: "req-12",
        currentVersion: 3,
        retryAfterSeconds: 15,
      });
    },
  );
  it("does not turn HTML or invalid JSON into a successful response", async () => {
    const client = createApiClient({
      fetch: vi.fn().mockResolvedValue(new Response("<h1>proxy password</h1>")),
    });
    await expect(client.GET("/health/live")).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const error = await responseError(new Response("private proxy exception", { status: 500 }));
    expect(error.message).not.toContain("private");
  });
  it("sends same-origin credentials and request ID, without following redirects", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({
            data: { status: "ok" },
            meta: { request_id: "r1", server_time: "2026-10-12T00:00:00Z" },
          }),
        ),
      );
    const result = await createApiClient({ fetch: fetcher }).GET("/health/live");
    expect(result.data?.data.status).toBe("ok");
    const request: Request = fetcher.mock.calls[0]![0];
    expect(request.credentials).toBe("same-origin");
    expect(request.redirect).toBe("error");
    expect(request.headers.get("X-Request-ID")).toMatch(/^[a-f0-9-]+$/);
  });
  it("rejects foreign origin overrides before fetch", async () => {
    const fetcher = vi.fn();
    const client = createApiClient({ fetch: fetcher });
    await expect(
      client.GET("/health/live", { baseUrl: "https://example.org" }),
    ).rejects.toMatchObject({ code: "INVALID_ORIGIN" });
    expect(fetcher).not.toHaveBeenCalled();
  });
  it("preserves one mutation key and validates record versions", () => {
    const key = crypto.randomUUID();
    expect(mutationHeaders(3, key)).toEqual({ "If-Match": '"3"', "Idempotency-Key": key });
    expect(() => mutationHeaders(0)).toThrow();
  });
  it("does not automatically retry authorization, conflict or rate limit errors", () => {
    const retry = createQueryClient().getDefaultOptions().queries!.retry as (
      count: number,
      error: Error,
    ) => boolean;
    for (const status of [401, 409, 429])
      expect(retry(0, new ApiError(status, "ERROR", "error"))).toBe(false);
  });
});
describe("account cache isolation", () => {
  it("remounts the cache on account changes", async () => {
    let previous: ReturnType<typeof useQueryClient> | undefined;
    function Consumer({ account }: { account: string }) {
      const client = useQueryClient();
      previous ??= client;
      return <p>{String(client.getQueryData(accountQueryKey(account, "private")) ?? "empty")}</p>;
    }
    const view = render(
      <AccountQueryProvider accountId="A">
        <Consumer account="A" />
      </AccountQueryProvider>,
    );
    previous!.setQueryData(accountQueryKey("A", "private"), "private A");
    view.rerender(
      <AccountQueryProvider accountId="B">
        <Consumer account="B" />
      </AccountQueryProvider>,
    );
    expect(screen.getByText("empty")).toBeInTheDocument();
    await waitFor(() => expect(previous!.getQueryCache().getAll()).toHaveLength(0));
    expect(() => accountQueryKey("", "private")).toThrow();
  });
});
