"use client";
import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { confirmFocus, getQuestFocusTime, type FocusSession } from "@/lib/focus";
import { duration } from "@/lib/focus-clock";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Form } from "./ui/form";
import { Select } from "./ui/field";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
const review = z.object({
  result: z.enum(["progress", "no_progress", "rest"]),
  comment: z.string().trim().max(2000),
});
export function FocusReview({ session, saved }: { session: FocusSession; saved: () => void }) {
  const snapshot = useRef(session),
    key = useRef({ body: "", value: "" });
  const flow = useValidatedForm<z.infer<typeof review>>({
    schema: review,
    defaultValues: { result: session.active_seconds > 0 ? "progress" : "no_progress", comment: "" },
    fields: ["result", "comment"],
    save: async (values) => {
      const body = JSON.stringify(values);
      if (key.current.body !== body) key.current = { body, value: crypto.randomUUID() };
      await confirmFocus(snapshot.current, values.result, values.comment, key.current.value);
      saved();
    },
  });
  return (
    <>
      <h3>Итог сессии</h3>
      <p>
        Работа по интервалам: {duration(session.active_seconds)}. Перерывы{" "}
        {duration(session.break_seconds)} исключены. Отметка отдыха сохранится в истории и не войдёт
        в рабочий итог задачи.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Select
          label="Результат фокус-сессии"
          {...flow.form.register("result")}
          error={flow.form.formState.errors.result?.message}
        >
          <option value="progress">Есть результат</option>
          <option value="no_progress">Пока без результата</option>
          <option value="rest">Это был отдых</option>
        </Select>
        <label>
          Комментарий к фокус-сессии
          <textarea
            rows={3}
            maxLength={2000}
            className="ui-input focus-comment"
            {...flow.form.register("comment")}
          />
        </label>
        {flow.form.formState.errors.comment?.message && (
          <p role="alert">{flow.form.formState.errors.comment.message}</p>
        )}
        <p className="ui-hint">
          Для фокус-достижений учитываются подтверждённые сессии с результатом и не менее пяти минут
          работы. XP за таймер не добавляется.
        </p>
        <Button type="submit">Подтвердить итог сессии</Button>
      </Form>
    </>
  );
}
export function QuestFocusTime({ questId }: { questId: string }) {
  const auth = useAuth(),
    q = useQuery({
      queryKey: ["account", auth.session?.account.id, "quest-focus-time", questId],
      enabled: !!auth.session,
      queryFn: ({ signal }) => getQuestFocusTime(questId, signal),
      refetchInterval: 15000,
    });
  return (
    <Card>
      <h2>Фактическое время задачи</h2>
      {q.data ? (
        <>
          <p>
            В итог задачи: <strong>{duration(q.data.effective_seconds)}</strong>.
          </p>
          <p>
            Источник:{" "}
            {q.data.overridden
              ? "ручной итог заменяет сумму фокуса"
              : "подтверждённые фокус-сессии"}
            .
          </p>
          <p>
            Сохранённый фокус: {duration(q.data.focus_seconds)} · сессий {q.data.confirmed_sessions}
            . Ручная запись: {q.data.manual_minutes} мин.
          </p>
          <p className="ui-hint">
            Источники исключают друг друга. Выбор источника находится в параметрах задачи; история
            сессий сохраняется.
          </p>
        </>
      ) : (
        <p role={q.isError ? "alert" : "status"}>
          {q.isError ? "Не удалось загрузить фактическое время." : "Загружаем источники времени…"}
        </p>
      )}
      {q.isError && <Button onClick={() => void q.refetch()}>Повторить загрузку времени</Button>}
      <SafeLink href="/app/focus">Открыть фокус и историю</SafeLink>
    </Card>
  );
}
