"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import {
  emptyGoal,
  emptyGoalFilters,
  getGoal,
  listGoals,
  saveGoal,
  goalInput,
  goalStates,
  progressTypes,
  type Goal,
  type GoalInput,
  type GoalFilters,
} from "@/lib/goals";
import { questLabels } from "@/lib/quests";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select, DatePicker } from "./ui/field";
import { Form } from "./ui/form";
import { GoalPlanPanel } from "./goal-plan";
import { GoalMetrics } from "./goal-metrics";
import { GoalTemplates } from "./goal-templates";
import { GoalFirstAction } from "./goal-first-action";
import { GoalDecisions, GoalHistory } from "./goal-lifecycle";

export function Goals({ id }: { id?: string }) {
  const auth = useAuth();
  const account = auth.session?.account.id;
  const discard = useConfirmDiscard();
  const cache = useQueryClient();
  const [editingDetails, setEditingDetails] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState(emptyGoalFilters);
  const [editing, setEditing] = useState(false);
  const [created, setCreated] = useState<Goal>();
  const [revision, setRevision] = useState(0);
  const [notice, setNotice] = useState("");
  const query = useQuery<Goal | Awaited<ReturnType<typeof listGoals>>>({
    queryKey: ["account", account, "goals", id ?? "", page, filters],
    enabled: !!account,
    queryFn: ({ signal }) => (id ? getGoal(id, signal) : listGoals(page, filters, signal)),
  });
  const labels = useQuery({
    queryKey: ["account", account, "quest-labels"],
    enabled: !!account,
    queryFn: ({ signal }) => questLabels(signal),
  });
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!account)
    return (
      <p>
        Для работы с целями{" "}
        <SafeLink className="text-link" href="/auth/login?next=/app/goals">
          войдите в аккаунт
        </SafeLink>
        .
      </p>
    );
  if (query.isPending || labels.isPending) return <p role="status">Загружаем цели…</p>;
  if (!query.data || !labels.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить цели.</p>
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
  const changed = () => {
    setNotice("Изменения сохранены.");
    void query.refetch().then(() => setRevision((v) => v + 1));
    void cache.invalidateQueries({ queryKey: ["account"] });
  };
  return (
    <div className="quest-workspace">
      {notice && <p role="status">{notice}</p>}
      <div className="button-row">
        <Button
          onClick={() => {
            if (discard()) {
              setEditing(!editing);
              setCreated(undefined);
            }
          }}
        >
          {editing ? "Закрыть форму" : "Создать цель"}
        </Button>
        <SafeLink className="text-link" href="/app/quests">
          Задачи
        </SafeLink>
      </div>
      {created && (
        <p role="status">
          Цель сохранена:{" "}
          <SafeLink className="text-link" href={"/app/goals/" + created.id}>
            {created.title}
          </SafeLink>
        </p>
      )}
      {editing && (
        <GoalForm
          labels={labels.data}
          onSaved={(g) => {
            setCreated(g);
            void query.refetch();
          }}
        />
      )}
      {current && !editing && (
        <>
          <SafeLink className="text-link" href="/app/goals">
            Все цели
          </SafeLink>
          <Card>
            <h2>{current.title}</h2>
            <p>
              {goalStates[current.state]}
              {current.archived_at ? " · Архив" : ""}
              {current.overdue ? " · Срок прошёл" : ""}
            </p>
            <p className="quest-description">{current.motive || "Мотив пока не добавлен."}</p>
            <h3>Критерий успеха</h3>
            <p className="quest-description">{current.criteria}</p>
            <p>
              {progressTypes[current.progress_type]} · {Number(current.progress.toFixed(2))}%
            </p>
            <progress aria-label="Прогресс цели" value={current.progress} max={100} />
            {current.progress_type === "metric" && (
              <p>
                {current.current} / {current.target} {current.unit}
              </p>
            )}
            {current.overdue && (
              <p className="ui-hint">Срок прошёл. Цель остаётся открытой до вашего решения.</p>
            )}
          </Card>
          {!current.archived_at && ["draft", "active", "paused"].includes(current.state) && (
            <Button
              variant="secondary"
              onClick={() => {
                if (discard()) setEditingDetails(!editingDetails);
              }}
            >
              {editingDetails ? "Закрыть редактор цели" : "Изменить цель"}
            </Button>
          )}
          {editingDetails ? (
            <GoalForm
              key={current.id + revision}
              current={current}
              labels={labels.data}
              onSaved={changed}
            />
          ) : (
            <>
              <GoalDecisions key={current.id + revision} goal={current} onChanged={changed} />
              {current.progress_type === "metric" && (
                <GoalMetrics goal={current} onChanged={changed} />
              )}
              <GoalPlanPanel goal={current} onChanged={changed} />
              <GoalFirstAction key={current.id} goal={current} onChanged={changed} />
              <GoalHistory goal={current} />
            </>
          )}
        </>
      )}
      {collection && (
        <>
          {!editing && (
            <GoalTemplates
              onSaved={(g) => {
                setCreated(g);
                changed();
              }}
            />
          )}
          <GoalFilterPanel
            filters={filters}
            labels={labels.data}
            onApply={(f) => {
              setFilters(f);
              setPage(1);
            }}
          />
          <p>Найдено: {collection.total}</p>
          <div className="quest-grid">
            {collection.items.map((g) => (
              <Card key={g.id}>
                <h2>
                  <SafeLink className="text-link" href={"/app/goals/" + g.id}>
                    {g.title}
                  </SafeLink>
                </h2>
                <p>
                  {goalStates[g.state]}
                  {g.overdue ? " · Срок прошёл" : ""}
                </p>
                <p>
                  {progressTypes[g.progress_type]} · {Number(g.progress.toFixed(2))}%
                </p>
                <p className="quest-description">{g.criteria}</p>
              </Card>
            ))}
          </div>
          {collection.items.length === 0 && (
            <Card>
              <p>Здесь пока нет целей. Создайте свою или измените фильтры.</p>
            </Card>
          )}
          <div className="button-row">
            <Button variant="secondary" disabled={page === 1} onClick={() => setPage((v) => v - 1)}>
              Назад
            </Button>
            <span>Страница {page}</span>
            <Button
              variant="secondary"
              disabled={page * 25 >= collection.total}
              onClick={() => setPage((v) => v + 1)}
            >
              Далее
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
type Labels = Awaited<ReturnType<typeof questLabels>>;
function GoalFilterPanel({
  filters,
  labels,
  onApply,
}: {
  filters: GoalFilters;
  labels: Labels;
  onApply: (f: GoalFilters) => void;
}) {
  const [draft, setDraft] = useState(filters);
  return (
    <Card>
      <h2>Найти цель</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onApply(draft);
        }}
      >
        <div className="quest-grid">
          <Input
            label="Поиск целей"
            maxLength={200}
            value={draft.search}
            onChange={(e) => setDraft({ ...draft, search: e.target.value })}
          />
          <Select
            label="Состояние в списке"
            value={draft.state}
            onChange={(e) => setDraft({ ...draft, state: e.target.value })}
          >
            <option value="">Все состояния</option>
            {Object.entries(goalStates).map(([v, n]) => (
              <option key={v} value={v}>
                {n}
              </option>
            ))}
          </Select>
          <Select
            label="Направление целей"
            value={draft.area_id}
            onChange={(e) => setDraft({ ...draft, area_id: e.target.value })}
          >
            <option value="">Все направления</option>
            {labels.areas.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </Select>
        </div>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.archived}
            onChange={(e) => setDraft({ ...draft, archived: e.target.checked })}
          />
          Показать архив целей
        </label>
        <div className="button-row">
          <Button type="submit">Применить фильтры целей</Button>
          <Button
            variant="secondary"
            onClick={() => {
              setDraft(emptyGoalFilters);
              onApply(emptyGoalFilters);
            }}
          >
            Сбросить
          </Button>
        </div>
      </form>
    </Card>
  );
}
const fieldsSchema = goalInput.omit({ area_id: true, start_date: true, target_date: true }).extend({
  area_id: z.string(),
  start_date: z.string(),
  target_date: z.string(),
  reason: z.string().max(1000),
  state: z.enum(["draft", "active"]),
});
type Fields = z.infer<typeof fieldsSchema>;
function values(g: GoalInput, state: "draft" | "active" = "draft"): Fields {
  return {
    ...g,
    area_id: g.area_id ?? "",
    start_date: g.start_date ?? "",
    target_date: g.target_date ?? "",
    reason: "",
    state,
  };
}
function GoalForm({
  current,
  labels,
  onSaved,
}: {
  current?: Goal;
  labels: Labels;
  onSaved: (g: Goal) => void;
}) {
  const [record, setRecord] = useState(current);
  const [saved, setSaved] = useState(false);
  const [reloadError, setReloadError] = useState("");
  const cache = useQueryClient();
  const discard = useConfirmDiscard();
  const flow = useValidatedForm<Fields>({
    schema: fieldsSchema,
    defaultValues: values(current ?? emptyGoal, current?.state === "active" ? "active" : "draft"),
    fields: [
      "title",
      "motive",
      "criteria",
      "area_id",
      "start_date",
      "target_date",
      "progress_type",
      "start",
      "target",
      "unit",
      "reason",
      "state",
    ],
    save: async (v) => {
      setSaved(false);
      const { reason, state, ...input } = v;
      const g = await saveGoal(
        {
          ...input,
          area_id: v.area_id || null,
          start_date: v.start_date || null,
          target_date: v.target_date || null,
        },
        reason,
        state,
        record,
      );
      if (current) setRecord(g);
      onSaved(g);
      setSaved(true);
      void cache.invalidateQueries({ queryKey: ["account"] });
      return values(current ? g : emptyGoal, current?.state === "active" ? "active" : "draft");
    },
  });
  const f = flow.form;
  const e = f.formState.errors;
  const editable =
    !record || (!record.archived_at && ["draft", "active", "paused"].includes(record.state));
  return (
    <Card>
      <h2>{current ? "Редактирование цели" : "Новая цель"}</h2>
      {record && <Input label="Состояние цели" value={goalStates[record.state]} readOnly />}
      <Form saving={flow.saving || !editable} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Название цели"
          maxLength={200}
          error={e.title?.message}
          {...f.register("title")}
        />
        <label className="ui-field">
          Почему это важно
          <textarea className="ui-input" rows={3} maxLength={10000} {...f.register("motive")} />
        </label>
        <Input
          label="Критерий успеха"
          maxLength={2000}
          error={e.criteria?.message}
          {...f.register("criteria")}
        />
        <div className="quest-grid">
          <Select label="Направление цели" {...f.register("area_id")} error={e.area_id?.message}>
            <option value="">Без направления</option>
            {labels.areas
              .filter((a) => !a.archived || a.id === record?.area_id)
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
          </Select>
          <DatePicker
            label="Начальная дата цели"
            {...f.register("start_date")}
            error={e.start_date?.message}
          />
          <DatePicker
            label="Целевая дата"
            {...f.register("target_date")}
            error={e.target_date?.message}
          />
        </div>
        <Select
          label="Способ расчёта прогресса"
          value={f.watch("progress_type")}
          onChange={(e) => {
            const type = e.target.value as Fields["progress_type"];
            f.setValue("progress_type", type, { shouldDirty: true });
            f.setValue("start", 0, { shouldDirty: true });
            f.setValue("target", type === "metric" ? 10 : 1, { shouldDirty: true });
            f.setValue("unit", "", { shouldDirty: true });
          }}
        >
          {Object.entries(progressTypes).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Select>
        {f.watch("progress_type") === "metric" && (
          <div className="quest-grid">
            <Input
              label="Начальное значение"
              type="number"
              step="0.001"
              error={e.start?.message}
              {...f.register("start", { valueAsNumber: true })}
            />
            <Input
              label="Целевое значение"
              type="number"
              step="0.001"
              error={e.target?.message}
              {...f.register("target", { valueAsNumber: true })}
            />
            <Input
              label="Единица результата"
              maxLength={40}
              error={e.unit?.message}
              {...f.register("unit")}
            />
          </div>
        )}
        {!current && (
          <Select label="Начальное состояние цели" {...f.register("state")}>
            <option value="draft">Черновик</option>
            <option value="active">Активна</option>
          </Select>
        )}
        {record && record.state !== "draft" && (
          <Input
            label="Причина изменения плана"
            maxLength={1000}
            error={e.reason?.message}
            {...f.register("reason")}
          />
        )}
        <Button type="submit">Сохранить цель</Button>
      </Form>
      {saved && <p role="status">Цель сохранена.</p>}
      {current && (
        <Button
          variant="secondary"
          onClick={async () => {
            if (!discard()) return;
            try {
              const g = await getGoal(current.id);
              setRecord(g);
              f.reset(values(g));
              setReloadError("");
              onSaved(g);
            } catch {
              setReloadError("Не удалось обновить цель. Попробуйте снова.");
            }
          }}
        >
          Загрузить свежую версию цели
        </Button>
      )}
      {reloadError && <p role="alert">{reloadError}</p>}
    </Card>
  );
}
