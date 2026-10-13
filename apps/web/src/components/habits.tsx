"use client";
import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useWatch } from "react-hook-form";
import { DateTime } from "luxon";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { questLabels } from "@/lib/quests";
import {
  habitInput,
  getHabitStarter,
  useHabits,
  createHabit,
  logHabit,
  pauseHabit,
  emergencyHabit,
  resumeHabit,
  type Habit,
  type HabitPeriod,
} from "@/lib/habits";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { HabitHistory } from "./habit-history";
import { useRewardNotice } from "./progression";
const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
const stateLabels = {
  open: "Период открыт",
  success: "Норма выполнена",
  missed: "Пропуск",
  rest: "Отдых",
};
export function useHabitChanged() {
  const auth = useAuth(),
    cache = useQueryClient();
  return () => void cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
}
function useRequestKey() {
  const memo = useRef({ body: "", key: "" });
  return (value: unknown) => {
    const body = JSON.stringify(value);
    if (body !== memo.current.body || !memo.current.key)
      memo.current = { body, key: crypto.randomUUID() };
    return memo.current.key;
  };
}
export function Habits() {
  const [archived, setArchived] = useState(false),
    discard = useConfirmDiscard();
  const auth = useAuth();
  const [starter, setStarter] = useState<{ id: string; title: string }>();
  const suggestion = useQuery({
    queryKey: ["account", auth.session?.account.id, "habit-starter"],
    queryFn: ({ signal }) => getHabitStarter(signal),
    enabled: !!auth.session?.account.id,
  });
  const data = useHabits(archived),
    settings = useSettings(),
    changed = useHabitChanged();
  if (data.isPending || !settings.data)
    return (
      <Card>
        <p role={data.isError || settings.isError ? "alert" : "status"}>
          {data.isError || settings.isError
            ? "Не удалось загрузить привычки и настройки."
            : "Загружаем привычки и ваш часовой пояс…"}
        </p>
        <Button
          onClick={() => {
            void data.refetch();
            void settings.refetch();
          }}
        >
          Повторить
        </Button>
      </Card>
    );
  return (
    <>
      <Button
        variant="secondary"
        onClick={() => {
          if (discard()) setArchived(!archived);
        }}
      >
        {archived ? "Показать активные" : "Показать архив"}
      </Button>
      {!archived && (
        <Card>
          <h2>Новая привычка</h2>
          {suggestion.data && !starter && (
            <>
              <p>Сохранённая идея: {suggestion.data.title}</p>
              <Button
                variant="secondary"
                onClick={() => {
                  if (discard()) setStarter(suggestion.data ?? undefined);
                }}
              >
                Настроить стартовую привычку
              </Button>
            </>
          )}
          <HabitCreate
            key={starter?.id ?? "new"}
            starter={starter}
            changed={() => {
              setStarter(undefined);
              changed();
            }}
          />
        </Card>
      )}
      {data.isPending && <p role="status">Загружаем привычки…</p>}
      {data.isError && (
        <p role="alert">
          Не удалось загрузить привычки.{" "}
          <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      {data.data?.length === 0 && (
        <Card>
          <h2>{archived ? "Архив пуст" : "Начните с малого"}</h2>
          <p>Добавьте одну привычку и удобную норму. Незапланированные дни не разрывают серию.</p>
        </Card>
      )}
      <div className="habit-grid">
        {data.data?.map((h) => (
          <HabitCard key={h.id} habit={h} changed={changed} />
        ))}
      </div>
    </>
  );
}
function HabitCreate({
  changed,
  starter,
}: {
  changed: () => void;
  starter?: { id: string; title: string };
}) {
  const settings = useSettings(),
    auth = useAuth(),
    key = useRequestKey();
  const labels = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-labels"],
    queryFn: ({ signal }) => questLabels(signal),
  });
  const flow = useValidatedForm({
    schema: habitInput,
    defaultValues: {
      title: starter?.title ?? "",
      starter_id: starter?.id ?? null,
      kind: "count",
      unit: "страниц",
      minimum: 1,
      area_id: null,
      starts_on: DateTime.now()
        .setZone(settings.data?.timezone ?? "UTC")
        .toISODate()!,
      ends_on: null,
      schedule: { frequency: "daily", weekdays: [1, 2, 3, 4, 5, 6, 7], target: 20 },
    },
    fields: [
      "title",
      "kind",
      "unit",
      "minimum",
      "area_id",
      "starts_on",
      "ends_on",
      "schedule.frequency",
      "schedule.weekdays",
      "schedule.target",
    ],
    save: async (v) => {
      await createHabit(v, key(v));
      changed();
      return { ...v, title: "" };
    },
  });
  const f = flow.form,
    e = f.formState.errors,
    frequency = useWatch({ control: f.control, name: "schedule.frequency" }),
    days = useWatch({ control: f.control, name: "schedule.weekdays" });
  return (
    <Form onSubmit={flow.onSubmit} saving={flow.saving} error={flow.error}>
      <Input label="Название привычки" {...f.register("title")} error={e.title?.message} />
      <div className="form-grid">
        <Select
          label="Тип привычки"
          {...f.register("kind")}
          onChange={(event) => {
            const kind = event.target.value as "binary" | "count" | "duration";
            f.setValue("kind", kind, { shouldDirty: true });
            if (kind === "binary") {
              f.setValue("minimum", 1);
              f.setValue("schedule.target", 1);
              f.setValue("unit", "раз");
            } else if (kind === "duration") f.setValue("unit", "мин");
          }}
        >
          <option value="binary">Выполнено / нет</option>
          <option value="count">Количество</option>
          <option value="duration">Длительность</option>
        </Select>
        <Input label="Единица" {...f.register("unit")} error={e.unit?.message} />
        <Input
          label="Минимальный шаг"
          type="number"
          min="0.001"
          step="0.001"
          {...f.register("minimum", { valueAsNumber: true })}
          error={e.minimum?.message}
        />
        <Input
          label="Норма периода"
          type="number"
          min="0.001"
          step="0.001"
          {...f.register("schedule.target", { valueAsNumber: true })}
          error={e.schedule?.target?.message}
        />
        <Select
          label="Период нормы"
          {...f.register("schedule.frequency")}
          onChange={(event) => {
            const v = event.target.value as "daily" | "weekly" | "monthly";
            f.setValue("schedule.frequency", v, { shouldDirty: true });
            f.setValue("schedule.weekdays", v === "daily" ? [1, 2, 3, 4, 5, 6, 7] : []);
          }}
        >
          <option value="daily">Выбранные дни</option>
          <option value="weekly">За неделю</option>
          <option value="monthly">За месяц</option>
        </Select>
        <Select
          label="Направление"
          value={useWatch({ control: f.control, name: "area_id" }) ?? ""}
          onChange={(event) =>
            f.setValue("area_id", event.target.value || null, { shouldDirty: true })
          }
        >
          <option value="">Без направления</option>
          {labels.data?.areas
            .filter((a) => !a.archived)
            .map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
        </Select>
        <Input
          label="Начало"
          type="date"
          {...f.register("starts_on")}
          error={e.starts_on?.message}
        />
        <Input
          label="Конец, необязательно"
          type="date"
          {...f.register("ends_on", { setValueAs: (v) => v || null })}
          error={e.ends_on?.message}
        />
      </div>
      {frequency === "daily" && (
        <fieldset>
          <legend>Запланированные дни</legend>
          <div className="habit-weekdays">
            {weekdays.map((name, i) => (
              <label key={name}>
                <input
                  type="checkbox"
                  checked={days.includes(i + 1)}
                  onChange={(event) =>
                    f.setValue(
                      "schedule.weekdays",
                      event.target.checked ? [...days, i + 1] : days.filter((d) => d !== i + 1),
                      { shouldDirty: true },
                    )
                  }
                />{" "}
                {name}
              </label>
            ))}
          </div>
          {e.schedule?.weekdays?.message && <p role="alert">{e.schedule.weekdays.message}</p>}
        </fieldset>
      )}
      <p className="ui-hint">
        Для «3 раза за неделю» выберите бинарный тип и недельную норму 3. Пояс фиксируется при
        создании.
      </p>
      <Button type="submit">Добавить привычку</Button>
    </Form>
  );
}
export function HabitCard({
  habit: h,
  changed,
  compact = false,
}: {
  habit: Habit;
  changed: () => void;
  compact?: boolean;
}) {
  const current = h.periods.find(
    (p) => Date.parse(p.start) <= Date.parse(h.as_of) && Date.parse(h.as_of) < Date.parse(p.end),
  );
  return (
    <Card className="habit-card" id={"habit-" + h.id}>
      <h2>{h.title}</h2>
      {h.has_more ? (
        <p role="status">Восстанавливаем периоды истории…</p>
      ) : (
        <>
          <p>
            Серия: <strong>{h.stats.current}</strong> · Лучшая: <strong>{h.stats.best}</strong>
          </p>
          <p className="ui-hint">
            Успешных периодов: {h.stats.successful}; пропусков: {h.stats.missed}. Доля выполнения:{" "}
            {h.stats.rate}%. {h.stats.recovering && "Вы снова возвращаетесь к привычке."}
          </p>
          {current ? (
            <>
              <p>
                {stateLabels[current.state]} · {current.amount} / {current.target} {h.unit} ·{" "}
                {current.progress}%
              </p>
              <progress
                aria-label={`Прогресс привычки ${h.title}`}
                max={100}
                value={current.progress}
              />
              <p className="ui-hint">
                До {DateTime.fromISO(current.end).setZone(h.timezone).toFormat("dd.MM.yyyy HH:mm")}{" "}
                ({h.timezone}). {current.emergency_reason || current.rest_reason}
              </p>
              {!current.excluded && !h.archived_at && (
                <HabitLogForm
                  key={h.version + ":" + current.id}
                  habit={h}
                  period={current}
                  changed={changed}
                />
              )}
            </>
          ) : (
            <p className="ui-hint">Сейчас нет запланированного периода.</p>
          )}
          {!compact && (
            <>
              <HabitHistory habit={h} changed={changed} />
              <details>
                <summary>Календарь отметок</summary>
                <p className="ui-hint">
                  До 120 последних периодов. Полная, частичная норма, пропуск и отдых различаются
                  словами и процентом.
                </p>
                <ul className="habit-heatmap">
                  {h.periods.map((p) => (
                    <li key={p.id} data-state={p.state}>
                      <time>{DateTime.fromISO(p.start).setZone(h.timezone).toFormat("dd.MM")}</time>
                      <span>{stateLabels[p.state]}</span>
                      <strong>{p.progress}%</strong>
                    </li>
                  ))}
                </ul>
              </details>
              <details hidden={!!h.archived_at}>
                <summary>Пауза и экстренный пропуск</summary>
                <HabitPauseForm key={"pause:" + h.version} habit={h} changed={changed} />
                {current &&
                  !current.excluded &&
                  current.state === "open" &&
                  !current.emergency_reason && (
                    <HabitEmergencyForm
                      key={"emergency:" + h.version}
                      habit={h}
                      period={current}
                      changed={changed}
                    />
                  )}
                <ul>
                  {h.pauses.map((p) => (
                    <li key={p.id}>
                      {p.starts_on} — {p.ends_on ?? "до возобновления"}: {p.reason}
                      <HabitResumeForm
                        key={h.version + ":" + p.id}
                        habit={h}
                        pause={p.id}
                        changed={changed}
                      />
                    </li>
                  ))}
                </ul>
              </details>
            </>
          )}
        </>
      )}
    </Card>
  );
}
const logForm = z.object({
  amount: z.number().positive("Укажите положительное количество").max(1000000),
  occurred_at: z.string().min(1, "Укажите время факта"),
});
function HabitLogForm({
  habit: h,
  period: p,
  changed,
}: {
  habit: Habit;
  period: HabitPeriod;
  changed: () => void;
}) {
  const key = useRequestKey(),
    rewardNotice = useRewardNotice();
  const flow = useValidatedForm({
    schema: logForm,
    defaultValues: {
      amount: h.kind === "binary" ? 1 : h.minimum,
      occurred_at: DateTime.now().setZone(h.timezone).toFormat("yyyy-MM-dd'T'HH:mm"),
    },
    fields: ["amount", "occurred_at"],
    save: async (v) => {
      const input = {
        period_id: p.id,
        amount: v.amount,
        occurred_at: DateTime.fromISO(v.occurred_at, { zone: h.timezone }).toUTC().toISO()!,
      };
      const saved = await logHabit(h, input, key(input));
      rewardNotice(saved.reward);
      changed();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <div className="form-grid">
        <Input
          label={`Количество (${h.unit})`}
          type="number"
          min="0.001"
          step="0.001"
          {...flow.form.register("amount", { valueAsNumber: true })}
          error={flow.form.formState.errors.amount?.message}
        />
        <Input
          label={`Время факта (${h.timezone})`}
          type="datetime-local"
          {...flow.form.register("occurred_at")}
          error={flow.form.formState.errors.occurred_at?.message}
        />
      </div>
      <Button type="submit">Добавить отметку</Button>
    </Form>
  );
}
const pauseForm = z.object({
  starts_on: z.iso.date(),
  ends_on: z.string(),
  reason: z.string().trim().min(1, "Укажите причину").max(1000),
});
function HabitPauseForm({ habit: h, changed }: { habit: Habit; changed: () => void }) {
  const key = useRequestKey();
  const flow = useValidatedForm({
    schema: pauseForm,
    defaultValues: { starts_on: "", ends_on: "", reason: "" },
    fields: ["starts_on", "ends_on", "reason"],
    save: async (v) => {
      const input = { ...v, ends_on: v.ends_on || null };
      await pauseHabit(h, input, key(input));
      changed();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <p>Плановая пауза начинается в будущем на границе периода. Прошлые пропуски сохраняются.</p>
      <Input label="Начало паузы" type="date" {...flow.form.register("starts_on")} />
      <Input
        label="Последний день паузы, необязательно"
        type="date"
        {...flow.form.register("ends_on")}
      />
      <Input
        label="Причина паузы"
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      <Button type="submit" variant="secondary">
        Запланировать паузу
      </Button>
    </Form>
  );
}
function HabitEmergencyForm({
  habit: h,
  period: p,
  changed,
}: {
  habit: Habit;
  period: HabitPeriod;
  changed: () => void;
}) {
  const key = useRequestKey();
  const schema = z.object({ reason: z.string().trim().min(1, "Укажите причину").max(1000) });
  const flow = useValidatedForm({
    schema,
    defaultValues: { reason: "" },
    fields: ["reason"],
    save: async (v) => {
      await emergencyHabit(h, { period_id: p.id, reason: v.reason }, key(v));
      changed();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Причина экстренного пропуска"
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      <p className="ui-hint">
        Причина сохраняется для текущего периода. Норма остаётся в расчёте; это не успешное
        выполнение.
      </p>
      <Button type="submit" variant="secondary">
        Отметить экстренный пропуск
      </Button>
    </Form>
  );
}
function HabitResumeForm({
  habit: h,
  pause,
  changed,
}: {
  habit: Habit;
  pause: string;
  changed: () => void;
}) {
  const key = useRequestKey();
  const schema = z.object({
    resumes_on: z.iso.date(),
    reason: z.string().trim().min(1, "Укажите причину").max(1000),
  });
  const flow = useValidatedForm({
    schema,
    defaultValues: { resumes_on: "", reason: "" },
    fields: ["resumes_on", "reason"],
    save: async (v) => {
      await resumeHabit(h, pause, v, key(v));
      changed();
    },
  });
  return (
    <details>
      <summary>Возобновить раньше</summary>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input label="Дата возобновления" type="date" {...flow.form.register("resumes_on")} />
        <Input
          label="Причина возобновления"
          {...flow.form.register("reason")}
          error={flow.form.formState.errors.reason?.message}
        />
        <Button type="submit" variant="secondary">
          Сохранить возобновление
        </Button>
      </Form>
    </details>
  );
}
export function DashboardHabits() {
  const data = useHabits(),
    changed = useHabitChanged();
  return (
    <>
      <h2>Привычки</h2>
      {data.isPending && <p role="status">Загружаем привычки…</p>}
      {data.isError && (
        <p role="alert">
          Не удалось загрузить привычки.{" "}
          <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      {data.data?.length === 0 && <p className="ui-hint">Добавьте одну посильную привычку.</p>}
      {data.data?.slice(0, 3).map((h) => (
        <HabitCard key={h.id} habit={h} changed={changed} compact />
      ))}
      <SafeLink href="/app/habits">Все привычки</SafeLink>
    </>
  );
}
