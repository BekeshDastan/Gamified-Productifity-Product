"use client";
import { z } from "zod";
import { useTabDraft, TabDraftRecovery } from "@/lib/forms/tab-draft";
import { draftKey } from "@/lib/private-drafts";
import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/client";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { applyWeekReview, type WeekPreview, type WeekApplyInput } from "@/lib/journal";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
const weeklyDraftSchema = z.object({
  choices: z.record(
    z.string(),
    z.object({
      action: z.enum(["keep", "move", "archive"]),
      date: z.string(),
      reason: z.string().max(1000),
      top: z.boolean(),
    }),
  ),
  days: z.record(
    z.string(),
    z.object({ selected: z.boolean(), minutes: z.number().min(0).max(1440), rest: z.boolean() }),
  ),
  reason: z.string().max(1000),
});
type Choice = { action: "keep" | "move" | "archive"; date: string; reason: string; top: boolean };
export function WeeklyResetApply({
  preview,
  account,
  saved,
}: {
  preview: WeekPreview;
  account: string;
  saved: () => void;
}) {
  const snap = preview.snapshot,
    cache = useQueryClient();
  const [choices, setChoices] = useState<Record<string, Choice>>({}),
    [days, setDays] = useState<
      Record<string, { selected: boolean; minutes: number; rest: boolean }>
    >({}),
    [reason, setReason] = useState(""),
    [confirm, setConfirm] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const operation = useRef({ body: "", key: "" }),
    lock = useRef(false);
  const changes = snap.items.filter((item) => (choices[item.id]?.action ?? "keep") !== "keep");
  useUnsavedChanges(
    changes.length > 0 || Object.values(days).some((day) => day.selected) || reason !== "" || busy,
  );
  const draft = useTabDraft({
    storageKey: draftKey(account, "weekly:" + snap.week),
    schema: weeklyDraftSchema,
    value: { choices, days, reason },
    dirty: changes.length > 0 || Object.values(days).some((day) => day.selected) || reason !== "",
    restore: (values) => {
      setChoices(values.choices);
      setDays(values.days);
      setReason(values.reason);
      setConfirm(false);
    },
  });
  function choice(id: string): Choice {
    return choices[id] ?? { action: "keep", date: snap.next_week, reason: "", top: false };
  }
  function update(id: string, patch: Partial<Choice>) {
    setChoices((old) => ({ ...old, [id]: { ...choice(id), ...patch } }));
  }
  async function apply() {
    if (lock.current) return;
    const input: WeekApplyInput = {
      preview_id: preview.id,
      reason,
      changes: changes.map((item) => {
        const c = choice(item.id);
        return {
          id: item.id,
          version: item.version,
          action: c.action as "move" | "archive",
          planned_date: c.action === "move" ? c.date : null,
          reason: c.reason,
        };
      }),
      plans: snap.plans
        .filter((plan) => days[plan.date]?.selected)
        .map((plan) => {
          const day = days[plan.date] ?? {
            selected: false,
            minutes: plan.available_minutes,
            rest: plan.rest,
          };
          const removed = new Set(
            changes
              .filter(
                (item) =>
                  choice(item.id).action === "archive" || choice(item.id).date !== plan.date,
              )
              .map((item) => item.id),
          );
          const moved = changes.filter(
            (item) => choice(item.id).action === "move" && choice(item.id).date === plan.date,
          );
          const ids = [
            ...new Set([
              ...plan.quest_ids.filter((id) => !removed.has(id)),
              ...moved.map((item) => item.id),
            ]),
          ];
          const tops = [
            ...new Set([
              ...plan.top_ids.filter((id) => ids.includes(id)),
              ...moved.filter((item) => choice(item.id).top).map((item) => item.id),
            ]),
          ];
          return {
            date: plan.date,
            version: plan.version,
            input: {
              available_minutes: day.rest ? 0 : day.minutes,
              windows: day.rest ? [] : plan.windows,
              quest_ids: day.rest ? [] : ids,
              top_ids: day.rest ? [] : tops,
              rest: day.rest,
            },
          };
        }),
    };
    if (
      !confirm ||
      !reason.trim() ||
      input.changes.some((change) => !change.reason.trim()) ||
      input.plans.some((plan) => plan.input.top_ids.length > 3)
    ) {
      setError(
        "Подтвердите решение, укажите причины и выберите не более трёх главных задач на день.",
      );
      return;
    }
    lock.current = true;
    setBusy(true);
    setError("");
    const body = JSON.stringify(input);
    if (operation.current.body !== body) operation.current = { body, key: crypto.randomUUID() };
    try {
      await applyWeekReview(snap.week, preview, input, operation.current.key);
      void cache.invalidateQueries({ queryKey: accountQueryKey(account) });
      draft.clear();
      saved();
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 409
          ? "Предпросмотр изменился или истёк. Подготовьте новый; выбранные изменения ещё видны ниже."
          : "Не удалось применить обзор. Изменения выполняются вместе; можно безопасно повторить запрос.",
      );
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>Выбрать изменения Weekly Reset</h2>
      <TabDraftRecovery draft={draft} />
      <p>
        Без выбора задача остаётся как есть. Перенос меняет плановую дату; дедлайн и фиксированные
        события сохраняются. Планы отмеченных дней будут подтверждены с сохранением исходных
        снимков.
      </p>
      {error && <p role="alert">{error}</p>}
      <fieldset disabled={busy} className="ui-form-fields">
        <legend>Незавершённые задачи</legend>
        {snap.items.map((item) => {
          const c = choice(item.id);
          return (
            <div key={item.id}>
              <h3>{item.title}</h3>
              <p>Исходная дата: {item.planned_date ?? "не задана"}</p>
              <label className="ui-field">
                Действие для «{item.title}»
                <select
                  className="ui-input"
                  value={c.action}
                  onChange={(e) => update(item.id, { action: e.target.value as Choice["action"] })}
                >
                  <option value="keep">Оставить как есть</option>
                  <option value="move">Перенести</option>
                  <option value="archive">Архивировать</option>
                </select>
              </label>
              {c.action === "move" && (
                <>
                  <label className="ui-field">
                    Новая дата для «{item.title}»
                    <select
                      className="ui-input"
                      value={c.date}
                      onChange={(e) => update(item.id, { date: e.target.value })}
                    >
                      {snap.plans.map((p) => (
                        <option key={p.date} value={p.date}>
                          {p.date}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={c.top}
                      onChange={(e) => update(item.id, { top: e.target.checked })}
                    />{" "}
                    Главная задача на выбранный день
                  </label>
                </>
              )}
              {c.action !== "keep" && (
                <label className="ui-field">
                  Причина для «{item.title}»
                  <input
                    className="ui-input"
                    maxLength={1000}
                    value={c.reason}
                    onChange={(e) => update(item.id, { reason: e.target.value })}
                  />
                </label>
              )}
            </div>
          );
        })}
      </fieldset>
      <fieldset disabled={busy} className="ui-form-fields">
        <legend>Планы следующей недели</legend>
        {snap.plans.map((plan) => {
          const day = days[plan.date] ?? {
            selected: false,
            minutes: plan.available_minutes || 60,
            rest: plan.rest,
          };
          return (
            <div key={plan.date}>
              <label>
                <input
                  type="checkbox"
                  checked={day.selected}
                  onChange={(e) =>
                    setDays((old) => ({
                      ...old,
                      [plan.date]: { ...day, selected: e.target.checked },
                    }))
                  }
                />{" "}
                Подтвердить план на {plan.date}
              </label>
              {day.selected && (
                <>
                  <label className="ui-field">
                    Доступно минут на {plan.date}
                    <input
                      className="ui-input"
                      type="number"
                      min={0}
                      max={1440}
                      value={day.minutes}
                      disabled={day.rest}
                      onChange={(e) =>
                        setDays((old) => ({
                          ...old,
                          [plan.date]: { ...day, minutes: Number(e.target.value) },
                        }))
                      }
                    />
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={day.rest}
                      onChange={(e) =>
                        setDays((old) => ({
                          ...old,
                          [plan.date]: { ...day, rest: e.target.checked },
                        }))
                      }
                    />{" "}
                    День отдыха без задач
                  </label>
                  <p>
                    В план войдут прежние задачи этого дня и выбранные переносы. Архивируемые и
                    переносимые на другой день задачи исключены. Фиксированные события не
                    перемещаются.
                  </p>
                </>
              )}
            </div>
          );
        })}
      </fieldset>
      <label className="ui-field">
        Вывод и причина недельного решения
        <textarea
          className="ui-input"
          maxLength={1000}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </label>
      <p>
        Выбрано изменений задач: {changes.length}; планов дней:{" "}
        {Object.values(days).filter((day) => day.selected).length}.
      </p>
      <label>
        <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />{" "}
        Подтверждаю выбранные переносы, архив и планы
      </label>
      <div className="form-actions">
        <Button disabled={busy || !confirm} onClick={() => void apply()}>
          Применить Weekly Reset
        </Button>
      </div>
    </Card>
  );
}
