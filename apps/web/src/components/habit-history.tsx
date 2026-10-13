"use client";
import { useRef, useState } from "react";
import { HabitVersion } from "./habit-version";
import { useRewardNotice } from "./progression";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useWatch } from "react-hook-form";
import { DateTime } from "luxon";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import {
  type Habit,
  type HabitPeriod,
  type HabitScheduleChange,
  type HabitSchedulePreview,
  habitScheduleChange,
  previewHabitSchedule,
  changeHabitSchedule,
  archiveHabit,
  getHabitLogPage,
  getHabitPeriodPage,
  correctHabitLog,
  logHabit,
} from "@/lib/habits";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
function useKey() {
  const ref = useRef({ body: "", key: "" });
  return (v: unknown) => {
    const body = JSON.stringify(v);
    if (body !== ref.current.body || !ref.current.key)
      ref.current = { body, key: crypto.randomUUID() };
    return ref.current.key;
  };
}
const why = z.object({ reason: z.string().trim().min(1, "Укажите причину").max(1000) });
export function HabitHistory({ habit: h, changed }: { habit: Habit; changed: () => void }) {
  return (
    <>
      <details>
        <summary>Будущее расписание и нормы</summary>
        <ol>
          {h.schedules.map((v) => (
            <li key={v.version}>
              С {v.effective_from}: {v.schedule.target} {h.unit}{" "}
              {v.schedule.frequency === "daily"
                ? "в запланированный день"
                : v.schedule.frequency === "weekly"
                  ? "за неделю"
                  : "за месяц"}
              . {v.reason}
            </li>
          ))}
        </ol>
        {!h.archived_at && (
          <ScheduleForm key={h.id + String(!!h.archived_at)} habit={h} changed={changed} />
        )}
      </details>
      <details>
        <summary>История периодов и исправлений</summary>
        <HistoryPages habit={h} changed={changed} />
      </details>
      <details>
        <summary>{h.archived_at ? "Вернуть привычку" : "Убрать в архив"}</summary>
        <ArchiveForm key={h.id + String(!!h.archived_at)} habit={h} changed={changed} />
        <ul>
          {h.archives.map((a) => (
            <li key={a.id}>
              Архив: {a.archived_at.slice(0, 10)} · {a.archive_reason}
              {a.restored_at && ` · Возврат: ${a.restored_at.slice(0, 10)} · ${a.restore_reason}`}
            </li>
          ))}
        </ul>
      </details>
    </>
  );
}
function ArchiveForm({ habit: h, changed }: { habit: Habit; changed: () => void }) {
  const record = useRef(h);
  const key = useKey();
  const flow = useValidatedForm({
    schema: why,
    defaultValues: { reason: "" },
    fields: ["reason"],
    save: async (v) => {
      record.current = await archiveHabit(
        record.current,
        !!h.archived_at,
        v.reason,
        key({ input: v, version: record.current.version }),
      );
      changed();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <HabitVersion recordRef={record} habit={h} flow={flow} />
      <p>
        {h.archived_at
          ? "История сохранится. Новые обязательства начнутся со следующего периода; период, начатый до архива, сохранится."
          : "История останется доступной. В архиве новые обязательные периоды не создаются."}
      </p>
      <Input
        label={h.archived_at ? "Причина возврата" : "Причина архива"}
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      <Button type="submit" variant="secondary">
        {h.archived_at ? "Вернуть привычку" : "Архивировать привычку"}
      </Button>
    </Form>
  );
}
function ScheduleForm({ habit: h, changed }: { habit: Habit; changed: () => void }) {
  const [preview, setPreview] = useState<{
      data: HabitSchedulePreview;
      input: HabitScheduleChange;
    }>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [accepted, setAccepted] = useState(false);
  const record = useRef(h);
  const key = useKey();
  useUnsavedChanges(!!preview);
  const current = h.schedules.at(-1)?.schedule ?? h.schedule;
  const flow = useValidatedForm({
    schema: habitScheduleChange,
    defaultValues: { effective_from: "", schedule: current, reason: "" },
    fields: [
      "effective_from",
      "schedule.frequency",
      "schedule.target",
      "schedule.weekdays",
      "reason",
    ],
    save: async (v) => {
      const data = await previewHabitSchedule(record.current, v);
      setPreview({ data, input: v });
      setAccepted(false);
    },
  });
  const f = flow.form,
    frequency = useWatch({ control: f.control, name: "schedule.frequency" }),
    days = useWatch({ control: f.control, name: "schedule.weekdays" });
  async function apply() {
    if (!preview || busy || !accepted) return;
    setBusy(true);
    setError("");
    try {
      record.current = await changeHabitSchedule(
        { ...record.current, version: preview.data.version },
        preview.input,
        key({ input: preview.input, version: preview.data.version }),
      );
      setPreview(undefined);
      changed();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Не удалось применить расписание");
      if (e instanceof ApiError && e.code === "VERSION_CONFLICT") {
        flow.form.setError("root.server", { type: e.code, message: e.message });
        setPreview(undefined);
        setAccepted(false);
      }
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Form saving={flow.saving || busy} error={flow.error} onSubmit={flow.onSubmit}>
        <HabitVersion recordRef={record} habit={h} flow={flow} frozen={!!preview} />
        <Input
          label="Дата новой нормы"
          type="date"
          {...f.register("effective_from")}
          error={f.formState.errors.effective_from?.message}
        />
        <Select
          label="Новый период нормы"
          {...f.register("schedule.frequency")}
          onChange={(e) => {
            const v = e.target.value as "daily" | "weekly" | "monthly";
            f.setValue("schedule.frequency", v, { shouldDirty: true });
            f.setValue("schedule.weekdays", v === "daily" ? [1, 2, 3, 4, 5, 6, 7] : []);
          }}
        >
          <option value="daily">Выбранные дни</option>
          <option value="weekly">За неделю</option>
          <option value="monthly">За месяц</option>
        </Select>
        <Input
          label="Новая норма"
          type="number"
          step="0.001"
          {...f.register("schedule.target", { valueAsNumber: true })}
          error={f.formState.errors.schedule?.target?.message}
        />
        {frequency === "daily" && (
          <fieldset className="habit-weekdays">
            <legend>Новые запланированные дни</legend>
            {["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"].map((label, i) => (
              <label key={label}>
                <input
                  type="checkbox"
                  checked={days.includes(i + 1)}
                  onChange={(e) =>
                    f.setValue(
                      "schedule.weekdays",
                      e.target.checked ? [...days, i + 1] : days.filter((d) => d !== i + 1),
                      { shouldDirty: true },
                    )
                  }
                />{" "}
                {label}{" "}
              </label>
            ))}
          </fieldset>
        )}
        <Input
          label="Причина смены нормы"
          {...f.register("reason")}
          error={f.formState.errors.reason?.message}
        />
        <p className="ui-hint">
          Только будущая общая граница старого и нового периодов. Прошлые нормы не меняются.
        </p>
        <Button type="submit" variant="secondary">
          Предпросмотр нормы
        </Button>
      </Form>
      {preview && (
        <div aria-label="Предпросмотр нормы">
          <p>
            С {preview.data.effective_from}: {preview.data.current.target} →{" "}
            {preview.data.proposed.target} {h.unit}. Первый период до{" "}
            {DateTime.fromISO(preview.data.first_end)
              .setZone(h.timezone)
              .toFormat("dd.MM.yyyy HH:mm")}
            .
          </p>
          <label>
            <input
              type="checkbox"
              checked={accepted}
              onChange={(e) => setAccepted(e.target.checked)}
            />{" "}
            Я подтверждаю новую норму с указанной даты
          </label>
          <Button disabled={!accepted || busy} onClick={() => void apply()}>
            Применить новую норму
          </Button>
          <Button variant="ghost" disabled={busy} onClick={() => setPreview(undefined)}>
            Отменить предпросмотр
          </Button>
        </div>
      )}
      {error && <p role="alert">{error}</p>}
    </>
  );
}
function HistoryPages({ habit: h, changed }: { habit: Habit; changed: () => void }) {
  const auth = useAuth();
  const periods = useInfiniteQuery({
    queryKey: ["account", auth.session?.account.id, "habit-periods", h.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => getHabitPeriodPage(h, pageParam, signal),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  const logs = useInfiniteQuery({
    queryKey: ["account", auth.session?.account.id, "habit-logs", h.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => getHabitLogPage(h, pageParam, signal),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  const [selected, setSelected] = useState("");
  const all = periods.data?.pages.flatMap((p) => p.items) ?? [];
  const period = all.find((p) => p.id === selected);
  return (
    <>
      {(periods.isPending || logs.isPending) && <p role="status">Загружаем историю…</p>}
      {(periods.isError || logs.isError) && (
        <p role="alert">
          Не удалось загрузить историю.{" "}
          <Button
            onClick={() => {
              void periods.refetch();
              void logs.refetch();
            }}
          >
            Повторить
          </Button>
        </p>
      )}
      <ul>
        {all.map((p) => (
          <li key={p.id}>
            {DateTime.fromISO(p.start).setZone(h.timezone).toISODate()}: {p.amount} / {p.target}{" "}
            {h.unit} · {p.progress}% ·{" "}
            {p.excluded
              ? "Отдых"
              : p.state === "missed"
                ? "Пропуск"
                : p.state === "success"
                  ? "Выполнено"
                  : "Период открыт"}{" "}
            {p.emergency_reason || p.rest_reason}
          </li>
        ))}
      </ul>
      {periods.hasNextPage && (
        <Button
          variant="secondary"
          disabled={periods.isFetchingNextPage}
          onClick={() => void periods.fetchNextPage()}
        >
          Более ранние периоды
        </Button>
      )}
      {!h.archived_at && (
        <>
          <Select
            label="Период для исторической отметки"
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
          >
            <option value="">Выберите период</option>
            {all
              .filter((p) => !p.excluded)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {DateTime.fromISO(p.start).setZone(h.timezone).toISODate()} · {p.amount}/
                  {p.target}
                </option>
              ))}
          </Select>
          {period && (
            <HistoricalLogForm key={period.id} habit={h} period={period} changed={changed} />
          )}
        </>
      )}
      <h3>Отметки и исправления</h3>
      <ol>
        {logs.data?.pages
          .flatMap((p) => p.items)
          .map((l) => (
            <li key={l.id}>
              <p>
                {DateTime.fromISO(l.occurred_at).setZone(h.timezone).toFormat("dd.MM.yyyy HH:mm")}:{" "}
                {l.amount} {h.unit} · исходно {l.original_amount}{" "}
                {l.backfilled && " · Внесено задним числом"}
              </p>
              <ul>
                {l.corrections.map((c) => (
                  <li key={c.id}>
                    {c.before} → {c.after}: {c.reason} · {c.created_at.slice(0, 10)}
                  </li>
                ))}
              </ul>
              <details>
                <summary>Исправить отметку</summary>
                <CorrectionForm
                  key={l.id}
                  habit={h}
                  log={l.id}
                  amount={l.amount}
                  changed={changed}
                />
              </details>
            </li>
          ))}
      </ol>
      {logs.hasNextPage && (
        <Button
          variant="secondary"
          disabled={logs.isFetchingNextPage}
          onClick={() => void logs.fetchNextPage()}
        >
          Более ранние отметки
        </Button>
      )}
    </>
  );
}
function CorrectionForm({
  habit: h,
  log,
  amount,
  changed,
}: {
  habit: Habit;
  log: string;
  amount: number;
  changed: () => void;
}) {
  const record = useRef(h);
  const key = useKey(),
    rewardNotice = useRewardNotice();
  const schema = why.extend({
    amount: z.number().min(0, "Количество не может быть отрицательным").max(1000000),
  });
  const flow = useValidatedForm({
    schema,
    defaultValues: { amount, reason: "" },
    fields: ["amount", "reason"],
    save: async (v) => {
      const saved = await correctHabitLog(
        record.current,
        log,
        v,
        key({ input: v, version: record.current.version }),
      );
      record.current = saved;
      rewardNotice(saved.reward);
      changed();
    },
  });
  return (
    <Form onSubmit={flow.onSubmit} saving={flow.saving} error={flow.error}>
      <HabitVersion recordRef={record} habit={h} flow={flow} frozen />
      <Input
        label="Исправленное количество"
        type="number"
        step="0.001"
        {...flow.form.register("amount", { valueAsNumber: true })}
        error={flow.form.formState.errors.amount?.message}
      />
      <Input
        label="Причина исправления"
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      <Button type="submit" variant="secondary">
        Сохранить исправление
      </Button>
    </Form>
  );
}
function HistoricalLogForm({
  habit: h,
  period: p,
  changed,
}: {
  habit: Habit;
  period: HabitPeriod;
  changed: () => void;
}) {
  const record = useRef(h);
  const key = useKey(),
    rewardNotice = useRewardNotice();
  const schema = z.object({
    amount: z.number().positive("Укажите положительное количество"),
    occurred_at: z.string().min(1),
  });
  const flow = useValidatedForm({
    schema,
    defaultValues: {
      amount: h.minimum,
      occurred_at: DateTime.fromISO(p.start).setZone(h.timezone).toFormat("yyyy-MM-dd'T'HH:mm"),
    },
    fields: ["amount", "occurred_at"],
    save: async (v) => {
      const input = {
        amount: v.amount,
        period_id: p.id,
        occurred_at: DateTime.fromISO(v.occurred_at, { zone: h.timezone }).toUTC().toISO()!,
      };
      const saved = await logHabit(
        record.current,
        input,
        key({ input: input, version: record.current.version }),
      );
      record.current = saved;
      rewardNotice(saved.reward);
      changed();
    },
  });
  return (
    <Form onSubmit={flow.onSubmit} saving={flow.saving} error={flow.error}>
      <HabitVersion recordRef={record} habit={h} flow={flow} />
      <p className="ui-hint">
        Сохраняйте фактическое время. Позднее внесение отмечается в истории.
      </p>
      <Input
        label="Количество исторической отметки"
        type="number"
        step="0.001"
        {...flow.form.register("amount", { valueAsNumber: true })}
        error={flow.form.formState.errors.amount?.message}
      />
      <Input
        label={`Историческое время факта (${h.timezone})`}
        type="datetime-local"
        {...flow.form.register("occurred_at")}
        error={flow.form.formState.errors.occurred_at?.message}
      />
      <Button type="submit">Добавить историческую отметку</Button>
    </Form>
  );
}
