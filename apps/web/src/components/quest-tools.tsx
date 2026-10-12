"use client";
import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { ApiError } from "@/lib/api/client";
import { useAuth } from "@/lib/auth/provider";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { statuses, type Quest } from "@/lib/quests";
import {
  getTemplates,
  saveTemplate,
  renameTemplate,
  deleteTemplate,
  previewTemplate,
  applyTemplate,
  questBatch,
  archiveQuest,
  type QuestBatch,
  type QuestTemplate,
} from "@/lib/quest-tools";
import { Card } from "./ui/card";
import { Input, Select, DatePicker } from "./ui/field";
import { Button } from "./ui/button";
import { Form } from "./ui/form";
function message(e: unknown) {
  return e instanceof ApiError
    ? Object.values(e.fields).join(". ") || e.message
    : "Не удалось выполнить действие. Попробуйте снова.";
}
export function QuestTemplates({
  quest,
  onCreated,
}: {
  quest?: Quest;
  onCreated: (q: Quest) => void;
}) {
  const auth = useAuth();
  const discard = useConfirmDiscard();
  const list = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-templates"],
    queryFn: ({ signal }) => getTemplates(signal),
  });
  const [selected, setSelected] = useState("");
  const [date, setDate] = useState("");
  const [deadline, setDeadline] = useState("");
  const [preview, setPreview] = useState<Awaited<ReturnType<typeof previewTemplate>>>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const retry = useRef("");
  const current = list.data?.find((t) => t.id === selected);
  return (
    <Card>
      <h2>Шаблоны задач</h2>
      {quest && <SaveTemplate quest={quest} done={() => void list.refetch()} />}
      <p className="ui-hint">
        Шаблон сохраняет описание, направление, теги и оценку. Новая задача начинается в идеях, без
        фактического времени и прежних результатов.
      </p>
      {list.isError && (
        <p role="alert">
          Шаблоны не загрузились. <Button onClick={() => void list.refetch()}>Повторить</Button>
        </p>
      )}
      <Select
        label="Выберите шаблон"
        value={selected}
        onChange={(e) => {
          setSelected(e.target.value);
          setPreview(undefined);
        }}
      >
        <option value="">Не выбрано</option>
        {list.data?.map((t) => (
          <option key={t.id} value={t.id}>
            {t.name}
          </option>
        ))}
      </Select>
      {current && (
        <>
          <RenameTemplate
            key={current.id + current.version}
            template={current}
            done={() => {
              setPreview(undefined);
              void list.refetch();
            }}
          />
          <DatePicker
            label="Плановая дата из шаблона"
            value={date}
            onChange={(e) => {
              setDate(e.target.value);
              setPreview(undefined);
            }}
          />
          <Input
            label="Дедлайн из шаблона с часовым поясом"
            value={deadline}
            onChange={(e) => {
              setDeadline(e.target.value);
              setPreview(undefined);
            }}
          />
          <div className="button-row">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  setPreview(
                    await previewTemplate(selected, {
                      planned_date: date || null,
                      deadline: deadline || null,
                    }),
                  );
                  retry.current = crypto.randomUUID();
                } catch (e) {
                  setError(message(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Предпросмотр задачи
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              onClick={async () => {
                if (
                  !discard() ||
                  !window.confirm("Удалить шаблон? Созданные из него задачи сохранятся.")
                )
                  return;
                setBusy(true);
                try {
                  await deleteTemplate(current);
                  setSelected("");
                  setPreview(undefined);
                  await list.refetch();
                } catch (e) {
                  setError(message(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Удалить шаблон
            </Button>
          </div>
        </>
      )}
      {preview && (
        <div className="quest-preview">
          <h3>Будет создана одна задача</h3>
          <p>
            {preview.input.title} · {preview.input.estimated_minutes} мин. · Сложность{" "}
            {preview.input.difficulty}/5
          </p>
          <p>План: {preview.input.planned_date ?? "не задан"}</p>
          <Button
            disabled={busy}
            onClick={async () => {
              if (!discard()) return;
              setBusy(true);
              setError("");
              try {
                const q = await applyTemplate(
                  selected,
                  { planned_date: date || null, deadline: deadline || null },
                  preview.version,
                  retry.current,
                );
                onCreated(q);
                setPreview(undefined);
              } catch (e) {
                setError(message(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            Создать из шаблона
          </Button>
        </div>
      )}
      {error && (
        <p role="alert" className="ui-error">
          {error}
        </p>
      )}
    </Card>
  );
}
function SaveTemplate({ quest, done }: { quest: Quest; done: () => void }) {
  const [saved, setSaved] = useState(false);
  const flow = useValidatedForm({
    schema: z.object({ name: z.string().trim().min(1).max(80) }),
    defaultValues: { name: "" },
    fields: ["name"],
    save: async (v) => {
      await saveTemplate(v.name, quest);
      setSaved(true);
      done();
      return { name: "" };
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Название нового шаблона"
        error={flow.form.formState.errors.name?.message}
        {...flow.form.register("name")}
      />
      <Button type="submit" variant="secondary">
        Сохранить как шаблон
      </Button>
      {saved && <p role="status">Шаблон сохранён.</p>}
    </Form>
  );
}
function RenameTemplate({ template, done }: { template: QuestTemplate; done: () => void }) {
  const flow = useValidatedForm({
    schema: z.object({ name: z.string().trim().min(1).max(80) }),
    defaultValues: { name: template.name },
    fields: ["name"],
    save: async (v) => {
      await renameTemplate(template, v.name);
      done();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input label="Название выбранного шаблона" {...flow.form.register("name")} />
      <Button type="submit" variant="ghost">
        Переименовать шаблон
      </Button>
    </Form>
  );
}
export function QuestArchive({ quest, onChanged }: { quest: Quest; onChanged: () => void }) {
  const discard = useConfirmDiscard();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const key = useRef("");
  return (
    <Card>
      <h2>Архив</h2>
      <p>Архив сохраняет результат и историю. Подзадачи архивируются отдельно.</p>
      <Button
        disabled={busy}
        variant="secondary"
        onClick={async () => {
          if (busy || !discard()) return;
          setBusy(true);
          setError("");
          key.current ||= crypto.randomUUID();
          try {
            await archiveQuest(quest, key.current);
            key.current = "";
            onChanged();
          } catch (e) {
            setError(message(e));
          } finally {
            setBusy(false);
          }
        }}
      >
        {quest.archived_at ? "Восстановить задачу" : "Архивировать задачу"}
      </Button>
      {error && <p role="alert">{error}</p>}
    </Card>
  );
}
export function QuestBatchPanel({ items, onChanged }: { items: Quest[]; onChanged: () => void }) {
  const [selected, setSelected] = useState<QuestBatch["items"]>([]);
  const [action, setAction] = useState<QuestBatch["action"]>("archive");
  const [reason, setReason] = useState("");
  const [date, setDate] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [preview, setPreview] = useState<{ body: QuestBatch; quests: Quest[]; key: string }>();
  return (
    <Card>
      <h2>Изменить несколько задач</h2>
      <p className="ui-hint">
        Выберите задачи текущей страницы. Перед применением покажем результат; изменения выполняются
        целиком.
      </p>
      <ul className="quest-batch">
        {items.map((q) => (
          <li key={q.id}>
            <label className="check-row">
              <input
                type="checkbox"
                checked={selected.some((i) => i.id === q.id)}
                onChange={(e) => {
                  setPreview(undefined);
                  setSelected((v) =>
                    e.target.checked
                      ? [...v, { id: q.id, version: q.version }]
                      : v.filter((i) => i.id !== q.id),
                  );
                }}
              />
              {"Выбрать: " + q.title}
            </label>
          </li>
        ))}
      </ul>
      <Select
        label="Пакетное действие"
        value={action}
        onChange={(e) => {
          setAction(e.target.value as QuestBatch["action"]);
          setPreview(undefined);
        }}
      >
        {Object.entries({
          archive: "В архив",
          restore: "Восстановить",
          start: "Начать",
          ready: "К выполнению",
          complete: "Завершить",
          reopen: "Открыть снова",
          postpone: "Перенести",
          skip: "Пропустить",
          fail: "Не получилось",
        }).map(([a, label]) => (
          <option key={a} value={a}>
            {label}
          </option>
        ))}
      </Select>
      {["postpone", "skip", "fail"].includes(action) && (
        <Input
          label="Причина пакетного действия"
          value={reason}
          maxLength={1000}
          onChange={(e) => {
            setReason(e.target.value);
            setPreview(undefined);
          }}
        />
      )}
      {action === "postpone" && (
        <DatePicker
          label="Новая дата для выбранных задач"
          value={date}
          onChange={(e) => {
            setDate(e.target.value);
            setPreview(undefined);
          }}
        />
      )}
      <Button
        variant="secondary"
        disabled={busy || selected.length === 0}
        onClick={async () => {
          setBusy(true);
          setError("");
          const body: QuestBatch = {
            items: selected,
            action,
            transition: {
              ...(["postpone", "skip", "fail"].includes(action) ? { reason } : {}),
              ...(action === "postpone" ? { planned_date: date || null } : {}),
            },
          };
          try {
            const quests = await questBatch(body, true, "");
            setPreview({ body, quests, key: crypto.randomUUID() });
          } catch (e) {
            setError(message(e));
          } finally {
            setBusy(false);
          }
        }}
      >
        Предпросмотр изменений ({selected.length})
      </Button>
      {preview && (
        <div className="quest-preview">
          <h3>Изменится задач: {preview.quests.length}</h3>
          <ul>
            {preview.quests.map((q) => (
              <li key={q.id}>
                <SafeLink className="text-link" href={"/app/quests/" + q.id}>
                  {q.title}
                </SafeLink>
                {" → " + statuses[q.status] + (q.archived_at ? " · в архиве" : "")}
              </li>
            ))}
          </ul>
          <Button
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                await questBatch(preview.body, false, preview.key);
                setPreview(undefined);
                setSelected([]);
                onChanged();
              } catch (e) {
                setError(message(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            Применить ко всем выбранным
          </Button>
        </div>
      )}
      {error && (
        <p role="alert" className="ui-error">
          {error}
        </p>
      )}
    </Card>
  );
}
