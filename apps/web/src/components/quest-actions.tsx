"use client";
import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  actOnQuest,
  questHistory,
  statuses,
  transitionSchema,
  type Quest,
  type QuestAction,
  type TransitionFields,
} from "@/lib/quests";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useAuth } from "@/lib/auth/provider";
import { formatMoment, useSettings } from "@/lib/settings";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
const names: Record<QuestAction, string> = {
  start: "Начать",
  ready: "К выполнению",
  complete: "Завершить",
  reopen: "Открыть снова",
  postpone: "Перенести",
  skip: "Пропустить",
  fail: "Не получилось",
};
const available: Record<Quest["status"], QuestAction[]> = {
  backlog: ["ready", "start", "complete", "skip", "fail"],
  todo: ["start", "complete", "postpone", "skip", "fail"],
  in_progress: ["ready", "complete", "postpone", "skip", "fail"],
  postponed: ["ready", "start", "complete", "postpone", "skip", "fail"],
  completed: ["reopen"],
  failed: ["reopen"],
  skipped: ["reopen"],
};
export function QuestActions({ quest, onChanged }: { quest: Quest; onChanged: () => void }) {
  const [action, setAction] = useState<QuestAction>();
  const discard = useConfirmDiscard();
  return (
    <Card>
      <h2>Следующий шаг</h2>
      <div className="button-row">
        {available[quest.status].map((a) => (
          <Button
            disabled={!!quest.archived_at}
            variant={a === "complete" ? "primary" : "secondary"}
            key={a}
            onClick={() => {
              if (discard()) setAction(a);
            }}
          >
            {names[a]}
          </Button>
        ))}
      </div>
      {action && (
        <ActionForm
          key={action + quest.version}
          quest={quest}
          action={action}
          done={() => {
            setAction(undefined);
            onChanged();
          }}
        />
      )}
    </Card>
  );
}
function ActionForm({
  quest,
  action,
  done,
}: {
  quest: Quest;
  action: QuestAction;
  done: () => void;
}) {
  const retry = useRef<{ body: string; key: string }>(undefined);
  const flow = useValidatedForm<TransitionFields>({
    schema: transitionSchema,
    defaultValues: {
      reason: "",
      planned_date: "",
      return_to_backlog: false,
      shift_deadline: false,
      deadline: "",
    },
    fields: ["reason", "planned_date", "deadline"],
    save: async (values) => {
      const body = JSON.stringify(values);
      if (retry.current?.body !== body) retry.current = { body, key: crypto.randomUUID() };
      await actOnQuest(quest, action, values, retry.current.key);
      done();
    },
  });
  const f = flow.form;
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <h3>{names[action]}</h3>
      <Input
        label="Причина действия"
        maxLength={1000}
        hint={
          ["postpone", "skip", "fail"].includes(action)
            ? "Обязательна: останется в истории."
            : "Необязательно."
        }
        error={f.formState.errors.reason?.message}
        {...f.register("reason")}
      />
      {action === "postpone" && (
        <>
          <DatePicker
            label="Новая плановая дата"
            error={f.formState.errors.planned_date?.message}
            {...f.register("planned_date")}
          />
          <label className="check-row">
            <input type="checkbox" {...f.register("return_to_backlog")} />
            Вернуть в идеи без плановой даты
          </label>
          <label className="check-row">
            <input type="checkbox" {...f.register("shift_deadline")} />
            Подтверждаю отдельное изменение дедлайна
          </label>
          {f.watch("shift_deadline") && (
            <Input
              label="Новый дедлайн с часовым поясом"
              hint="ISO 8601 со смещением UTC. Пустое поле снимает дедлайн."
              error={f.formState.errors.deadline?.message}
              {...f.register("deadline")}
            />
          )}
          <p className="ui-hint">Без отдельного подтверждения дедлайн останется прежним.</p>
        </>
      )}
      <Button type="submit">Подтвердить действие</Button>
    </Form>
  );
}
export function QuestHistory({ quest }: { quest: Quest }) {
  const auth = useAuth();
  const settings = useSettings();
  const [before, setBefore] = useState<number>();
  const history = useQuery({
    queryKey: [
      "account",
      auth.session?.account.id,
      "quest-history",
      quest.id,
      quest.version,
      before,
    ],
    queryFn: ({ signal }) => questHistory(quest.id, before, signal),
  });
  return (
    <Card>
      <h2>История задачи</h2>
      {history.isPending && <p role="status">Загружаем историю…</p>}
      {history.isError && (
        <p role="alert">
          История не загрузилась. <Button onClick={() => void history.refetch()}>Повторить</Button>
        </p>
      )}
      <ol className="quest-history">
        {history.data?.map((e) => (
          <li key={e.id}>
            <strong>
              {(
                {
                  created: "Создана",
                  edited: "Изменена",
                  start: "Начата",
                  ready: "Готова к выполнению",
                  complete: "Завершена",
                  reopen: "Открыта снова",
                  postpone: "Перенесена",
                  skip: "Пропущена",
                  fail: "Отмечена неудача",
                  archive: "В архиве",
                  restore: "Восстановлена",
                  dependencies: "Изменены блокировки",
                  attachment_added: "Добавлен файл",
                  attachment_removed: "Удалён файл",
                  rollup_completed: "Выполнены обязательные подзадачи",
                  rollup_reopened: "Изменён прогресс подзадач",
                } as Record<string, string>
              )[e.kind] ?? "Изменение"}
            </strong>{" "}
            · версия {e.after.version}
            <p>
              {e.before ? statuses[e.before.status] + " → " : ""}
              {statuses[e.after.status]}
            </p>
            {e.reason && <p>{e.reason}</p>}
            {e.kind === "dependencies" && (
              <p>
                Блокировки: {e.details.previous_titles?.join(", ") || "нет"} →{" "}
                {e.details.titles?.join(", ") || "нет"}
              </p>
            )}
            {e.before && e.before.title !== e.after.title && (
              <p>
                Название: {e.before.title} → {e.after.title}
              </p>
            )}
            {e.kind === "edited" && (
              <details>
                <summary>Сохранённые поля</summary>
                <dl>
                  <dt>Описание</dt>
                  <dd className="quest-description">{e.after.description || "Не добавлено"}</dd>
                  <dt>Сложность</dt>
                  <dd>{e.after.difficulty}/5</dd>
                  <dt>Оценка / фактически</dt>
                  <dd>
                    {e.after.estimated_minutes} / {e.after.actual_minutes} мин.
                  </dd>
                </dl>
              </details>
            )}
            <p>
              План: {e.after.planned_date ?? "не задан"}; дедлайн:{" "}
              {e.after.deadline && settings.data
                ? formatMoment(e.after.deadline, settings.data)
                : "не задан"}
            </p>
            {settings.data && (
              <time dateTime={e.created_at}>{formatMoment(e.created_at, settings.data)}</time>
            )}
          </li>
        ))}
      </ol>
      <div className="button-row">
        {history.data?.length === 50 && (
          <Button variant="secondary" onClick={() => setBefore(history.data?.at(-1)?.id)}>
            Более ранние события
          </Button>
        )}
        {before && (
          <Button variant="secondary" onClick={() => setBefore(undefined)}>
            Последние события
          </Button>
        )}
      </div>
    </Card>
  );
}
