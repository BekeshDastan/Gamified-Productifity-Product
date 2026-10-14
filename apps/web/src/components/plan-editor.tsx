"use client";
import { useRef, useState } from "react";
import { z } from "zod";
import {
  planInput,
  savePlan,
  addPlanBlock,
  blockInput,
  movePlanQuest,
  type DayPlan,
  type PlanInput,
} from "@/lib/planning";
import { getQuest } from "@/lib/quests";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { QuestPicker } from "./quest-structure";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { Card } from "./ui/card";
import { useTabDraft, TabDraftRecovery } from "@/lib/forms/tab-draft";
import type { RoutineTemplate } from "@/lib/journal";
import { RoutineChecklist } from "./routine-templates";
import { ChoiceChips, EmptyScene } from "./ui/experience";
import { Clock3, ListChecks, Sparkles, Sun, Coffee } from "lucide-react";
const editorSchema = planInput.extend({
  reason: z.string().max(1000),
  checked: z.array(z.string()).max(20),
});
export function PlanEditor({
  plan,
  onChanged,
  saveOverride,
  title,
  submitLabel,
  draftStorageKey,
  checklist,
  initialChecked = [],
}: {
  plan: DayPlan;
  onChanged: () => void | Promise<unknown>;
  saveOverride?: (
    input: PlanInput,
    reason: string,
    key: string,
    checked: string[],
  ) => Promise<void>;
  title?: string;
  submitLabel?: string;
  draftStorageKey?: string;
  checklist?: RoutineTemplate;
  initialChecked?: string[];
}) {
  const operation = useRef({ body: "", key: "" });
  const [selected, setSelected] = useState("");
  const [titles, setTitles] = useState<Record<string, string>>({});
  const { form, onSubmit, saving, error } = useValidatedForm({
    schema: editorSchema,
    defaultValues: { ...planInput.parse(plan), reason: "", checked: initialChecked },
    fields: ["available_minutes", "windows", "quest_ids", "top_ids", "rest", "reason"],
    save: async (values) => {
      const body = JSON.stringify(values);
      if (operation.current.body !== body) operation.current = { body, key: crypto.randomUUID() };
      if (saveOverride)
        await saveOverride(
          planInput.parse(values),
          values.reason,
          operation.current.key,
          values.checked,
        );
      else await savePlan(plan, planInput.parse(values), values.reason, operation.current.key);
      draft.clear();
      await onChanged();
    },
  });
  const draft = useTabDraft({
    storageKey: draftStorageKey,
    schema: editorSchema,
    value: form.watch(),
    dirty: form.formState.isDirty,
    restore: (values) => form.reset(values, { keepDefaultValues: true }),
  });
  const ids = form.watch("quest_ids"),
    top = form.watch("top_ids"),
    windows = form.watch("windows");
  const set = (name: "quest_ids" | "top_ids", value: string[]) =>
    form.setValue(name, value, { shouldDirty: true, shouldValidate: true });
  async function add() {
    if (!selected || ids.includes(selected)) return;
    try {
      const q = await getQuest(selected);
      setTitles((v) => ({ ...v, [q.id]: q.title }));
      set("quest_ids", [...ids, q.id]);
      setSelected("");
    } catch {
      form.setError("root.server", {
        message: "Не удалось загрузить задачу. Повторите добавление.",
      });
    }
  }
  return (
    <Card className="plan-composer">
      <div className="panel-heading">
        <span className="panel-icon">
          <Sun aria-hidden="true" />
        </span>
        <div>
          <p className="eyebrow">СОБЕРИТЕ СВОЙ ДЕНЬ</p>
          <h2>{title ?? (plan.confirmed_at ? "Изменить текущий план" : "Черновик плана")}</h2>
        </div>
      </div>
      <Form saving={saving} error={error} onSubmit={onSubmit}>
        {draftStorageKey && <TabDraftRecovery draft={draft} />}
        {checklist && (
          <RoutineChecklist
            template={checklist}
            checked={form.watch("checked")}
            onChange={(value) => form.setValue("checked", value, { shouldDirty: true })}
          />
        )}
        <section className="time-budget">
          <div className="step-heading">
            <span>01</span>
            <h3>Сколько времени у вас есть?</h3>
            <Clock3 size={20} aria-hidden="true" />
          </div>
          <ChoiceChips
            label="Быстрый выбор доступного времени"
            value={form.watch("available_minutes")}
            options={[30, 60, 120, 240].map((value) => ({
              value,
              label: value < 60 ? `${value} мин` : `${value / 60} ч`,
            }))}
            onChange={(value) =>
              form.setValue("available_minutes", value, { shouldDirty: true, shouldValidate: true })
            }
            disabled={form.watch("rest")}
          />
          <div className="budget-controls">
            <input
              type="range"
              aria-label="Запас времени на день"
              min={0}
              max={1440}
              step={15}
              value={form.watch("available_minutes") || 0}
              disabled={form.watch("rest")}
              onChange={(e) =>
                form.setValue("available_minutes", Number(e.target.value), {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            />
            <Input
              label="Доступно минут"
              type="number"
              readOnly={form.watch("rest")}
              min={0}
              max={1440}
              {...form.register("available_minutes", { valueAsNumber: true })}
              error={form.formState.errors.available_minutes?.message}
            />
          </div>
          <label className="check-row rest-toggle">
            <input
              type="checkbox"
              {...form.register("rest", {
                onChange: (e) => {
                  if (e.target.checked) {
                    form.setValue("available_minutes", 0, { shouldDirty: true });
                    form.setValue("windows", [], { shouldDirty: true });
                    set("quest_ids", []);
                    set("top_ids", []);
                  }
                },
              })}
            />
            <Coffee size={19} aria-hidden="true" /> День отдыха без оценки
          </label>
        </section>
        <fieldset disabled={form.watch("rest")} className="ui-form-fields plan-composer-body">
          <legend className="sr-only">Задачи и рабочие окна</legend>
          <section className="plan-windows-panel">
            <div className="step-heading">
              <span>02</span>
              <h3>Рабочие окна</h3>
              <Sun size={20} aria-hidden="true" />
            </div>
            <div className="day-ribbon" aria-label="Рабочие окна на шкале дня">
              {[0, 6, 12, 18, 24].map((hour) => (
                <span key={hour} style={{ left: `${(hour / 24) * 100}%` }}>
                  {hour}:00
                </span>
              ))}
              {windows.map((w, i) => {
                const minute = (v: string) => {
                  const [h = NaN, m = NaN] = v.split(":").map(Number);
                  return h * 60 + m;
                };
                const a = minute(w.start),
                  b = minute(w.end);
                return Number.isFinite(a) && Number.isFinite(b) && b > a && a >= 0 && b <= 1440 ? (
                  <i
                    key={i}
                    title={`${w.start} — ${w.end}`}
                    style={{ left: `${(a / 1440) * 100}%`, width: `${((b - a) / 1440) * 100}%` }}
                  />
                ) : null;
              })}
            </div>
            {windows.map((w, i) => (
              <div className="plan-window" key={i}>
                <Input
                  label={`Начало окна ${i + 1}`}
                  type="time"
                  {...form.register(`windows.${i}.start`)}
                />
                <Input
                  label={`Конец окна ${i + 1}`}
                  hint="Можно 24:00 для конца дня"
                  {...form.register(`windows.${i}.end`)}
                />
                <Button
                  variant="ghost"
                  onClick={() =>
                    form.setValue(
                      "windows",
                      windows.filter((_, n) => n !== i),
                      { shouldDirty: true },
                    )
                  }
                >
                  Убрать окно {i + 1}
                </Button>
              </div>
            ))}
            <Button
              variant="secondary"
              disabled={windows.length >= 8}
              onClick={() =>
                form.setValue("windows", [...windows, { start: "09:00", end: "18:00" }], {
                  shouldDirty: true,
                })
              }
            >
              Добавить рабочее окно
            </Button>
          </section>
          <section className="plan-queue-panel">
            <div className="step-heading">
              <span>03</span>
              <h3>Что важно сделать?</h3>
              <ListChecks size={20} aria-hidden="true" />
            </div>
            <QuestPicker label="Задача в план" value={selected} onChange={setSelected} />
            <Button
              variant="secondary"
              onClick={() => void add()}
              disabled={!selected || ids.includes(selected) || ids.length >= 100}
            >
              <Sparkles size={16} aria-hidden="true" /> Добавить выбранную задачу
            </Button>
          </section>
          <div className="plan-selected-heading">
            <h3>Ваш план</h3>
            <span className="count-badge">Главных задач: {top.length} из 3</span>
          </div>
          {form.formState.errors.top_ids?.message && (
            <p role="alert">{form.formState.errors.top_ids.message}</p>
          )}
          <ul className="plan-items">
            {ids.map((id) => (
              <li key={id}>
                <SafeLink href={`/app/quests/${id}`}>
                  {plan.items.find((q) => q.id === id)?.title ?? titles[id] ?? "Выбранная задача"}
                </SafeLink>
                <label className="check-row">
                  <input
                    type="checkbox"
                    checked={top.includes(id)}
                    onChange={(e) => {
                      if (e.target.checked && top.length === 3) {
                        form.setError("top_ids", {
                          message: "Сначала снимите одну из трёх главных задач",
                        });
                        return;
                      }
                      form.clearErrors("top_ids");
                      set("top_ids", e.target.checked ? [...top, id] : top.filter((v) => v !== id));
                    }}
                  />
                  Главная задача
                </label>
                <Button
                  variant="ghost"
                  onClick={() => {
                    set(
                      "quest_ids",
                      ids.filter((v) => v !== id),
                    );
                    set(
                      "top_ids",
                      top.filter((v) => v !== id),
                    );
                  }}
                >
                  Убрать из плана
                </Button>
              </li>
            ))}
          </ul>
          {!ids.length && (
            <EmptyScene
              title="Оставьте место для главного"
              description="Выберите задачи выше и отметьте до трёх главных."
              tone="mint"
            />
          )}
        </fieldset>
        <details className="quiet-details">
          <summary>Как сохраняется план</summary>
          <p className="ui-hint">
            Выбор задач не переносит их плановую дату. Перенос и блоки времени доступны после
            сохранения. Подтверждение фиксирует исходный план.
          </p>
        </details>
        {plan.confirmed_at && (
          <Input label="Причина изменения плана" {...form.register("reason")} />
        )}
        <Button type="submit">
          {submitLabel ?? (plan.confirmed_at ? "Сохранить дополнение" : "Сохранить черновик")}
        </Button>
      </Form>
    </Card>
  );
}
export function PlanBlocks({ plan, onChanged }: { plan: DayPlan; onChanged: () => void }) {
  const key = useRef(crypto.randomUUID());
  const schema = blockInput.extend({ action: z.enum(["block", "move"]) });
  const { form, onSubmit, saving, error } = useValidatedForm({
    schema,
    defaultValues: {
      quest_id:
        plan.items.find(
          (q) => !["completed", "failed", "skipped"].includes(q.status) && !q.archived,
        )?.id ?? "",
      quest_version: 0,
      start: "09:00",
      end: "10:00",
      reason: "",
      action: "block",
    },
    fields: ["quest_id", "reason", "start", "end"],
    save: async (v) => {
      const q = plan.items.find((q) => q.id === v.quest_id);
      if (!q) throw Error("missing task");
      const input = { ...blockInput.parse(v), quest_version: q.version };
      if (v.action === "move") await movePlanQuest(plan, input, key.current);
      else await addPlanBlock(plan, input, key.current);
      key.current = crypto.randomUUID();
      onChanged();
    },
  });
  return (
    <Card>
      <h2>Блоки работы и перенос</h2>
      <p className="ui-hint">
        Пояс плана: {plan.timezone}. Можно добавить несколько блоков одной задачи. Плановая дата
        изменится на {plan.date}; дедлайн сохранится. Существующие блоки редактируются в календаре.
      </p>
      <Form saving={saving} error={error} onSubmit={onSubmit}>
        <Select label="Задача для блока" {...form.register("quest_id")}>
          <option value="">Выберите задачу</option>
          {plan.items
            .filter((q) => !["completed", "failed", "skipped"].includes(q.status) && !q.archived)
            .map((q) => (
              <option key={q.id} value={q.id}>
                {q.title}
              </option>
            ))}
        </Select>
        <Select label="Действие с задачей" {...form.register("action")}>
          <option value="block">Добавить блок и назначить дату</option>
          <option value="move">Только перенести плановую дату</option>
        </Select>
        {form.watch("action") === "block" && (
          <div className="plan-window">
            <Input label="Начало блока" type="time" {...form.register("start")} />
            <Input label="Конец блока" type="time" {...form.register("end")} />
          </div>
        )}
        <Input label="Причина переноса или блока" {...form.register("reason")} />
        <Button type="submit" disabled={!plan.id}>
          Применить к задаче
        </Button>
      </Form>
      <SafeLink href="/app/calendar">Открыть календарь</SafeLink>
    </Card>
  );
}
