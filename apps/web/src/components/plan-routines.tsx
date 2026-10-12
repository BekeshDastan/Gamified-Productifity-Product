"use client";
import { useRef, useState } from "react";
import { DateTime } from "luxon";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import {
  listRoutines,
  previewRoutine,
  applyRoutine,
  saveRoutine,
  routineInput,
  type Routine,
  type RoutinePreview,
} from "@/lib/routines";
import { ApiError } from "@/lib/api/client";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { Card } from "./ui/card";
const weekdays = [
  "Понедельник",
  "Вторник",
  "Среда",
  "Четверг",
  "Пятница",
  "Суббота",
  "Воскресенье",
];
export function PlanRoutines({ date, onChanged }: { date: string; onChanged: () => void }) {
  const auth = useAuth(),
    discard = useConfirmDiscard();
  const [selected, setSelected] = useState("study-day"),
    [preview, setPreview] = useState<RoutinePreview>(),
    [editing, setEditing] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const library = useQuery({
    queryKey: ["account", auth.session?.account.id, "routines"],
    queryFn: ({ signal }) => listRoutines(signal),
  });
  const current = library.data?.find((t) => t.id === selected);
  const start =
    current?.period === "week" ? DateTime.fromISO(date).startOf("week").toISODate()! : date;
  async function showPreview() {
    if (!current || busy) return;
    setBusy(true);
    setError("");
    try {
      setPreview(await previewRoutine(current.id, start));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Не удалось подготовить предпросмотр.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>Идеальный день и неделя</h2>
      <p className="ui-hint">
        Шаблон добавляет действия только после предпросмотра и подтверждения. Уже применённые дни не
        дублируются; прошлые даты пропускаются. Окна и лимит существующего плана сохраняются.
      </p>
      {library.isPending && <p role="status">Загружаем шаблоны…</p>}
      {library.isError && (
        <p role="alert">
          Не удалось загрузить шаблоны.
          <Button onClick={() => void library.refetch()}>Повторить</Button>
        </p>
      )}
      {current && (
        <>
          <Select
            label="Шаблон плана"
            value={selected}
            disabled={editing || busy}
            onChange={(e) => {
              if (discard()) {
                setSelected(e.target.value);
                setPreview(undefined);
                setError("");
              }
            }}
          >
            {library.data?.map((t) => (
              <option key={t.id} value={t.id}>
                {t.title}
              </option>
            ))}
          </Select>
          <p>
            Начало: {start}
            {current.period === "week" ? " · неделя с понедельника" : ""}
          </p>
          <div className="button-row">
            <Button
              variant="secondary"
              onClick={() => {
                if (discard()) {
                  setEditing(!editing);
                  setPreview(undefined);
                }
              }}
            >
              {editing ? "Закрыть настройки шаблона" : "Настроить шаблон"}
            </Button>
            {!editing && (
              <Button disabled={busy} onClick={() => void showPreview()}>
                {busy ? "Готовим предпросмотр…" : "Предпросмотр шаблона"}
              </Button>
            )}
          </div>
          {editing ? (
            <RoutineEditor
              key={current.id + ":" + current.version}
              current={current}
              onChanged={(routine) => {
                setSelected(routine.id);
                setEditing(false);
                void library.refetch();
                onChanged();
              }}
            />
          ) : (
            preview && (
              <RoutineApplication
                key={preview.hash}
                preview={preview}
                onRefresh={() => void showPreview()}
                onApplied={(value) => {
                  setPreview(value);
                  onChanged();
                }}
              />
            )
          )}
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </Card>
  );
}
function RoutineEditor({
  current,
  onChanged,
}: {
  current: Routine;
  onChanged: (value: Routine) => void;
}) {
  const key = useRef(crypto.randomUUID());
  const schema = routineInput.extend({ copy: z.boolean() });
  const { form, onSubmit, saving, error } = useValidatedForm({
    schema,
    defaultValues: { ...routineInput.parse(current), copy: false },
    fields: ["title", "period", "days"],
    save: async (v) => {
      const result = await saveRoutine(current, routineInput.parse(v), v.copy, key.current);
      key.current = crypto.randomUUID();
      onChanged(result);
    },
  });
  const days = form.watch("days");
  return (
    <Form saving={saving} error={error} onSubmit={onSubmit}>
      <Input label="Название шаблона" {...form.register("title")} />
      <p>{current.period === "week" ? "Недельный шаблон" : "Дневной шаблон"}</p>
      <label className="check-row">
        <input type="checkbox" {...form.register("copy")} />
        Сохранить как отдельный шаблон
      </label>
      {days.map((d, i) => (
        <fieldset key={d.day} className="ui-form-fields plan-template-day">
          <legend>{current.period === "week" ? weekdays[d.day - 1] : "Настройки дня"}</legend>
          <Input
            label={`Доступно минут — ${i + 1}`}
            type="number"
            min={0}
            max={1440}
            {...form.register(`days.${i}.available_minutes`, { valueAsNumber: true })}
          />
          <label className="check-row">
            <input
              type="checkbox"
              {...form.register(`days.${i}.rest`, {
                onChange: (e) => {
                  if (e.target.checked) {
                    form.setValue(`days.${i}.available_minutes`, 0, { shouldDirty: true });
                    form.setValue(`days.${i}.windows`, [], { shouldDirty: true });
                    form.setValue(`days.${i}.actions`, [], { shouldDirty: true });
                  }
                },
              })}
            />
            День отдыха
          </label>
          <fieldset disabled={d.rest} className="ui-form-fields">
            <legend>Окна и действия</legend>
            {d.windows.map((w, n) => (
              <div key={n} className="plan-window">
                <Input
                  label={`Начало окна шаблона ${i + 1}.${n + 1}`}
                  type="time"
                  {...form.register(`days.${i}.windows.${n}.start`)}
                />
                <Input
                  label={`Конец окна шаблона ${i + 1}.${n + 1}`}
                  {...form.register(`days.${i}.windows.${n}.end`)}
                />
                <Button
                  variant="ghost"
                  onClick={() =>
                    form.setValue(
                      `days.${i}.windows`,
                      d.windows.filter((_, j) => j !== n),
                      { shouldDirty: true },
                    )
                  }
                >
                  Убрать окно {i + 1}.{n + 1}
                </Button>
              </div>
            ))}
            <Button
              variant="secondary"
              disabled={d.windows.length >= 8}
              onClick={() =>
                form.setValue(
                  `days.${i}.windows`,
                  [...d.windows, { start: "09:00", end: "18:00" }],
                  { shouldDirty: true },
                )
              }
            >
              Добавить окно в день {i + 1}
            </Button>
            {d.actions.map((a, n) => (
              <div key={a.key} className="plan-template-action">
                <Input
                  label={`Действие ${i + 1}.${n + 1}`}
                  {...form.register(`days.${i}.actions.${n}.title`)}
                />
                <Input
                  label={`Оценка минут ${i + 1}.${n + 1}`}
                  type="number"
                  min={0}
                  max={1440}
                  {...form.register(`days.${i}.actions.${n}.minutes`, { valueAsNumber: true })}
                />
                <Select
                  label={`Приоритет действия ${i + 1}.${n + 1}`}
                  {...form.register(`days.${i}.actions.${n}.priority`)}
                >
                  <option value="low">Низкий</option>
                  <option value="normal">Обычный</option>
                  <option value="high">Высокий</option>
                </Select>
                <div className="plan-window">
                  <Input
                    label={`Начало действия ${i + 1}.${n + 1}`}
                    type="time"
                    {...form.register(`days.${i}.actions.${n}.start`)}
                  />
                  <Input
                    label={`Конец действия ${i + 1}.${n + 1}`}
                    type="time"
                    {...form.register(`days.${i}.actions.${n}.end`)}
                  />
                </div>
                <p className="ui-hint">Для задачи без блока оставьте обе границы пустыми.</p>
                <Button
                  variant="ghost"
                  onClick={() =>
                    form.setValue(
                      `days.${i}.actions`,
                      d.actions.filter((_, j) => j !== n),
                      { shouldDirty: true },
                    )
                  }
                >
                  Убрать действие {i + 1}.{n + 1}
                </Button>
              </div>
            ))}
            <Button
              variant="secondary"
              disabled={d.actions.length >= 20}
              onClick={() =>
                form.setValue(
                  `days.${i}.actions`,
                  [
                    ...d.actions,
                    {
                      key: crypto.randomUUID(),
                      title: "",
                      minutes: 30,
                      priority: "normal",
                      start: "",
                      end: "",
                    },
                  ],
                  { shouldDirty: true },
                )
              }
            >
              Добавить действие в день {i + 1}
            </Button>
          </fieldset>
        </fieldset>
      ))}
      <Button type="submit">Сохранить шаблон</Button>
    </Form>
  );
}
function RoutineApplication({
  preview,
  onRefresh,
  onApplied,
}: {
  preview: RoutinePreview;
  onRefresh: () => void;
  onApplied: (value: RoutinePreview) => void;
}) {
  const key = useRef(crypto.randomUUID());
  const schema = z.object({
    reason: z.string().trim().min(1, "Укажите причину").max(1000),
    accept_conflicts: z.boolean(),
  });
  const { form, onSubmit, saving, error } = useValidatedForm({
    schema,
    defaultValues: { reason: "", accept_conflicts: false },
    fields: ["reason", "accept_conflicts"],
    save: async (v) =>
      onApplied(await applyRoutine(preview, v.reason, v.accept_conflicts, key.current)),
  });
  return (
    <div className="plan-preview">
      <h3>Предпросмотр: {preview.template.title}</h3>
      <ul className="plan-items">
        {preview.days.map((d) => (
          <li key={d.date}>
            <strong>{d.date}</strong> ·{" "}
            {d.past
              ? "Прошедший день, пропускаем"
              : d.already_applied
                ? "Уже применено"
                : d.input.rest
                  ? "Отдых без оценки"
                  : "Будет добавлено"}
            {d.confirmed && " · исходный снимок сохраняется"}
            <p>
              {d.actions
                .map((a) => a.title + (a.start ? ` · ${a.start}–${a.end}` : ""))
                .join("; ") || "Без обязательных действий"}
            </p>
            <p>
              Пересечений: {d.availability.conflicts.length} · дефицит:{" "}
              {d.availability.deficit_minutes} мин
            </p>
          </li>
        ))}
      </ul>
      <Form saving={saving} error={error} onSubmit={onSubmit}>
        <Input label="Причина применения шаблона" {...form.register("reason")} />
        <label className="check-row">
          <input type="checkbox" {...form.register("accept_conflicts")} />Я принимаю показанные
          пересечения и дефицит времени
        </label>
        <Button type="submit" disabled={preview.days.every((d) => d.already_applied || d.past)}>
          Применить шаблон
        </Button>
        <Button variant="ghost" onClick={onRefresh}>
          Обновить предпросмотр
        </Button>
      </Form>
    </div>
  );
}
