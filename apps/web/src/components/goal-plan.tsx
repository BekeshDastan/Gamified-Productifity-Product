"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import {
  getGoalPlan,
  questGoal,
  saveMilestone,
  saveGoalLink,
  type Milestone,
  type LinkInput,
} from "@/lib/goal-plan";
import type { Goal } from "@/lib/goals";
import { statuses } from "@/lib/quests";
import { QuestPicker } from "./quest-structure";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
export function GoalPlanPanel({ goal, onChanged }: { goal: Goal; onChanged: () => void }) {
  const account = useAuth().session?.account.id;
  const query = useQuery({
    queryKey: ["account", account, "goal-plan", goal.id],
    queryFn: ({ signal }) => getGoalPlan(goal.id, signal),
  });
  if (query.isPending) return <p role="status">Загружаем план…</p>;
  if (!query.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить план.</p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  const plan = query.data;
  const editable = !goal.archived_at && ["draft", "active", "paused"].includes(goal.state);
  const content = (
    <>
      <Card>
        <h2>Этапы цели</h2>
        <p className="ui-hint">
          {goal.progress_type === "milestones"
            ? "Сводка учитывает обязательные этапы с их весами. Если у этапа есть задачи, прогресс берётся из обязательных листьев."
            : goal.progress_type === "quests"
              ? "Этапы группируют задачи. Процент считается по весам обязательных листьев; вес этапа дополнительно не умножается."
              : "Этапы помогают организовать работу. Процент этой цели считается только по числовому журналу."}
        </p>
        {plan.milestones.length === 0 && <p>Этапы пока не добавлены.</p>}
        {plan.milestones.map((m) => (
          <div className="goal-plan-part" key={m.id}>
            <h3>{m.title}</h3>
            <p>
              {m.required ? "Обязательный" : "Необязательный"} · вес {m.weight}
              {m.completed ? " · Отмечен" : ""}
            </p>
            {editable && (
              <details>
                <summary>Изменить этап</summary>
                <MilestoneForm
                  key={m.id + ":" + m.version}
                  goal={goal}
                  current={m}
                  onChanged={onChanged}
                />
              </details>
            )}
          </div>
        ))}
        {editable && <MilestoneForm goal={goal} onChanged={onChanged} />}
      </Card>
      <Card>
        <h2>Задачи цели</h2>
        <p className="ui-hint">
          Контейнер учитывается через обязательные листья. Повторная связь с родителем или потомком
          запрещена. Вес связанного контейнера применяется к каждому его листу.
        </p>
        <ul className="quest-batch">
          {plan.links.map((l) => (
            <li className="goal-plan-part" key={l.quest_id}>
              <SafeLink className="text-link" href={"/app/quests/" + l.quest_id}>
                {l.title}
              </SafeLink>
              <p>
                {statuses[l.status as keyof typeof statuses] ?? l.status} · вес {l.weight} ·{" "}
                {l.required ? "Обязательная" : "Необязательная"}
                {l.milestone_id
                  ? " · " + plan.milestones.find((m) => m.id === l.milestone_id)?.title
                  : ""}
              </p>
              {editable && <RemoveLink goal={goal} id={l.quest_id} onChanged={onChanged} />}
            </li>
          ))}
        </ul>
        {editable && <LinkForm goal={goal} milestones={plan.milestones} onChanged={onChanged} />}
      </Card>
    </>
  );
  return goal.progress_type === "metric" ? (
    <details>
      <summary>Вспомогательные этапы и задачи</summary>
      {content}
    </details>
  ) : (
    content
  );
}
const milestoneFields = z.object({
  title: z.string().trim().min(1, "Укажите название").max(200),
  weight: z.number().positive().max(1000),
  required: z.boolean(),
  completed: z.boolean(),
  reason: z.string().max(1000),
});
function MilestoneForm({
  goal,
  current,
  onChanged,
}: {
  goal: Goal;
  current?: Milestone;
  onChanged: () => void;
}) {
  const [remove, setRemove] = useState(false);
  const flow = useValidatedForm({
    schema: milestoneFields,
    defaultValues: current ?? {
      title: "",
      weight: 1,
      required: true,
      completed: false,
      reason: "",
    },
    fields: ["title", "weight", "required", "completed", "reason"],
    save: async (v) => {
      await saveMilestone(goal, v, current, remove);
      onChanged();
      return current ? v : { title: "", weight: 1, required: true, completed: false, reason: "" };
    },
  });
  const f = flow.form;
  const e = f.formState.errors;
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label={current ? "Название этапа" : "Название нового этапа"}
        maxLength={200}
        error={e.title?.message}
        {...f.register("title")}
      />
      <Input
        label="Вес этапа"
        type="number"
        step="0.001"
        error={e.weight?.message}
        {...f.register("weight", { valueAsNumber: true })}
      />
      <label className="check-row">
        <input type="checkbox" {...f.register("required")} />
        Обязательный этап
      </label>
      <label className="check-row">
        <input type="checkbox" {...f.register("completed")} />
        Этап выполнен вручную
      </label>
      <Input
        label="Причина изменения этапа"
        maxLength={1000}
        error={e.reason?.message}
        {...f.register("reason")}
      />
      <div className="button-row">
        <Button type="submit" variant="secondary" onClick={() => setRemove(false)}>
          {current ? "Сохранить этап" : "Добавить этап"}
        </Button>
        {current && (
          <Button type="submit" variant="ghost" onClick={() => setRemove(true)}>
            Удалить этап
          </Button>
        )}
      </div>
    </Form>
  );
}
const linkFields = z.object({
  quest_id: z.uuid("Выберите задачу"),
  milestone_id: z.string(),
  weight: z.number().positive().max(1000),
  required: z.boolean(),
  reason: z.string().max(1000),
  confirm_transfer: z.boolean(),
});
function LinkForm({
  goal,
  milestones,
  onChanged,
}: {
  goal: Goal;
  milestones: Milestone[];
  onChanged: () => void;
}) {
  const account = useAuth().session?.account.id;
  const [selected, setSelected] = useState("");
  const assignment = useQuery({
    queryKey: ["account", account, "quest-goal", selected],
    enabled: !!selected,
    queryFn: ({ signal }) => questGoal(selected, signal),
  });
  const flow = useValidatedForm({
    schema: linkFields,
    defaultValues: {
      quest_id: "",
      milestone_id: "",
      weight: 1,
      required: true,
      reason: "",
      confirm_transfer: false,
    },
    fields: ["quest_id", "milestone_id", "weight", "required", "reason", "confirm_transfer"],
    save: async (v) => {
      const a = assignment.data;
      if (!a) throw new Error("Дождитесь загрузки связи");
      if (a.goal_id && a.goal_id !== goal.id && !v.confirm_transfer)
        throw new Error("Подтвердите перенос");
      await saveGoalLink(goal, v.quest_id, {
        milestone_id: v.milestone_id || null,
        weight: v.weight,
        required: v.required,
        previous_goal_id: a.goal_id,
        previous_goal_version: a.goal_version,
        reason: v.reason,
      });
      setSelected("");
      onChanged();
      return {
        quest_id: "",
        milestone_id: "",
        weight: 1,
        required: true,
        reason: "",
        confirm_transfer: false,
      };
    },
  });
  const f = flow.form;
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <QuestPicker
        label="Задача для цели"
        value={f.watch("quest_id")}
        onChange={(id) => {
          setSelected(id);
          f.setValue("quest_id", id, { shouldDirty: true });
          f.setValue("confirm_transfer", false);
        }}
      />
      {f.formState.errors.quest_id && <p role="alert">{f.formState.errors.quest_id.message}</p>}
      <Select label="Этап задачи" {...f.register("milestone_id")}>
        <option value="">Без этапа</option>
        {milestones.map((m) => (
          <option key={m.id} value={m.id}>
            {m.title}
          </option>
        ))}
      </Select>
      <Input
        label="Вес задачи в цели"
        type="number"
        step="0.001"
        {...f.register("weight", { valueAsNumber: true })}
      />
      <label className="check-row">
        <input type="checkbox" {...f.register("required")} />
        Обязательная задача цели
      </label>
      {assignment.isError && (
        <p role="alert">
          Не удалось проверить прежнюю связь.{" "}
          <Button onClick={() => void assignment.refetch()}>Повторить</Button>
        </p>
      )}
      {assignment.data?.goal_id && assignment.data.goal_id !== goal.id && (
        <>
          <p>Сейчас задача относится к цели «{assignment.data.goal_title}».</p>
          <label className="check-row">
            <input type="checkbox" {...f.register("confirm_transfer")} />
            Перенести задачу в эту цель
          </label>
        </>
      )}
      <Input label="Причина изменения связи" maxLength={1000} {...f.register("reason")} />
      <Button type="submit" disabled={!assignment.data}>
        Связать задачу с целью
      </Button>
    </Form>
  );
}
function RemoveLink({ goal, id, onChanged }: { goal: Goal; id: string; onChanged: () => void }) {
  const flow = useValidatedForm({
    schema: z.object({ reason: z.string().trim().min(1, "Укажите причину").max(1000) }),
    defaultValues: { reason: "" },
    fields: ["reason"],
    save: async (v) => {
      const input: LinkInput = {
        milestone_id: null,
        weight: 1,
        required: true,
        previous_goal_id: null,
        previous_goal_version: 0,
        reason: v.reason,
      };
      await saveGoalLink(goal, id, input, true);
      onChanged();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Причина снятия связи"
        error={flow.form.formState.errors.reason?.message}
        {...flow.form.register("reason")}
      />
      <Button type="submit" variant="ghost">
        Убрать из цели
      </Button>
    </Form>
  );
}
export function QuestGoalLink({ id }: { id: string }) {
  const account = useAuth().session?.account.id;
  const discard = useConfirmDiscard();
  const q = useQuery({
    queryKey: ["account", account, "quest-goal", id],
    queryFn: ({ signal }) => questGoal(id, signal),
  });
  return (
    <Card>
      <h2>Связанная цель</h2>
      {q.isPending ? (
        <p role="status">Загружаем связь…</p>
      ) : q.data?.goal_id ? (
        <SafeLink className="text-link" href={"/app/goals/" + q.data.goal_id}>
          {q.data.goal_title}
        </SafeLink>
      ) : q.isError ? (
        <p role="alert">
          Связь не загрузилась.{" "}
          <Button
            onClick={() => {
              if (discard()) void q.refetch();
            }}
          >
            Повторить
          </Button>
        </p>
      ) : (
        <p>Задача пока не связана с целью. Добавить связь можно в плане цели.</p>
      )}
    </Card>
  );
}
