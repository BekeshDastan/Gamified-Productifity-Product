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
import { useSettings, formatMoment } from "@/lib/settings";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
import { QuestFiltersPanel } from "./quest-filters";
import { QuickFocusStart } from "./quick-focus";
import { QuestActions, QuestHistory } from "./quest-actions";
import { QuestStructure, QuestPicker } from "./quest-structure";
import { QuestFiles } from "./quest-files";
import { QuestTemplates, QuestArchive, QuestBatchPanel } from "./quest-tools";
import { QuestGoalLink } from "./goal-plan";

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
  if (query.isPending || labels.isPending) return <p role="status">Загружаем задачи…</p>;
  if (!query.data || !labels.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить задачи.</p>
        <Button
          onClick={() => {
            void query.refetch();
            void labels.refetch();
          }}
        >
          Повторить
        </Button>
      </Card>
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
          onSaved={(q) => {
            setCreated(q);
            void query.refetch();
          }}
        />
      )}
      {current && (
        <>
          <SafeLink className="text-link" href="/app/quests">
            Все задачи
          </SafeLink>
          <Card>
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
          </Card>
          <QuestForm
            key={current.id + revision}
            current={current}
            labels={labels.data}
            onSaved={() => void query.refetch()}
          />
          <QuickFocusStart key={current.id} quest={current} />
          <QuestActions
            quest={current}
            onChanged={() => {
              void query.refetch().then(() => setRevision((v) => v + 1));
            }}
          />
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
        </>
      )}
      {collection && (
        <>
          <QuestTemplates
            onCreated={(q) => {
              setCreated(q);
              void query.refetch();
            }}
          />
          <QuestFiltersPanel
            filters={filters}
            labels={labels.data}
            onApply={(f) => {
              setFilters(f);
              setPage(1);
            }}
          />
          <QuestBatchPanel items={collection.items} onChanged={() => void query.refetch()} />
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
                <Card key={q.id}>
                  <h2>
                    <SafeLink className="text-link" href={"/app/quests/" + q.id}>
                      {q.title}
                    </SafeLink>
                  </h2>
                  <p>
                    {statuses[q.status]} · Сложность {q.difficulty}/5
                  </p>
                  <p>
                    План: {q.planned_date ?? "не задан"} · Оценка: {q.estimated_minutes} мин.
                  </p>
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
                  <section className="quest-column" key={status}>
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
              <h2>Начните с одного шага</h2>
              <p>Создайте задачу, выберите направление и подходящий срок.</p>
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
  onSaved: (q: Quest) => void;
  initialParent?: string | null;
}) {
  const cache = useQueryClient();
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
      onSaved(q);
      setSaved(true);
      void cache.invalidateQueries({ queryKey: ["account"] });
      return current ? formValues(q) : formValues(emptyQuest);
    },
  });
  const f = flow.form;
  const errors = f.formState.errors;
  return (
    <Card className="settings-card">
      <h2>{current ? "Редактирование задачи" : "Новая задача"}</h2>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Название задачи"
          maxLength={200}
          error={errors.title?.message}
          {...f.register("title")}
        />
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
        <DatePicker
          label="Плановая дата"
          error={errors.planned_date?.message}
          {...f.register("planned_date")}
        />
        <Input
          label="Дедлайн с часовым поясом"
          placeholder="2026-10-12T18:00:00+05:00"
          hint="Необязательно. Укажите время и смещение UTC. Плановая дата хранится отдельно."
          error={errors.deadline?.message}
          {...f.register("deadline")}
        />
        <div className="quest-grid">
          <Input
            label="Оценка, минут"
            type="number"
            min={0}
            max={525600}
            error={errors.estimated_minutes?.message}
            {...f.register("estimated_minutes", { valueAsNumber: true })}
          />
          <Input
            label="Фактически, минут"
            type="number"
            min={0}
            max={525600}
            error={errors.actual_minutes?.message}
            {...f.register("actual_minutes", { valueAsNumber: true })}
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
        <Button type="submit" disabled={!!record?.archived_at}>
          Сохранить задачу
        </Button>
      </Form>
      {saved && <p role="status">Сохранено.</p>}
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
