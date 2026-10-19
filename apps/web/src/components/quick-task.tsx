"use client";
import { useLayoutEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { Plus, Check } from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { useSettings, getAccountToday } from "@/lib/settings";
import { emptyQuest, saveQuest, type Quest } from "@/lib/quests";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { Form } from "./ui/form";
const schema = z.object({
  title: z.string().trim().min(1, "Напишите, что хотите сделать").max(200, "До 200 символов"),
});
export function QuickTask({
  date,
  onCreated,
  onStateChange,
  first = false,
}: {
  date?: string;
  onCreated?: (quest: Quest) => void;
  onStateChange?: (state: { dirty: boolean; saving: boolean }) => void;
  first?: boolean;
}) {
  const settings = useSettings(),
    auth = useAuth(),
    cache = useQueryClient();
  const [created, setCreated] = useState<Quest>();
  const flow = useValidatedForm({
    schema,
    defaultValues: { title: "" },
    fields: ["title"],
    save: async ({ title }) => {
      const day = date ?? (await getAccountToday());
      const quest = await saveQuest({ ...emptyQuest, title, status: "todo", planned_date: day });
      setCreated(quest);
      void cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
      onCreated?.(quest);
      return { title: "" };
    },
  });
  const dirty = flow.form.formState.isDirty;
  // The close guard must see a new draft before the next Escape/click event.
  useLayoutEffect(() => {
    onStateChange?.({ dirty, saving: flow.saving });
  }, [dirty, flow.saving, onStateChange]);
  if (!auth.session)
    return (
      <p>
        Для сохранения задачи{" "}
        <SafeLink className="text-link" href="/auth/login?next=/app">
          войдите в аккаунт
        </SafeLink>
        .
      </p>
    );
  if (!date && !settings.data)
    return (
      <p role={settings.isError ? "alert" : "status"}>
        {settings.isError ? (
          <>
            Не удалось узнать ваш часовой пояс.{" "}
            <Button variant="secondary" onClick={() => void settings.refetch()}>
              Повторить
            </Button>
          </>
        ) : (
          "Готовим добавление задачи…"
        )}
      </p>
    );
  return (
    <div className="quick-task">
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label={first ? "Ваш первый шаг" : "Быстро добавить задачу"}
          placeholder="Например, прочитать 10 страниц"
          maxLength={200}
          error={flow.form.formState.errors.title?.message}
          {...flow.form.register("title")}
        />
        <Button type="submit">
          <Plus size={18} aria-hidden="true" />
          Добавить на сегодня
        </Button>
      </Form>
      {created && (
        <p role="status" className="save-notice">
          <Check size={18} aria-hidden="true" />
          Добавлено на сегодня: {created.title}.{" "}
          <SafeLink className="text-link" href={`/app/quests/${created.id}`}>
            Открыть задачу
          </SafeLink>
        </p>
      )}
    </div>
  );
}
