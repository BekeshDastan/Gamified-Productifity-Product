"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DateTime } from "luxon";
import { useAuth } from "@/lib/auth/provider";
import { getHabit, getHabitPeriod } from "@/lib/habits";
import { HabitCard } from "./habits";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { SafeLink } from "@/lib/forms/unsaved-changes";
export function HabitSource({ id, periodID }: { id: string; periodID?: string }) {
  const account = useAuth().session?.account.id,
    cache = useQueryClient(),
    h = useQuery({
      queryKey: ["account", account, "habit-source", id],
      queryFn: () => getHabit(id),
    }),
    p = useQuery({
      queryKey: ["account", account, "habit-period-source", id, periodID],
      queryFn: ({ signal }) => getHabitPeriod(id, periodID!, signal),
      enabled: !!periodID,
    });
  return (
    <>
      <SafeLink href="/app/habits">Все привычки</SafeLink>
      {h.isError || p.isError ? (
        <Card>
          <p role="alert">
            Источник недоступен: привычка или период не найдены либо доступ отозван.
          </p>
          <Button
            onClick={() => {
              void h.refetch();
              if (periodID) void p.refetch();
            }}
          >
            Повторить
          </Button>
        </Card>
      ) : null}
      {p.data && (
        <Card>
          <h2>Период, учтённый в испытании</h2>
          <p>
            {DateTime.fromISO(p.data.start).setZone(p.data.timezone).toFormat("dd.MM.yyyy HH:mm")} —{" "}
            {DateTime.fromISO(p.data.end).setZone(p.data.timezone).toFormat("dd.MM.yyyy HH:mm")} ·{" "}
            {p.data.timezone}
          </p>
          <p>
            {p.data.amount} / {p.data.target} {h.data?.unit} ·{" "}
            {p.data.excluded
              ? "Отдых"
              : p.data.state === "success"
                ? "Норма выполнена"
                : p.data.state === "missed"
                  ? "Пропуск"
                  : "Период открыт"}
            . Версия расписания: {p.data.schedule_version}.
          </p>
        </Card>
      )}
      {h.data && (
        <HabitCard
          habit={h.data}
          changed={() => void cache.invalidateQueries({ queryKey: ["account", account] })}
        />
      )}
    </>
  );
}
