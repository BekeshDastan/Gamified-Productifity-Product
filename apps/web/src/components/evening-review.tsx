"use client";
import { useRef, useState } from "react";
import type { z } from "zod";
import { statuses } from "@/lib/quests";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import {
  getReview,
  saveReview,
  reviewInput,
  getRoutineTemplate,
  type RoutineTemplate,
  type Review,
} from "@/lib/journal";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Select } from "./ui/field";
import { useTabDraft, TabDraftRecovery } from "@/lib/forms/tab-draft";
import { draftKey } from "@/lib/private-drafts";
import { RoutineChecklist } from "./routine-templates";
import { ChoiceChips, StatTile } from "./ui/experience";
import { CircleCheck, CalendarCheck, ArrowRight, Moon, Battery, Smile } from "lucide-react";
import { Form } from "./ui/form";
const obstacles = {
  none: "Не было",
  insufficient_time: "Не хватило времени",
  low_energy: "Мало энергии",
  external: "Внешние обстоятельства",
  unclear: "Неясный следующий шаг",
  other: "Другое",
};
export function EveningReview({ initialDate }: { initialDate?: string } = {}) {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p>Загружаем вечерний разбор…</p>;
  return (
    <Evening
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
      initialDate={initialDate}
    />
  );
}
function Evening({
  account,
  timezone,
  initialDate,
}: {
  account: string;
  timezone: string;
  initialDate?: string;
}) {
  const discard = useConfirmDiscard(),
    cache = useQueryClient();
  const [date, setDate] = useState(() =>
      initialDate &&
      /^\d{4}-\d{2}-\d{2}$/.test(initialDate) &&
      DateTime.fromISO(initialDate).isValid
        ? initialDate
        : DateTime.now().setZone(timezone).toISODate()!,
    ),
    [notice, setNotice] = useState("");
  const q = useQuery({
    queryKey: accountQueryKey(account, "evening", date),
    queryFn: ({ signal }) => getReview(date, signal),
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const template = useQuery({
    queryKey: accountQueryKey(account, "routine-template", "evening"),
    queryFn: ({ signal }) => getRoutineTemplate("evening", signal),
  });
  return (
    <div className="journal-layout">
      <Card>
        <h2>Вечерний разбор</h2>
        <p>
          Итоги ниже получены из ваших задач. Перенос задач выполняется отдельно и не происходит от
          записи вывода. Энергия и настроение — необязательные личные отметки.
        </p>
        <label className="ui-field">
          Дата вечернего разбора
          <input
            className="ui-input"
            type="date"
            max={DateTime.now().setZone(timezone).toISODate()!}
            value={date}
            onChange={(e) => {
              if (e.target.value && discard()) {
                setDate(e.target.value);
                setNotice("");
              }
            }}
          />
        </label>
        {q.isError && (
          <p role="alert">
            Не удалось загрузить разбор. <Button onClick={() => void q.refetch()}>Повторить</Button>
          </p>
        )}
        {notice && <p role="status">{notice}</p>}
      </Card>
      {q.data && template.data && (
        <ReviewEditor
          key={date + ":" + q.data.run?.version}
          account={account}
          template={template.data}
          date={date}
          review={q.data}
          saved={async () => {
            await cache.invalidateQueries({ queryKey: accountQueryKey(account) });
            setNotice("Вечерний разбор сохранён.");
          }}
        />
      )}
    </div>
  );
}
function ReviewEditor({
  account,
  template,
  date,
  review,
  saved,
}: {
  account: string;
  template: RoutineTemplate;
  date: string;
  review: Review;
  saved: () => Promise<void>;
}) {
  const [frozen] = useState(template);
  const operation = useRef({ body: "", key: "" });
  const initial = reviewInput.safeParse(review.run?.payload.answers);
  const { form, onSubmit, saving, error } = useValidatedForm<z.infer<typeof reviewInput>>({
    schema: reviewInput,
    defaultValues: initial.success
      ? {
          ...initial.data,
          template_version: frozen.version,
          checked: initial.data.checked.filter((id) => frozen.steps.some((step) => step.id === id)),
        }
      : {
          template_version: frozen.version,
          checked: [],
          obstacle: "none",
          obstacle_note: "",
          conclusion: "",
          energy: null,
          mood: null,
        },
    fields: ["obstacle", "obstacle_note", "conclusion", "energy", "mood"],
    save: async (input) => {
      const body = JSON.stringify(input);
      if (operation.current.body !== body) operation.current = { body, key: crypto.randomUUID() };
      await saveReview(date, review, input, operation.current.key);
      draft.clear();
      await saved();
    },
  });
  const draft = useTabDraft({
    storageKey: draftKey(account, "evening:" + date),
    schema: reviewInput,
    value: form.watch(),
    dirty: form.formState.isDirty,
    restore: (values) =>
      form.reset(
        {
          ...values,
          template_version: frozen.version,
          checked: values.checked.filter((id) => frozen.steps.some((step) => step.id === id)),
        },
        { keepDefaultValues: true },
      ),
  });
  return (
    <Card className="evening-studio">
      <h2>Что получилось за день</h2>
      <div className="stat-grid">
        <StatTile label="В плане" value={review.stats.planned} icon={<CalendarCheck />} />
        <StatTile
          label="Выполнено сегодня"
          value={review.stats.completed_today}
          detail={`Из плана: ${review.stats.completed}`}
          tone="mint"
          icon={<CircleCheck />}
        />
        <StatTile
          label="Перенесено"
          value={review.stats.postponed}
          tone="gold"
          icon={<ArrowRight />}
        />
        <StatTile
          label="Без завершения"
          value={review.stats.failed + review.stats.skipped}
          detail={`Не достигнуто: ${review.stats.failed} · Пропущено: ${review.stats.skipped}`}
          tone="blue"
          icon={<Moon />}
        />
      </div>
      <details>
        <summary>Исходные задачи</summary>
        {review.stats.items.length === 0 ? (
          <p>Задач за этот день нет.</p>
        ) : (
          <ul>
            {review.stats.items.map((item) => (
              <li key={item.id}>
                <SafeLink href={"/app/quests/" + item.id}>{item.title}</SafeLink> ·{" "}
                {statuses[item.status as keyof typeof statuses] ?? item.status} · плановая дата:{" "}
                {item.planned_date ?? "не задана"}
              </li>
            ))}
          </ul>
        )}
        {review.stats.more && <p>Показаны первые 500 задач; итог посчитан по всем источникам.</p>}
      </details>
      <Form saving={saving} error={error} onSubmit={onSubmit}>
        <TabDraftRecovery draft={draft} />
        <RoutineChecklist
          template={frozen}
          checked={form.watch("checked")}
          onChange={(value) => form.setValue("checked", value, { shouldDirty: true })}
        />
        <Select label="Главное препятствие" {...form.register("obstacle")}>
          {Object.entries(obstacles).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <label className="ui-field" htmlFor="evening-obstacle-note">
          Заметка о препятствии
        </label>
        <textarea
          id="evening-obstacle-note"
          className="ui-input"
          maxLength={2000}
          {...form.register("obstacle_note")}
        />
        <label className="ui-field" htmlFor="evening-conclusion">
          Один вывод дня
        </label>
        <textarea
          id="evening-conclusion"
          className="ui-input"
          maxLength={5000}
          {...form.register("conclusion")}
        />
        {form.formState.errors.conclusion && <p role="alert">Запишите хотя бы один вывод.</p>}
        {(["energy", "mood"] as const).map((field) => (
          <section className="feeling-picker" key={field} data-kind={field}>
            <h3>
              {field === "energy" ? (
                <Battery size={21} aria-hidden="true" />
              ) : (
                <Smile size={21} aria-hidden="true" />
              )}
              {field === "energy" ? "Сколько осталось энергии?" : "Как вы себя чувствуете?"}
            </h3>
            <ChoiceChips
              label={field === "energy" ? "Энергия (необязательно)" : "Настроение (необязательно)"}
              value={form.watch(field) ?? 0}
              options={[
                { value: 0, label: "Без отметки" },
                ...(field === "energy"
                  ? ["На нуле", "Немного", "Средне", "Хорошо", "Полон сил"]
                  : ["Тяжело", "Не очень", "Нейтрально", "Хорошо", "Отлично"]
                ).map((label, i) => ({ value: i + 1, label })),
              ]}
              onChange={(value) => form.setValue(field, value || null, { shouldDirty: true })}
            />
          </section>
        ))}
        <Button type="submit">Сохранить вечерний разбор</Button>
      </Form>
    </Card>
  );
}
