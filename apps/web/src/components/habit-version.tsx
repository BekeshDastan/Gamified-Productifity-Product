"use client";
import { useEffect, type RefObject } from "react";
import type { FieldValues, UseFormReturn } from "react-hook-form";
import { getHabit, type Habit } from "@/lib/habits";
import { Button } from "./ui/button";
export function HabitVersion<T extends FieldValues>({
  recordRef,
  habit,
  flow,
  frozen = false,
}: {
  frozen?: boolean;
  recordRef: RefObject<Habit>;
  habit: Habit;
  flow: { form: UseFormReturn<T, unknown, T>; saving: boolean; error?: string };
}) {
  const { isDirty } = flow.form.formState;
  useEffect(() => {
    if (
      !frozen &&
      !isDirty &&
      !flow.saving &&
      !flow.error &&
      habit.version >= recordRef.current.version
    )
      recordRef.current = habit;
  }, [habit, isDirty, flow.saving, flow.error, recordRef, frozen]);
  if (flow.form.formState.errors.root?.server?.type !== "VERSION_CONFLICT") return null;
  return (
    <Button
      type="button"
      variant="secondary"
      onClick={() =>
        void getHabit(habit.id)
          .then((fresh) => {
            recordRef.current = fresh;
            flow.form.clearErrors("root.server");
          })
          .catch(() =>
            flow.form.setError("root.server", {
              type: "VERSION_CONFLICT",
              message: "Не удалось обновить. Ваш ввод сохранён.",
            }),
          )
      }
    >
      Загрузить свежую версию, сохранив ввод
    </Button>
  );
}
