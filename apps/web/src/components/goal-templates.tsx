"use client";
import { useRef, useState } from "react";
import { useFieldArray } from "react-hook-form";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import {
  goalTemplates,
  templateDraft,
  previewTemplate,
  applyTemplate,
  type GoalTemplate,
  type TemplateDraft,
  type TemplatePreview,
} from "@/lib/goal-templates";
import type { Goal } from "@/lib/goals";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
export function GoalTemplates({ onSaved }: { onSaved: (goal: Goal) => void }) {
  const account = useAuth().session?.account.id;
  const templates = useQuery({
    queryKey: ["account", account, "goal-templates"],
    queryFn: ({ signal }) => goalTemplates(signal),
  });
  const [selected, setSelected] = useState<GoalTemplate>();
  const discard = useConfirmDiscard();
  return (
    <Card>
      <h2>Начать с шаблона</h2>
      {templates.isPending ? (
        <p role="status">Загружаем шаблоны…</p>
      ) : templates.data ? (
        <>
          <div className="button-row">
            {templates.data.map((t) => (
              <Button
                key={t.id}
                variant="secondary"
                onClick={() => {
                  if (discard()) setSelected(t);
                }}
              >
                {t.name}
              </Button>
            ))}
          </div>
          {selected && <TemplateEditor key={selected.id} template={selected} onSaved={onSaved} />}
        </>
      ) : (
        <p role="alert">
          Шаблоны не загрузились.{" "}
          <Button onClick={() => void templates.refetch()}>Повторить</Button>
        </p>
      )}
    </Card>
  );
}
function TemplateEditor({
  template,
  onSaved,
}: {
  template: GoalTemplate;
  onSaved: (goal: Goal) => void;
}) {
  const [checked, setChecked] = useState(false);
  const [result, setResult] = useState<{ body: TemplateDraft; preview: TemplatePreview }>();
  const pending = useRef<{ body: string; key: string } | undefined>(undefined);
  const [applyError, setApplyError] = useState("");
  const [applying, setApplying] = useState(false);
  const [done, setDone] = useState(false);
  const flow = useValidatedForm({
    schema: templateDraft,
    defaultValues: {
      template_id: template.id,
      input: template.input,
      milestones: template.milestones,
      tasks: template.tasks,
      duration_days: 30,
      confirmed: false,
    },
    fields: ["input.title", "input.start_date", "duration_days", "input.criteria", "input.target"],
    save: async (body) => {
      setResult({ body, preview: await previewTemplate(body) });
      setChecked(false);
      setApplyError("");
    },
  });
  const milestones = useFieldArray({ control: flow.form.control, name: "milestones" });
  const tasks = useFieldArray({ control: flow.form.control, name: "tasks" });
  const register = flow.form.register;
  async function apply() {
    if (!result || !checked || applying || done || flow.form.formState.isDirty) return;
    setApplying(true);
    setApplyError("");
    try {
      const body = { ...result.body, confirmed: true };
      const encoded = JSON.stringify(body);
      if (pending.current?.body !== encoded)
        pending.current = { body: encoded, key: crypto.randomUUID() };
      const goal = await applyTemplate(body, pending.current.key);
      setDone(true);
      onSaved(goal);
    } catch (error) {
      setApplyError(
        error instanceof Error ? error.message : "Не удалось создать план. Попробуйте снова.",
      );
    } finally {
      setApplying(false);
    }
  }
  return (
    <div className="goal-plan-part">
      <h3>{template.name}</h3>
      <p>{template.description}</p>
      <Form saving={flow.saving || applying || done} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Название цели"
          {...register("input.title")}
          error={flow.form.formState.errors.input?.title?.message}
        />
        <Input label="Мотив" {...register("input.motive")} />
        <Input
          label="Критерий успеха"
          {...register("input.criteria")}
          error={flow.form.formState.errors.input?.criteria?.message}
        />
        <DatePicker
          label="Начало плана"
          {...register("input.start_date")}
          error={flow.form.formState.errors.input?.start_date?.message}
        />
        <Input
          label="Длительность в днях"
          type="number"
          min={1}
          max={3660}
          {...register("duration_days", { valueAsNumber: true })}
          error={flow.form.formState.errors.duration_days?.message}
        />
        {template.input.progress_type === "metric" && (
          <>
            <Input
              label="Начальное количество"
              type="number"
              step="0.001"
              {...register("input.start", { valueAsNumber: true })}
            />
            <Input
              label="Целевое количество"
              type="number"
              step="0.001"
              {...register("input.target", { valueAsNumber: true })}
            />
            <Input label="Единица результата" {...register("input.unit")} />
          </>
        )}
        <h3>Этапы плана</h3>
        {milestones.fields.map((m, i) => (
          <div key={m.id} className="goal-plan-part">
            <Input
              label={"Этап " + (i + 1)}
              {...register(("milestones." + i + ".title") as "milestones.0.title")}
            />
            <Input
              label={"Вес этапа " + (i + 1)}
              type="number"
              step="0.001"
              min={0.001}
              max={1000}
              {...register(("milestones." + i + ".weight") as "milestones.0.weight", {
                valueAsNumber: true,
              })}
            />
            <label className="check-row">
              <input
                type="checkbox"
                {...register(("milestones." + i + ".required") as "milestones.0.required")}
              />
              Обязательный этап
            </label>
          </div>
        ))}
        <h3>Будущие задачи</h3>
        {tasks.fields.map((t, i) => (
          <div key={t.id} className="goal-plan-part">
            <Input
              label={"Задача " + (i + 1)}
              {...register(("tasks." + i + ".title") as "tasks.0.title")}
            />
            <Input
              label={"Описание задачи " + (i + 1)}
              {...register(("tasks." + i + ".description") as "tasks.0.description")}
            />
            <Input
              label={"Дней после начала для задачи " + (i + 1)}
              type="number"
              min={0}
              {...register(("tasks." + i + ".day_offset") as "tasks.0.day_offset", {
                valueAsNumber: true,
              })}
            />
            <Input
              label={"Номер этапа задачи " + (i + 1) + " (0 — без этапа)"}
              type="number"
              min={0}
              max={milestones.fields.length}
              value={flow.form.watch(("tasks." + i + ".milestone") as "tasks.0.milestone") + 1}
              onChange={(e) =>
                flow.form.setValue(
                  ("tasks." + i + ".milestone") as "tasks.0.milestone",
                  Number(e.target.value) - 1,
                  { shouldDirty: true },
                )
              }
            />
            <label className="check-row">
              <input
                type="checkbox"
                {...register(("tasks." + i + ".required") as "tasks.0.required")}
              />
              Обязательная задача
            </label>
          </div>
        ))}
        <Button type="submit">Показать план и даты</Button>
      </Form>
      {result && (
        <div className="goal-plan-part">
          <h3>Предпросмотр плана</h3>
          <p>
            {result.preview.input.start_date} — {result.preview.input.target_date}
          </p>
          <ul>
            {result.preview.tasks.map((t, i) => (
              <li key={i}>
                {t.planned_date} · {t.title}
              </li>
            ))}
          </ul>
          <p>{result.preview.notice}</p>
          {flow.form.formState.isDirty && <p role="status">План изменён. Обновите предпросмотр.</p>}
          <label className="check-row">
            <input
              type="checkbox"
              checked={checked}
              disabled={done || applying}
              onChange={(e) => setChecked(e.target.checked)}
            />
            Я проверил даты, задачи и нагрузку
          </label>
          <Button
            disabled={!checked || applying || done || flow.form.formState.isDirty}
            onClick={() => void apply()}
          >
            {done ? "План создан" : applying ? "Создаём план…" : "Создать цель и задачи"}
          </Button>
          {applyError && <p role="alert">{applyError}</p>}
        </div>
      )}
    </div>
  );
}
