"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { emptyFilters, listQuests, getQuest, statuses, type Quest } from "@/lib/quests";
import { questStructure, saveDependencies } from "@/lib/quest-structure";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { Circle, CircleCheck, Clock3 } from "lucide-react";
export function QuestPicker({
  label,
  value,
  onChange,
  exclude,
}: {
  label: string;
  value: string;
  onChange: (id: string) => void;
  exclude?: string;
}) {
  const auth = useAuth();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const options = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-picker", search, page],
    queryFn: ({ signal }) => listQuests(page, signal, { ...emptyFilters, search, sort: "title" }),
  });
  return (
    <div className="quest-picker">
      <div className="quest-pick-cards" role="group" aria-label={"Быстрый выбор: " + label}>
        {options.data?.items
          .filter((q) => q.id !== exclude)
          .slice(0, 6)
          .map((q) => (
            <button
              type="button"
              key={q.id}
              aria-pressed={value === q.id}
              onClick={() => onChange(value === q.id ? "" : q.id)}
            >
              {value === q.id ? (
                <CircleCheck size={21} aria-hidden="true" />
              ) : (
                <Circle size={21} aria-hidden="true" />
              )}
              <span>
                <strong>{q.title}</strong>
                <small>
                  <Clock3 size={12} aria-hidden="true" />
                  {q.estimated_minutes ? `${q.estimated_minutes} мин` : "В своём темпе"}
                </small>
              </span>
            </button>
          ))}
      </div>
      <div className="picker-fields">
        <Input
          label={"Поиск: " + label}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(1);
          }}
        />
        <Select label={label} value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">Не выбрано</option>
          {value && !options.data?.items.some((q) => q.id === value) && (
            <option value={value}>Текущая задача</option>
          )}
          {options.data?.items
            .filter((q) => q.id !== exclude)
            .map((q) => (
              <option key={q.id} value={q.id}>
                {q.title}
              </option>
            ))}
        </Select>
      </div>
      {options.isError && (
        <p role="alert">
          Не удалось загрузить варианты.{" "}
          <Button onClick={() => void options.refetch()}>Повторить</Button>
        </p>
      )}
      <div className="button-row picker-pagination">
        <Button
          variant="ghost"
          aria-label={"Предыдущие варианты: " + label}
          disabled={page === 1}
          onClick={() => setPage((p) => p - 1)}
        >
          Назад
        </Button>
        <span>Варианты, стр. {page}</span>
        <Button
          variant="ghost"
          aria-label={"Следующие варианты: " + label}
          disabled={!options.data || page * 25 >= options.data.total}
          onClick={() => setPage((p) => p + 1)}
        >
          Далее
        </Button>
      </div>
    </div>
  );
}
export function QuestStructure({
  quest,
  onChanged,
  onAddChild,
}: {
  quest: Quest;
  onChanged: () => void;
  onAddChild: () => void;
}) {
  const auth = useAuth();
  const data = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-structure", quest.id, quest.version],
    queryFn: ({ signal }) => questStructure(quest.id, signal),
  });
  return (
    <Card>
      <h2>Подзадачи и блокировки</h2>
      {quest.parent_id && (
        <p>
          <SafeLink className="text-link" href={"/app/quests/" + quest.parent_id}>
            Родительская задача
          </SafeLink>
        </p>
      )}
      {data.isPending && <p role="status">Загружаем структуру…</p>}
      {data.isError && (
        <p role="alert">
          Не удалось загрузить структуру.{" "}
          <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      {data.data && (
        <>
          {data.data.container && (
            <p>
              Обязательные шаги: {data.data.completed_leaves} из {data.data.required_leaves}.
              Родитель учитывает их результат; закрытие родителя не закрывает детей.
            </p>
          )}
          <ul>
            {data.data.children.map((q) => (
              <li key={q.id}>
                <SafeLink className="text-link" href={"/app/quests/" + q.id}>
                  {q.title}
                </SafeLink>{" "}
                · {statuses[q.status]}
                {q.required ? "" : " · необязательная"}
              </li>
            ))}
          </ul>
          <Button
            variant="secondary"
            disabled={!!quest.archived_at || quest.status === "completed"}
            onClick={onAddChild}
          >
            Добавить подзадачу
          </Button>
          <Dependencies
            key={quest.id + quest.version}
            quest={quest}
            initial={data.data.dependencies}
            onChanged={onChanged}
          />
        </>
      )}
    </Card>
  );
}
const schema = z.object({ ids: z.array(z.uuid()).max(50), reason: z.string().max(1000) });
function Dependencies({
  quest,
  initial,
  onChanged,
}: {
  quest: Quest;
  initial: Quest[];
  onChanged: () => void;
}) {
  const [picked, setPicked] = useState("");
  const [known, setKnown] = useState(initial);
  const [lookupError, setLookupError] = useState("");
  const [lookupBusy, setLookupBusy] = useState(false);
  const flow = useValidatedForm({
    schema,
    defaultValues: { ids: initial.map((q) => q.id), reason: "" },
    fields: ["ids", "reason"],
    save: async (values) => {
      await saveDependencies(quest, values.ids, values.reason);
      onChanged();
    },
  });
  const selected = flow.form.watch("ids");
  return (
    <>
      <h3>Что нужно завершить сначала</h3>
      <p className="ui-hint">
        Пропуск блокирующей задачи не снимает блокировку. Для её удаления укажите причину.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <QuestPicker
          label="Блокирующая задача"
          value={picked}
          onChange={setPicked}
          exclude={quest.id}
        />
        <Button
          variant="secondary"
          disabled={lookupBusy || !picked || selected.includes(picked) || selected.length >= 50}
          onClick={async () => {
            setLookupBusy(true);
            setLookupError("");
            try {
              const value = await getQuest(picked);
              setKnown((v) => [...v.filter((x) => x.id !== value.id), value]);
              flow.form.setValue("ids", [...selected, picked], { shouldDirty: true });
              setPicked("");
            } catch {
              setLookupError("Не удалось загрузить задачу. Попробуйте снова.");
            } finally {
              setLookupBusy(false);
            }
          }}
        >
          Добавить блокировку
        </Button>
        {lookupError && <p role="alert">{lookupError}</p>}
        <ul>
          {selected.map((id) => (
            <li key={id}>
              {known.find((q) => q.id === id)?.title ?? "Задача"}{" "}
              <Button
                variant="ghost"
                aria-label={
                  "Снять блокировку " + (known.find((q) => q.id === id)?.title ?? "задачи")
                }
                onClick={() =>
                  flow.form.setValue(
                    "ids",
                    selected.filter((v) => v !== id),
                    { shouldDirty: true },
                  )
                }
              >
                Убрать
              </Button>
            </li>
          ))}
        </ul>
        <Input
          label="Причина изменения блокировок"
          error={flow.form.formState.errors.reason?.message}
          {...flow.form.register("reason")}
        />
        <Button type="submit" disabled={!!quest.archived_at}>
          Сохранить блокировки
        </Button>
      </Form>
    </>
  );
}
