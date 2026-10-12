"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import {
  emptyFilters,
  getQuestViews,
  saveQuestView,
  deleteQuestView,
  statuses,
  type QuestFilters,
  questLabels,
} from "@/lib/quests";
import { Input, Select, DatePicker } from "./ui/field";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { listGoals } from "@/lib/goals";
export function QuestFiltersPanel({
  filters,
  labels,
  onApply,
}: {
  filters: QuestFilters;
  labels: Awaited<ReturnType<typeof questLabels>>;
  onApply: (f: QuestFilters) => void;
}) {
  const auth = useAuth();
  const [draft, setDraft] = useState(filters);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const views = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-views"],
    queryFn: ({ signal }) => getQuestViews(signal),
  });
  function field<K extends keyof QuestFilters>(key: K, value: QuestFilters[K]) {
    setDraft((d) => ({ ...d, [key]: value }));
  }
  return (
    <Card>
      <h2>Найти нужное</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onApply(draft);
        }}
      >
        <div className="quest-grid">
          <Input
            label="Поиск задач"
            value={draft.search}
            maxLength={200}
            onChange={(e) => field("search", e.target.value)}
          />
          <Select
            label="Фильтр по статусу"
            value={draft.status}
            onChange={(e) => field("status", e.target.value)}
          >
            <option value="">Все статусы</option>
            {Object.entries(statuses).map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </Select>
          <Select
            label="Фильтр по направлению"
            value={draft.area_id}
            onChange={(e) => field("area_id", e.target.value)}
          >
            <option value="">Все направления</option>
            {labels.areas.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </Select>
          <Select
            label="Фильтр по цели"
            value={draft.goal_id}
            onChange={(e) => field("goal_id", e.target.value)}
          >
            <option value="">Все цели</option>
            {draft.goal_id && <option value={draft.goal_id}>Выбранная цель</option>}
          </Select>
          <GoalChoice onChoose={(id) => field("goal_id", id)} />
          <Select
            label="Фильтр по приоритету"
            value={draft.priority}
            onChange={(e) => field("priority", e.target.value)}
          >
            <option value="">Все приоритеты</option>
            <option value="high">Высокий</option>
            <option value="normal">Обычный</option>
            <option value="low">Низкий</option>
          </Select>
          <DatePicker
            label="План с"
            value={draft.planned_from}
            onChange={(e) => field("planned_from", e.target.value)}
          />
          <DatePicker
            label="План до"
            value={draft.planned_to}
            onChange={(e) => field("planned_to", e.target.value)}
          />
          <Select
            label="Сортировка задач"
            value={draft.sort}
            onChange={(e) => field("sort", e.target.value as QuestFilters["sort"])}
          >
            <option value="newest">Сначала новые</option>
            <option value="oldest">Сначала старые</option>
            <option value="title">По названию</option>
            <option value="planned">По плановой дате</option>
            <option value="priority">По приоритету</option>
          </Select>
        </div>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.overdue}
            onChange={(e) => field("overdue", e.target.checked)}
          />
          Только просроченные
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.archived}
            onChange={(e) => field("archived", e.target.checked)}
          />
          Показать архив
        </label>
        <div className="button-row">
          <Button type="submit">Применить фильтры</Button>
          <Button
            variant="secondary"
            onClick={() => {
              setDraft(emptyFilters);
              onApply(emptyFilters);
            }}
          >
            Сбросить
          </Button>
        </div>
      </form>
      <hr />
      <h3>Сохранённые представления</h3>
      <Input
        label="Название представления"
        maxLength={60}
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <Button
        variant="secondary"
        disabled={busy || !name.trim()}
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            await saveQuestView(name, draft);
            setName("");
            await views.refetch();
          } catch (e) {
            setError(e instanceof ApiError ? e.message : "Не удалось сохранить представление");
          } finally {
            setBusy(false);
          }
        }}
      >
        Сохранить представление
      </Button>
      {error && <p role="alert">{error}</p>}
      {views.isError && (
        <p role="alert">
          Представления не загрузились.{" "}
          <Button onClick={() => void views.refetch()}>Повторить</Button>
        </p>
      )}
      <ul className="quest-views">
        {views.data?.map((v) => (
          <li key={v.id}>
            <Button
              variant="ghost"
              onClick={() => {
                setDraft(v.filters);
                onApply(v.filters);
              }}
            >
              {v.name}
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              aria-label={"Удалить представление " + v.name}
              onClick={async () => {
                setBusy(true);
                try {
                  await deleteQuestView(v);
                  await views.refetch();
                } catch {
                  setError("Не удалось удалить представление");
                } finally {
                  setBusy(false);
                }
              }}
            >
              Удалить
            </Button>
          </li>
        ))}
      </ul>
    </Card>
  );
}

function GoalChoice({ onChoose }: { onChoose: (id: string) => void }) {
  const account = useAuth().session?.account.id;
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const goals = useQuery({
    queryKey: ["account", account, "quest-goal-choices", search, page],
    queryFn: ({ signal }) =>
      listGoals(page, { search, state: "", area_id: "", archived: false }, signal),
  });
  return (
    <div>
      <Input
        label="Найти цель для фильтра"
        value={search}
        maxLength={200}
        onChange={(e) => {
          setSearch(e.target.value);
          setPage(1);
        }}
      />
      {goals.data ? (
        <>
          <Select label="Выбрать цель" value="" onChange={(e) => onChoose(e.target.value)}>
            <option value="">Выберите из найденных</option>
            {goals.data.items.map((g) => (
              <option key={g.id} value={g.id}>
                {g.title}
              </option>
            ))}
          </Select>
          <div className="button-row">
            <Button disabled={page === 1} onClick={() => setPage((p) => p - 1)}>
              Цели: назад
            </Button>
            <Button
              disabled={page * goals.data.page_size >= goals.data.total}
              onClick={() => setPage((p) => p + 1)}
            >
              Цели: далее
            </Button>
          </div>
        </>
      ) : (
        <p role={goals.isPending ? "status" : "alert"}>
          {goals.isPending ? "Загружаем цели…" : "Цели не загрузились."}
        </p>
      )}
    </div>
  );
}
