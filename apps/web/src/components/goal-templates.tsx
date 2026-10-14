"use client";
import { useRef, useState } from "react";
import { useFieldArray } from "react-hook-form";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
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
import { Compass, ArrowUpRight } from "lucide-react";
export function GoalTemplates({
  onSaved,
  guided = false,
}: {
  onSaved: (goal: Goal) => void;
  guided?: boolean;
}) {
  const account = useAuth().session?.account.id;
  const templates = useQuery({
    queryKey: ["account", account, "goal-templates"],
    queryFn: ({ signal }) => goalTemplates(signal),
  });
  const [selected, setSelected] = useState<GoalTemplate>();
  const discard = useConfirmDiscard();
  return (
    <Card className="goal-template-gallery">
      <h2>{guided ? "Навигатор новой цели" : "Начать с шаблона"}</h2>
      {guided && (
        <p>
          Выберите направление результата, отредактируйте этапы и первый шаг. Все записи появятся
          только после просмотра полного плана и подтверждения.
        </p>
      )}
      {templates.isPending ? (
        <p role="status">Загружаем шаблоны…</p>
      ) : templates.data ? (
        <>
          <div className="goal-template-choices">
            {templates.data.map((t) => (
              <Button
                key={t.id}
                variant="secondary"
                aria-label={t.name}
                aria-pressed={selected?.id === t.id}
                onClick={() => {
                  if (discard()) setSelected(t);
                }}
              >
                <Compass size={24} aria-hidden="true" />
                <strong>{t.name}</strong>
                <small>{t.description}</small>
                <ArrowUpRight size={18} aria-hidden="true" />
              </Button>
            ))}
          </div>
          {selected && (
            <TemplateEditor
              key={selected.id}
              template={selected}
              onSaved={onSaved}
              guided={guided}
            />
          )}
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
  guided,
}: {
  template: GoalTemplate;
  onSaved: (goal: Goal) => void;
  guided: boolean;
}) {
  const [step, setStep] = useState(1);
  const [checked, setChecked] = useState(false);
  const [result, setResult] = useState<{ body: TemplateDraft; preview: TemplatePreview }>();
  const pending = useRef<{ body: string; key: string } | undefined>(undefined);
  const [applyError, setApplyError] = useState("");
  const [applying, setApplying] = useState(false);
  const [done, setDone] = useState(false);
  useUnsavedChanges(!!result && !done);
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
      setStep(4);
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
      <Form saving={flow.saving || applying} error={flow.error} onSubmit={flow.onSubmit}>
        <fieldset hidden={guided && step !== 1} disabled={applying || done}>
          {guided && <legend>1. Проверяемый результат</legend>}
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
        </fieldset>
        <fieldset hidden={guided && step !== 2} disabled={applying || done}>
          <legend>{guided ? "2. Этапы плана" : "Этапы плана"}</legend>
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
        </fieldset>
        <fieldset hidden={guided && step !== 3} disabled={applying || done}>
          <legend>{guided ? "3. Первый шаг и будущие задачи" : "Будущие задачи"}</legend>
          {guided && (
            <p>
              Первая задача — начало плана. Каждый шаг можно переименовать и изменить его дату,
              оценку и обязательность.
            </p>
          )}
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
                label={"Оценка задачи " + (i + 1) + ", минут"}
                type="number"
                min={0}
                max={525600}
                {...register(("tasks." + i + ".estimated_minutes") as "tasks.0.estimated_minutes", {
                  valueAsNumber: true,
                })}
              />
              <Input
                label={"Сложность задачи " + (i + 1)}
                type="number"
                min={1}
                max={5}
                {...register(("tasks." + i + ".difficulty") as "tasks.0.difficulty", {
                  valueAsNumber: true,
                })}
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
        </fieldset>
        {guided && (
          <div className="button-row">
            <Button
              variant="secondary"
              disabled={step <= 1 || applying || done}
              onClick={() => setStep(step - 1)}
            >
              Назад по шагам цели
            </Button>
            {step < 3 && (
              <Button variant="secondary" onClick={() => setStep(step + 1)}>
                Далее по шагам цели
              </Button>
            )}
          </div>
        )}
        {(!guided || step === 3) && <Button type="submit">Показать план и даты</Button>}
      </Form>
      {result && (!guided || step === 4) && (
        <div className="goal-plan-part">
          <h3>Предпросмотр плана</h3>
          <p>
            {result.preview.input.title} · {result.preview.input.criteria}
          </p>
          {result.preview.input.progress_type === "metric" && (
            <p>
              Результат: {result.preview.input.start} — {result.preview.input.target}{" "}
              {result.preview.input.unit}
            </p>
          )}
          <p>
            {result.preview.input.start_date} — {result.preview.input.target_date}
          </p>
          <ul>
            {result.preview.milestones.map((m, i) => (
              <li key={"m" + i}>
                Этап {i + 1}: {m.title} · вес {m.weight} ·{" "}
                {m.required ? "обязательный" : "необязательный"}
              </li>
            ))}
            {result.preview.tasks.map((t, i) => (
              <li key={i}>
                {t.planned_date} · {t.title} · {t.description} · {t.estimated_minutes} минут ·
                сложность {t.difficulty} · {t.required ? "обязательная" : "необязательная"} ·{" "}
                {t.milestone < 0 ? "без этапа" : `этап ${t.milestone + 1}`}
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
