"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import {
  emptyQuest,
  emptyFilters,
  getQuest,
  listQuests,
  questInput,
  questLabels,
  saveQuest,
  statuses,
  type Quest,
  type QuestInput,
} from "@/lib/quests";
import { DeadlineField } from "./deadline-field";
import { useSettings, formatMoment } from "@/lib/settings";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { ErrorState, LoadingState } from "./ui/states";
import { Input, Select, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
import { QuestFiltersPanel } from "./quest-filters";
import { QuestFocusTime } from "./focus-review";
import { QuickFocusStart } from "./quick-focus";
import { QuestActions, QuestHistory } from "./quest-actions";
import { QuestStructure, QuestPicker } from "./quest-structure";
import { QuestFiles } from "./quest-files";
import { QuestTemplates, QuestArchive, QuestBatchPanel } from "./quest-tools";
import { QuestGoalLink } from "./goal-plan";
import { DateTime } from "luxon";
import { CalendarDays, Clock3, Flag, ListChecks } from "lucide-react";
import { ChoiceChips } from "./ui/experience";

export function Quests({ id }: { id?: string }) {
  const auth = useAuth();
  const account = auth.session?.account.id;
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState(emptyFilters);
  const [board, setBoard] = useState(false);
  const [revision, setRevision] = useState(0);
  const [newParent, setNewParent] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [created, setCreated] = useState<Quest>();
  const confirmDiscard = useConfirmDiscard();
  const query = useQuery<Quest | Awaited<ReturnType<typeof listQuests>>>({
    queryKey: ["account", account, "quests", id ?? "", page, filters],
    enabled: !!account,
    queryFn: ({ signal }) => (id ? getQuest(id, signal) : listQuests(page, signal, filters)),
  });
  const labels = useQuery({
    queryKey: ["account", account, "quest-labels"],
    enabled: !!account,
    queryFn: ({ signal }) => questLabels(signal),
  });
  const settings = useSettings();
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!account)
    return (
      <p>
        Для работы с задачами{" "}
        <SafeLink className="text-link" href="/auth/login?next=/app/quests">
          войдите в аккаунт
        </SafeLink>
        .
      </p>
    );
  if (query.isPending || labels.isPending) return <LoadingState label="Загружаем задачи…" />;
  if (!query.data || !labels.data)
    return (
      <ErrorState
        error={query.error ?? labels.error}
        retry={() => {
          void query.refetch();
          void labels.refetch();
        }}
      />
    );
  const current = "id" in query.data ? query.data : undefined;
  const collection = "items" in query.data ? query.data : undefined;
  return (
    <div className="quest-workspace">
      <div className="button-row">
        <Button
          onClick={() => {
            if (confirmDiscard()) {
              setEditing(!editing);
              setCreated(undefined);
            }
          }}
        >
          {editing ? "Закрыть форму" : "Создать задачу"}
        </Button>
        <SafeLink className="text-link" href="/app/settings/areas">
          Направления и теги
        </SafeLink>
      </div>
      {created && (
        <p role="status">
          Задача сохранена:{" "}
          <SafeLink className="text-link" href={"/app/quests/" + created.id}>
            {created.title}
          </SafeLink>
        </p>
      )}
      {editing && (
        <QuestForm
          key={newParent ?? "new"}
          initialParent={newParent}
          labels={labels.data}
          onSaved={async (q) => {
            await query.refetch();
            setCreated(q);
          }}
        />
      )}
      {current && (
        <>
          <SafeLink className="text-link" href="/app/quests">
            Все задачи
          </SafeLink>
          <Card className="task-detail-hero" data-priority={current.priority}>
            <span className="detail-emblem" aria-hidden="true">
              <ListChecks size={29} />
            </span>
            <h2>{current.title}</h2>
            <p>
              {statuses[current.status]}
              {current.archived_at ? " · В архиве" : ""}
            </p>
            {current.completed_at && settings.data && (
              <p>Завершена: {formatMoment(current.completed_at, settings.data)}</p>
            )}
            <p className="quest-description">
              {current.description || "Описание пока не добавлено."}
            </p>
            <div className="record-meta">
              <span>
                <Clock3 size={15} aria-hidden="true" />
                {current.estimated_minutes
                  ? `${current.estimated_minutes} мин`
                  : "Без оценки времени"}
              </span>
              <span>
                <CalendarDays size={15} aria-hidden="true" />
                {current.planned_date ?? "Без плановой даты"}
              </span>
              <span>
                <Flag size={15} aria-hidden="true" />
                {{ low: "Низкий", normal: "Обычный", high: "Высокий" }[current.priority]} приоритет
              </span>
            </div>
          </Card>
          <QuickFocusStart key={current.id} quest={current} />
          <QuestActions
            quest={current}
            onChanged={() => {
              void query.refetch().then(() => setRevision((v) => v + 1));
            }}
          />
          <details className="section-disclosure">
            <summary>Изменить задачу</summary>
            <div className="disclosure-content">
              <QuestForm
                key={current.id + revision}
                current={current}
                labels={labels.data}
                onSaved={() => query.refetch()}
              />
            </div>
          </details>
          <details className="section-disclosure">
            <summary>История, связи и файлы</summary>
            <div className="disclosure-content">
              <QuestFocusTime questId={current.id} />
              <QuestHistory quest={current} />
              <QuestGoalLink id={current.id} />
              <QuestArchive
                quest={current}
                onChanged={() => {
                  void query.refetch().then(() => setRevision((v) => v + 1));
                }}
              />
              <QuestTemplates
                quest={current}
                onCreated={(q) => {
                  setCreated(q);
                  void query.refetch();
                }}
              />
              <QuestFiles
                quest={current}
                onChanged={() => {
                  void query.refetch().then(() => setRevision((v) => v + 1));
                }}
              />
              <QuestStructure
                quest={current}
                onChanged={() => {
                  void query.refetch().then(() => setRevision((v) => v + 1));
                }}
                onAddChild={() => {
                  if (confirmDiscard()) {
                    setNewParent(current.id);
                    setEditing(true);
                  }
                }}
              />
            </div>
          </details>
        </>
      )}
      {collection && (
        <>
          <ChoiceChips
            label="Быстрый фильтр задач"
            value={filters.status}
            options={[
              { value: "", label: "Все задачи" },
              ...["todo", "in_progress", "completed"].map((value) => ({
                value,
                label: statuses[value as keyof typeof statuses],
              })),
            ]}
            onChange={(status) => {
              if (confirmDiscard()) {
                setFilters({ ...filters, status });
                setPage(1);
              }
            }}
          />
          <details className="section-disclosure">
            <summary>Шаблоны задач</summary>
            <div className="disclosure-content">
              <QuestTemplates
                onCreated={(q) => {
                  setCreated(q);
                  void query.refetch();
                }}
              />
            </div>
          </details>
          <details className="section-disclosure">
            <summary>Поиск и фильтры</summary>
            <div className="disclosure-content">
              <QuestFiltersPanel
                key={JSON.stringify(filters)}
                filters={filters}
                labels={labels.data}
                onApply={(f) => {
                  setFilters(f);
                  setPage(1);
                }}
              />
            </div>
          </details>
          <details className="section-disclosure">
            <summary>Действия с несколькими задачами</summary>
            <div className="disclosure-content">
              <QuestBatchPanel items={collection.items} onChanged={() => void query.refetch()} />
            </div>
          </details>
          <div className="button-row">
            <Button
              variant={board ? "secondary" : "primary"}
              aria-pressed={!board}
              onClick={() => setBoard(false)}
            >
              Список
            </Button>
            <Button
              variant={board ? "primary" : "secondary"}
              aria-pressed={board}
              onClick={() => setBoard(true)}
            >
              Доска
            </Button>
          </div>
          <p className="ui-hint">Найдено: {collection.total}.</p>
          {!board && (
            <div className="quest-list">
              {collection.items.map((q) => (
                <Card
                  key={q.id}
                  className="task-record-card"
                  data-status={q.status}
                  data-priority={q.priority}
                >
                  <h2>
                    <SafeLink className="text-link" href={"/app/quests/" + q.id}>
                      {q.title}
                    </SafeLink>
                  </h2>
                  <div className="record-meta">
                    <span className="record-status">{statuses[q.status]}</span>
                    <span>Сложность {q.difficulty}/5</span>
                  </div>
                  <div className="record-meta">
                    <span>
                      <CalendarDays size={14} aria-hidden="true" />
                      {q.planned_date ?? "Без даты"}
                    </span>
                    <span>
                      <Clock3 size={14} aria-hidden="true" />
                      {q.estimated_minutes ? `${q.estimated_minutes} мин` : "Без оценки"}
                    </span>
                  </div>
                  {q.deadline && settings.data && (
                    <p>Дедлайн: {formatMoment(q.deadline, settings.data)}</p>
                  )}
                </Card>
              ))}
            </div>
          )}
          {board && (
            <>
              <p className="ui-hint">
                Доска показывает те же задачи текущей страницы. Откройте карточку для изменения
                статуса.
              </p>
              <div className="quest-kanban" aria-label="Доска задач" tabIndex={0}>
                {Object.entries(statuses).map(([status, label]) => (
                  <section className="quest-column" key={status} data-status={status}>
                    <h2>
                      {label} · {collection.items.filter((q) => q.status === status).length}
                    </h2>
                    {collection.items
                      .filter((q) => q.status === status)
                      .map((q) => (
                        <Card key={q.id}>
                          <h3>
                            <SafeLink className="text-link" href={"/app/quests/" + q.id}>
                              {q.title}
                            </SafeLink>
                          </h3>
                          <p>План: {q.planned_date ?? "не задан"}</p>
                        </Card>
                      ))}
                  </section>
                ))}
              </div>
            </>
          )}
          {collection.total === 0 && (
            <Card>
              <h2>
                {JSON.stringify(filters) === JSON.stringify(emptyFilters)
                  ? "Начните с одного шага"
                  : "По этим условиям задач нет"}
              </h2>
              <p>Запишите конкретное дело. Направление, описание и срок можно добавить позже.</p>
              <div className="button-row">
                <Button
                  onClick={() => {
                    if (confirmDiscard()) setEditing(true);
                  }}
                >
                  Добавить первую задачу
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => {
                    setFilters(emptyFilters);
                    setPage(1);
                  }}
                >
                  Сбросить фильтры
                </Button>
              </div>
            </Card>
          )}
          <div className="button-row">
            <Button variant="secondary" disabled={page === 1} onClick={() => setPage(page - 1)}>
              Назад
            </Button>
            <span>Страница {page}</span>
            <Button
              variant="secondary"
              disabled={page * collection.page_size >= collection.total}
              onClick={() => setPage(page + 1)}
            >
              Далее
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
const formSchema = questInput.extend({
  parent_id: z.string(),
  area_id: z.string(),
  deadline: z
    .string()
    .refine(
      (v) => v === "" || z.iso.datetime({ offset: true }).safeParse(v).success,
      "Время в ISO 8601 с часовым поясом, например 2026-10-12T18:00:00+05:00",
    ),
  planned_date: z
    .string()
    .refine((v) => v === "" || z.iso.date().safeParse(v).success, "Укажите дату"),
});
type Fields = z.infer<typeof formSchema>;
type Labels = Awaited<ReturnType<typeof questLabels>>;
function formValues(q: QuestInput): Fields {
  return {
    ...q,
    actual_time_mode:
      q.actual_time_mode ?? (q.actual_minutes > 0 ? "manual_override" : "from_focus"),
    area_id: q.area_id ?? "",
    parent_id: q.parent_id ?? "",
    deadline: q.deadline ?? "",
    planned_date: q.planned_date ?? "",
  };
}
function QuestForm({
  current,
  labels,
  onSaved,
  initialParent,
}: {
  current?: Quest;
  labels: Labels;
  onSaved: (q: Quest) => void | Promise<unknown>;
  initialParent?: string | null;
}) {
  const cache = useQueryClient(),
    settings = useSettings();
  const confirmDiscard = useConfirmDiscard();
  const [saved, setSaved] = useState(false);
  const [record, setRecord] = useState(current);
  const flow = useValidatedForm<Fields>({
    schema: formSchema,
    defaultValues: formValues(current ?? { ...emptyQuest, parent_id: initialParent ?? null }),
    fields: [
      "title",
      "description",
      "area_id",
      "priority",
      "difficulty",
      "deadline",
      "planned_date",
      "estimated_minutes",
      "actual_minutes",
      "actual_time_mode",
      "tags",
      "status",
      "parent_id",
      "required",
    ],
    save: async (values) => {
      setSaved(false);
      const q = await saveQuest(
        {
          ...values,
          area_id: values.area_id || null,
          parent_id: values.parent_id || null,
          deadline: values.deadline || null,
          planned_date: values.planned_date || null,
        },
        record,
      );
      if (current) setRecord(q);
      await onSaved(q);
      setSaved(true);
      void cache.invalidateQueries({ queryKey: ["account"] });
      return current ? formValues(q) : formValues(emptyQuest);
    },
  });
  const f = flow.form;
  const errors = f.formState.errors;
  return (
    <Card className="settings-card task-composer">
      <h2>{current ? "Редактирование задачи" : "Новая задача"}</h2>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Название задачи"
          placeholder="Какой следующий шаг?"
          maxLength={200}
          error={errors.title?.message}
          {...f.register("title")}
        />
        <div className="task-composer-essentials">
          <DatePicker
            label="Плановая дата"
            error={errors.planned_date?.message}
            {...f.register("planned_date")}
          />
          <Input
            label="Оценка, минут"
            type="number"
            min={0}
            max={525600}
            error={errors.estimated_minutes?.message}
            {...f.register("estimated_minutes", { valueAsNumber: true })}
          />
        </div>
        <div className="task-shortcuts">
          <ChoiceChips
            label="Когда заняться задачей"
            value={f.watch("planned_date")}
            options={[
              { value: "", label: "Без даты" },
              {
                value: DateTime.now()
                  .setZone(settings.data?.timezone ?? "UTC")
                  .toISODate()!,
                label: "Сегодня",
              },
              {
                value: DateTime.now()
                  .setZone(settings.data?.timezone ?? "UTC")
                  .plus({ days: 1 })
                  .toISODate()!,
                label: "Завтра",
              },
            ]}
            onChange={(value) =>
              f.setValue("planned_date", value, { shouldDirty: true, shouldValidate: true })
            }
          />
          <ChoiceChips
            label="Сколько времени займёт задача"
            value={f.watch("estimated_minutes")}
            options={[15, 25, 45, 60].map((value) => ({ value, label: `${value} мин` }))}
            onChange={(value) =>
              f.setValue("estimated_minutes", value, { shouldDirty: true, shouldValidate: true })
            }
          />
          <ChoiceChips
            label="Быстрый выбор приоритета"
            value={f.watch("priority")}
            options={[
              { value: "low", label: "Не спеша" },
              { value: "normal", label: "Обычный" },
              { value: "high", label: "Важное", icon: <Flag size={15} /> },
            ]}
            onChange={(value) => f.setValue("priority", value, { shouldDirty: true })}
          />
        </div>
        <details className="form-disclosure">
          <summary>Дополнительные параметры</summary>
          <div className="disclosure-content">
            <label className="ui-field">
              Описание
              <textarea
                className="ui-input"
                rows={4}
                maxLength={10000}
                {...f.register("description")}
              />
            </label>
            <Select label="Направление" error={errors.area_id?.message} {...f.register("area_id")}>
              <option value="">Без направления</option>
              {labels.areas
                .filter((a) => !a.archived || a.id === record?.area_id)
                .map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                    {a.archived ? " (архив)" : ""}
                  </option>
                ))}
            </Select>
            <QuestPicker
              label="Родительская задача"
              value={f.watch("parent_id")}
              onChange={(id) => f.setValue("parent_id", id, { shouldDirty: true })}
              exclude={current?.id}
            />
            {errors.parent_id && (
              <p role="alert" className="ui-error">
                {errors.parent_id.message}
              </p>
            )}
            <label className="check-row">
              <input type="checkbox" {...f.register("required")} />
              Обязательная подзадача
            </label>
            <div className="quest-grid">
              <Select label="Приоритет" {...f.register("priority")}>
                <option value="low">Низкий</option>
                <option value="normal">Обычный</option>
                <option value="high">Высокий</option>
              </Select>
              <Input
                label="Сложность (1–5)"
                type="number"
                min={1}
                max={5}
                error={errors.difficulty?.message}
                {...f.register("difficulty", { valueAsNumber: true })}
              />
            </div>
            <Select label="Статус" disabled={!!current} {...f.register("status")}>
              {Object.entries(statuses)
                .filter(([key]) => current || key === "backlog" || key === "todo")
                .map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
            </Select>
            <DeadlineField
              value={f.watch("deadline")}
              zone={settings.data?.timezone ?? "UTC"}
              error={errors.deadline?.message}
              onChange={(value) =>
                f.setValue("deadline", value, { shouldDirty: true, shouldValidate: true })
              }
            />
            <Select label="Источник фактического времени" {...f.register("actual_time_mode")}>
              <option value="from_focus">Из подтверждённых фокус-сессий</option>
              <option value="manual_override">Ручной итог вместо фокуса</option>
            </Select>
            <div className="quest-grid">
              <Input
                label="Фактически, минут"
                hint="Ручной ввод заменяет сумму фокуса. Переключитесь обратно на фокус, чтобы использовать подтверждённые сессии."
                type="number"
                min={0}
                max={525600}
                error={errors.actual_minutes?.message}
                {...f.register("actual_minutes", { valueAsNumber: true })}
                onChange={(e) => {
                  void f.register("actual_minutes", { valueAsNumber: true }).onChange(e);
                  f.setValue("actual_time_mode", "manual_override", { shouldDirty: true });
                }}
              />
            </div>
            <fieldset>
              <legend>Теги</legend>
              {labels.tags
                .filter((t) => !t.archived || record?.tags.includes(t.id))
                .map((t) => (
                  <label className="check-row" key={t.id}>
                    <input type="checkbox" value={t.id} {...f.register("tags")} />
                    {t.name}
                  </label>
                ))}
              {labels.tags.length === 0 && (
                <p className="ui-hint">Добавьте теги в настройках направлений.</p>
              )}
            </fieldset>
          </div>
        </details>
        <Button type="submit" disabled={!!record?.archived_at}>
          Сохранить задачу
        </Button>
      </Form>
      {saved && !flow.form.formState.isDirty && <p role="status">Сохранено.</p>}
      {current && flow.error && (
        <Button
          variant="secondary"
          onClick={async () => {
            if (!confirmDiscard()) return;
            const fresh = await getQuest(current.id);
            setRecord(fresh);
            f.reset(formValues(fresh));
          }}
        >
          Загрузить актуальную версию
        </Button>
      )}
    </Card>
  );
}
