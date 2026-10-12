"use client";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2 } from "lucide-react";
import { z } from "zod";
import { createApiClient, ApiError } from "@/lib/api/client";
import { ErrorState, LoadingState } from "./ui/states";
const health = z.object({ status: z.literal("ok") });
export function ServiceStatus() {
  const query = useQuery({
    queryKey: ["public", "readiness"],
    queryFn: async ({ signal }) => {
      const result = await createApiClient().GET("/health/ready", { signal });
      const parsed = health.safeParse(result.data?.data);
      if (!parsed.success)
        throw new ApiError(
          502,
          "INVALID_RESPONSE",
          "Сервер пока не готов. Повторите проверку позже.",
        );
      return parsed.data;
    },
    retry: false,
    refetchOnWindowFocus: false,
  });
  if (query.isPending) return <LoadingState label="Проверяем соединение…" />;
  if (query.isError)
    return (
      <ErrorState
        error={query.error}
        retry={() => {
          void query.refetch();
        }}
      />
    );
  return (
    <p className="service-ready" role="status">
      <CheckCircle2 size={18} aria-hidden="true" />
      Соединение с сервером установлено
    </p>
  );
}
