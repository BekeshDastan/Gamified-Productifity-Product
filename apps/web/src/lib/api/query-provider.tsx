"use client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { ApiError } from "./client";

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        retry: (count, error) =>
          count < 1 && (!(error instanceof ApiError) || error.status === 0 || error.status >= 500),
      },
      mutations: { retry: false },
    },
  });
}
export function accountQueryKey(accountId: string, ...parts: readonly unknown[]) {
  if (!accountId.trim()) throw new Error("Private queries require an account ID");
  return ["account", accountId, ...parts] as const;
}
function Scope({ children }: { children: ReactNode }) {
  const [client] = useState(createQueryClient);
  useEffect(
    () => () => {
      void client.cancelQueries();
      client.clear();
    },
    [client],
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
// A new identity always gets a new in-memory client; old requests cannot populate its cache.
export function AccountQueryProvider({
  accountId,
  children,
}: {
  accountId: string | null;
  children: ReactNode;
}) {
  return <Scope key={accountId ?? "public"}>{children}</Scope>;
}
