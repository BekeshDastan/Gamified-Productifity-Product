"use client";
import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { metricEntries, saveMetric, type MetricEntry, type MetricInput } from "@/lib/goal-metrics";
import type { Goal } from "@/lib/goals";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
export function GoalMetrics({ goal, onChanged }: { goal: Goal; onChanged: () => void }) {
  const account = useAuth().session?.account.id;
  const [before, setBefore] = useState<number>();
  const discard = useConfirmDiscard();
  const entries = useQuery({
    queryKey: ["account", account, "goal-entries", goal.id, before],
    queryFn: ({ signal }) => metricEntries(goal.id, before, undefined, signal),
  });
  return (
    <Card>
      <h2>Журнал результатов</h2>
      <p>
        Начало: {goal.start} · Сейчас: {goal.current} · Цель: {goal.target} {goal.unit}
      </p>
      <p className="ui-hint">
        Добавляйте только результаты в единице «{goal.unit}». Исправление сохраняет прежнее значение
        и причину.
      </p>
      {!goal.archived_at && ["draft", "active", "paused"].includes(goal.state) && (
        <MetricForm goal={goal} onChanged={onChanged} />
      )}
      {entries.isPending ? (
        <p role="status">Загружаем результаты…</p>
      ) : entries.data ? (
        <>
          <ul className="quest-batch">
            {entries.data.map((e) => (
              <li className="goal-plan-part" key={e.id}>
                <h3>
                  {e.value} {e.unit} · {e.occurred_on}
                </h3>
                <p>
                  {e.note || "Без заметки"} · версия {e.version}
                </p>
                {!goal.archived_at && (
                  <details>
                    <summary>Исправить результат</summary>
                    <MetricForm
                      key={e.id + ":" + e.version}
                      goal={goal}
                      current={e}
                      onChanged={onChanged}
                    />
                  </details>
                )}
                <MetricHistory goal={goal} entry={e} />
              </li>
            ))}
          </ul>
          {entries.data.length === 0 && <p>Результаты пока не добавлены.</p>}
          <div className="button-row">
            {entries.data.length === 50 && (
              <Button
                variant="secondary"
                onClick={() => {
                  if (discard()) setBefore(entries.data.at(-1)?.id);
                }}
              >
                Более ранние результаты
              </Button>
            )}
            {before && (
              <Button
                variant="secondary"
                onClick={() => {
                  if (discard()) setBefore(undefined);
                }}
              >
                Последние результаты
              </Button>
            )}
          </div>
        </>
      ) : (
        <p role="alert">
          Результаты не загрузились.{" "}
          <Button onClick={() => void entries.refetch()}>Повторить</Button>
        </p>
      )}
    </Card>
  );
}
const schema = z.object({
  value: z.number().min(0).max(1e9),
  occurred_on: z.iso.date("Укажите дату результата"),
  note: z.string().max(1000),
  reason: z.string().max(1000),
});
function MetricForm({
  goal,
  current,
  onChanged,
}: {
  goal: Goal;
  current?: MetricEntry;
  onChanged: () => void;
}) {
  const retry = useRef<{ body: string; key: string; version: number } | undefined>(undefined);
  const flow = useValidatedForm({
    schema,
    defaultValues: current ?? { value: 1, occurred_on: "", note: "", reason: "" },
    fields: ["value", "occurred_on", "note", "reason"],
    save: async (v) => {
      const input: MetricInput = { ...v, unit: goal.unit, entry_version: current?.version ?? 0 };
      const body = JSON.stringify(input);
      if (retry.current?.body !== body)
        retry.current = { body, key: crypto.randomUUID(), version: goal.version };
      await saveMetric(
        { ...goal, version: retry.current.version },
        input,
        retry.current.key,
        current,
      );
      retry.current = undefined;
      onChanged();
      return current ? v : { value: 1, occurred_on: "", note: "", reason: "" };
    },
  });
  const f = flow.form;
  const e = f.formState.errors;
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label={current ? "Исправленное значение" : "Добавить результат"}
        type="number"
        step="0.001"
        error={e.value?.message}
        {...f.register("value", { valueAsNumber: true })}
      />
      <DatePicker
        label="Дата результата"
        error={e.occurred_on?.message}
        {...f.register("occurred_on")}
      />
      <Input label="Заметка к результату" maxLength={1000} {...f.register("note")} />
      {current && (
        <Input
          label="Причина исправления результата"
          maxLength={1000}
          error={e.reason?.message}
          {...f.register("reason")}
        />
      )}
      <Button type="submit" variant="secondary">
        {current ? "Сохранить исправление" : "Записать результат"}
      </Button>
    </Form>
  );
}
function MetricHistory({ goal, entry }: { goal: Goal; entry: MetricEntry }) {
  const account = useAuth().session?.account.id;
  const [open, setOpen] = useState(false);
  const [before, setBefore] = useState<number>();
  const history = useQuery({
    queryKey: ["account", account, "metric-history", goal.id, entry.id, entry.version, before],
    enabled: open,
    queryFn: ({ signal }) => metricEntries(goal.id, before, entry.id, signal),
  });
  return (
    <details onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>История результата</summary>
      {history.isPending ? (
        <p role="status">Загружаем историю…</p>
      ) : history.data ? (
        <>
          <ol>
            {history.data.map((v) => (
              <li key={v.id}>
                {v.value} {v.unit} · {v.occurred_on} · версия {v.version}
                <p>{v.reason || "Первоначальная запись"}</p>
                <p>{v.note}</p>
              </li>
            ))}
          </ol>
          {history.data.length === 50 && (
            <Button onClick={() => setBefore(history.data.at(-1)?.id)}>
              Более ранние исправления
            </Button>
          )}
          {before && <Button onClick={() => setBefore(undefined)}>Последние исправления</Button>}
        </>
      ) : (
        <p role="alert">
          История не загрузилась. <Button onClick={() => void history.refetch()}>Повторить</Button>
        </p>
      )}
    </details>
  );
}
