"use client";
import { useRef, useState } from "react";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { goalInput, goalStates, type Goal } from "@/lib/goals";
import { statuses } from "@/lib/quests";
import { goalActions, goalHistory, transitionGoal, type GoalAction } from "@/lib/goal-lifecycle";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
const actionSchema = z.object({
  action: z.enum([
    "resume",
    "pause",
    "achieve",
    "close-unachieved",
    "cancel",
    "review",
    "archive",
    "restore",
  ]),
  reason: z.string().trim().min(1, "Укажите причину решения").max(1000),
  completed: z.string().max(2000),
  obstacles: z.string().max(2000),
  criteria_confirmed: z.boolean(),
  resolution: z.enum(["continue", "rebuild", "close"]),
  title: z.string().min(1).max(200),
  criteria: z.string().min(1).max(2000),
  target_date: z.string(),
});
export function GoalDecisions({ goal, onChanged }: { goal: Goal; onChanged: () => void }) {
  const closed = ["achieved", "not_achieved", "cancelled"].includes(goal.state);
  const choices: GoalAction[] = goal.archived_at
    ? ["restore"]
    : [
        ...(goal.state === "draft" || goal.state === "paused" ? ["resume" as const] : []),
        ...(goal.state === "active" ? ["pause" as const, "achieve" as const] : []),
        ...(goal.state === "active" || goal.state === "paused"
          ? ["close-unachieved" as const]
          : []),
        ...(!closed ? ["cancel" as const] : []),
        ...(goal.state !== "draft" ? ["review" as const] : []),
        ...(closed ? ["archive" as const] : []),
      ];
  const initial = useRef(goal);
  const pending = useRef<{ body: string; key: string } | undefined>(undefined);
  const [message, setMessage] = useState("");
  const flow = useValidatedForm({
    schema: actionSchema,
    defaultValues: {
      action: choices[0] ?? "cancel",
      reason: "",
      completed: "",
      obstacles: "",
      criteria_confirmed: false,
      resolution: "continue",
      title: goal.title,
      criteria: goal.criteria,
      target_date: goal.target_date ?? "",
    },
    fields: ["reason", "completed", "obstacles", "criteria_confirmed", "resolution"],
    save: async (values) => {
      const body = {
        reason: values.reason,
        completed: values.completed,
        obstacles: values.obstacles,
        criteria_confirmed: values.criteria_confirmed,
        resolution: values.action === "review" ? values.resolution : "",
        next_input:
          values.action === "review" && values.resolution !== "close"
            ? {
                ...goalInput.parse(initial.current),
                title: values.title,
                criteria: values.criteria,
                target_date: values.target_date || null,
              }
            : null,
      };
      const encoded = JSON.stringify({ action: values.action, body });
      if (pending.current?.body !== encoded)
        pending.current = { body: encoded, key: crypto.randomUUID() };
      await transitionGoal(initial.current, values.action, body, pending.current.key);
      setMessage("Решение сохранено.");
      onChanged();
    },
  });
  const action = flow.form.watch("action");
  const resolution = flow.form.watch("resolution");
  const f = flow.form;
  const errors = f.formState.errors;
  return (
    <Card>
      <h2>Решение по цели</h2>
      {goal.progress >= 100 && goal.state !== "achieved" && !closed && (
        <p>Порог достигнут. Проверьте критерий и подтвердите результат.</p>
      )}
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Select label="Действие с целью" {...f.register("action")}>
          {choices.map((a) => (
            <option value={a} key={a}>
              {goalActions[a]}
            </option>
          ))}
        </Select>
        {action === "achieve" && (
          <label className="check-row">
            <input type="checkbox" {...f.register("criteria_confirmed")} />
            Подтверждаю: критерий «{goal.criteria}» выполнен
          </label>
        )}
        {errors.criteria_confirmed && <p role="alert">{errors.criteria_confirmed.message}</p>}
        {(action === "close-unachieved" || action === "review") && (
          <>
            <Input
              label="Что удалось выполнить"
              {...f.register("completed")}
              error={errors.completed?.message}
            />
            <Input
              label="Что помешало"
              {...f.register("obstacles")}
              error={errors.obstacles?.message}
            />
          </>
        )}
        {action === "review" && (
          <>
            <Select label="Как пересмотреть цель" {...f.register("resolution")}>
              <option value="continue">Продолжить с новой датой</option>
              <option value="rebuild">Перестроить будущий план</option>
              {closed && <option value="close">Закрыть и убрать в архив</option>}
            </Select>
            {resolution !== "close" && (
              <>
                <Input
                  label="Пересмотренное название"
                  {...f.register("title")}
                  error={errors.title?.message}
                />
                <Input
                  label="Пересмотренный критерий"
                  {...f.register("criteria")}
                  error={errors.criteria?.message}
                />
                <DatePicker label="Новая целевая дата" {...f.register("target_date")} />
                {resolution === "rebuild" && (
                  <p>
                    Связи и этапы будут сняты. Задачи, числовой журнал и прежние версии сохранятся.
                    Добавьте новый план после пересмотра.
                  </p>
                )}
              </>
            )}
          </>
        )}
        <Input label="Причина решения" {...f.register("reason")} error={errors.reason?.message} />
        <Button type="submit">Сохранить решение</Button>
      </Form>
      {message && <p role="status">{message}</p>}
    </Card>
  );
}
export function GoalHistory({ goal }: { goal: Goal }) {
  const account = useAuth().session?.account.id;
  const [before, setBefore] = useState<number>();
  const discard = useConfirmDiscard();
  const history = useQuery({
    queryKey: ["account", account, "goal-history", goal.id, goal.version, before],
    queryFn: ({ signal }) => goalHistory(goal.id, before, signal),
  });
  const kinds: Record<string, string> = {
    created: "Цель создана",
    edited: "Условия изменены",
    milestones_changed: "Этапы изменены",
    links_changed: "Связи изменены",
    quest_transferred: "Задача перенесена",
    metric_added: "Результат добавлен",
    metric_corrected: "Результат исправлен",
    template_applied: "Шаблон применён",
    "goal.achievement_reverted": "Достижение отменено: источники исправлены",
    ...goalActions,
  };
  return (
    <Card>
      <h2>История цели</h2>
      {history.isPending ? (
        <p role="status">Загружаем историю…</p>
      ) : history.data ? (
        <>
          <ol className="quest-batch">
            {history.data.map((v) => (
              <li key={v.id} className="goal-plan-part">
                <h3>
                  Версия {v.goal_version} · {kinds[v.kind] ?? "Изменена связанная задача"}
                </h3>
                <p className="quest-description">{v.reason || "Без дополнительного пояснения"}</p>
                <details>
                  <summary>Условия и план этой версии</summary>
                  <p>
                    {v.snapshot.title} · {goalStates[v.snapshot.state]} ·{" "}
                    {Number(v.snapshot.progress.toFixed(2))}%
                  </p>
                  <p className="quest-description">{v.snapshot.criteria}</p>
                  <p>
                    {v.snapshot.start_date ?? "Начало не указано"} —{" "}
                    {v.snapshot.target_date ?? "Дата не указана"}
                  </p>
                  {v.snapshot.progress_type === "metric" && (
                    <p>
                      {v.snapshot.current} / {v.snapshot.target} {v.snapshot.unit}
                    </p>
                  )}
                  <ul>
                    {v.snapshot.plan.milestones.map((m) => (
                      <li key={m.id}>
                        {m.title} · вес {m.weight} ·{" "}
                        {m.required ? "обязательный" : "необязательный"}
                      </li>
                    ))}
                  </ul>
                  <ul>
                    {v.snapshot.plan.links.map((l) => (
                      <li key={l.quest_id}>
                        {l.title} · вес {l.weight} ·{" "}
                        {statuses[l.status as keyof typeof statuses] ?? l.status}
                      </li>
                    ))}
                  </ul>
                </details>
              </li>
            ))}
          </ol>
          {history.data.length === 50 && (
            <Button
              onClick={() => {
                if (discard()) setBefore(history.data.at(-1)?.id);
              }}
            >
              Ранние версии
            </Button>
          )}
          {before && (
            <Button
              onClick={() => {
                if (discard()) setBefore(undefined);
              }}
            >
              Последние версии
            </Button>
          )}
        </>
      ) : (
        <p role="alert">
          История не загрузилась. <Button onClick={() => void history.refetch()}>Повторить</Button>
        </p>
      )}
    </Card>
  );
}
