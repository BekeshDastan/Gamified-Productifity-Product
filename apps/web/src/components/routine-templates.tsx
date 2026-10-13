"use client";
import { useRef, useState } from "react";
import { z } from "zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import {
  getRoutineTemplate,
  saveRoutineTemplate,
  getRoutineHistory,
  type RoutineTemplate,
} from "@/lib/journal";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
export function RoutineChecklist({
  template,
  checked,
  onChange,
}: {
  template: RoutineTemplate;
  checked: string[];
  onChange: (v: string[]) => void;
}) {
  return (
    <fieldset>
      <legend>Мой чек-лист {template.kind === "morning" ? "утра" : "вечера"}</legend>
      {template.steps.map((step) => (
        <label className="check-row" key={step.id}>
          <input
            type="checkbox"
            checked={checked.includes(step.id)}
            onChange={(e) =>
              onChange(
                e.target.checked ? [...checked, step.id] : checked.filter((id) => id !== step.id),
              )
            }
          />
          {step.label}
        </label>
      ))}
      {template.steps.length === 0 && <p>В вашем шаблоне нет шагов.</p>}
    </fieldset>
  );
}
export function RoutineTemplates() {
  const auth = useAuth();
  return auth.session ? <TemplatesOwner account={auth.session.account.id} /> : null;
}
function TemplatesOwner({ account }: { account: string }) {
  return (
    <div className="journal-layout">
      <Card>
        <h2>Мои ритуалы</h2>
        <p>
          Порядок шагов можно менять. Сохранённые разборы и история используют прежний снимок
          чек-листа.
        </p>
      </Card>
      {(["morning", "evening"] as const).map((kind) => (
        <TemplateSection key={kind} account={account} kind={kind} />
      ))}
      <RoutineHistory account={account} />
    </div>
  );
}
function TemplateSection({ account, kind }: { account: string; kind: "morning" | "evening" }) {
  const q = useQuery({
    queryKey: accountQueryKey(account, "routine-template", kind),
    queryFn: ({ signal }) => getRoutineTemplate(kind, signal),
  });
  return q.data ? (
    <TemplateEditor key={kind + q.data.version} account={account} template={q.data} />
  ) : (
    <p role={q.isError ? "alert" : "status"}>
      {q.isError ? "Не удалось загрузить шаблон" : "Загружаем шаблон…"}
      <Button onClick={() => void q.refetch()}>Повторить</Button>
    </p>
  );
}
function TemplateEditor({ account, template }: { account: string; template: RoutineTemplate }) {
  const cache = useQueryClient();
  const [steps, setSteps] = useState(template.steps),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const operation = useRef({ body: "", key: "" });
  useUnsavedChanges(JSON.stringify(steps) !== JSON.stringify(template.steps) || busy);
  function move(index: number, offset: number) {
    const copy = [...steps],
      a = copy[index],
      b = copy[index + offset];
    if (!a || !b) return;
    copy[index] = b;
    copy[index + offset] = a;
    setSteps(copy);
  }
  return (
    <Card>
      <h2>
        {template.kind === "morning" ? "Шаблон утреннего ритуала" : "Шаблон вечернего ритуала"}
      </h2>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      <fieldset disabled={busy}>
        {steps.map((step, i) => (
          <div key={step.id}>
            <label className="ui-field">
              Шаг {i + 1} {template.kind === "morning" ? "утра" : "вечера"}
              <input
                className="ui-input"
                maxLength={200}
                value={step.label}
                onChange={(e) =>
                  setSteps((old) =>
                    old.map((v) => (v.id === step.id ? { ...v, label: e.target.value } : v)),
                  )
                }
              />
            </label>
            <div className="form-actions">
              <Button variant="secondary" disabled={i === 0} onClick={() => move(i, -1)}>
                Шаг {i + 1} выше
              </Button>
              <Button
                variant="secondary"
                disabled={i === steps.length - 1}
                onClick={() => move(i, 1)}
              >
                Шаг {i + 1} ниже
              </Button>
              <Button
                variant="secondary"
                onClick={() => setSteps((old) => old.filter((v) => v.id !== step.id))}
              >
                Удалить шаг {i + 1}
              </Button>
            </div>
          </div>
        ))}
        <Button
          variant="secondary"
          disabled={steps.length >= 20}
          onClick={() =>
            setSteps((old) => [...old, { id: crypto.randomUUID(), label: "Новый шаг" }])
          }
        >
          Добавить шаг {template.kind === "morning" ? "утра" : "вечера"}
        </Button>
        <Button
          onClick={async () => {
            setBusy(true);
            setError("");
            const body = JSON.stringify(steps);
            if (operation.current.body !== body)
              operation.current = { body, key: crypto.randomUUID() };
            try {
              await saveRoutineTemplate(template, steps, operation.current.key);
              setNotice("Чек-лист сохранён.");
              void cache.invalidateQueries({
                queryKey: accountQueryKey(account, "routine-template", template.kind),
              });
            } catch {
              setError(
                "Не удалось сохранить чек-лист. Проверьте названия и свежую версию; изменения остались в форме.",
              );
            } finally {
              setBusy(false);
            }
          }}
        >
          Сохранить шаблон {template.kind === "morning" ? "утра" : "вечера"}
        </Button>
      </fieldset>
    </Card>
  );
}
function RoutineHistory({ account }: { account: string }) {
  const [before, setBefore] = useState(""),
    [pages, setPages] = useState<string[]>([]);
  const q = useQuery({
    queryKey: accountQueryKey(account, "routine-history", before),
    queryFn: ({ signal }) => getRoutineHistory(before, signal),
  });
  return (
    <Card>
      <h2>История ритуалов</h2>
      {q.isError && (
        <p role="alert">
          Не удалось загрузить историю. <Button onClick={() => void q.refetch()}>Повторить</Button>
        </p>
      )}
      {q.data?.items.length === 0 && <p>Запусков пока нет.</p>}
      {q.data?.items.map((event) => {
        const snapshot = event.snapshot;
        const t = z
          .object({ steps: z.array(z.object({ id: z.string(), label: z.string() })) })
          .safeParse(snapshot.payload.template);
        return (
          <details key={event.id}>
            <summary>
              {snapshot.date} · {snapshot.kind === "morning" ? "Утро" : "Вечер"} · версия{" "}
              {snapshot.version}
            </summary>
            <p>Сохранено: {new Date(snapshot.updated_at).toLocaleString("ru-RU")}</p>
            {t.success ? (
              <ol>
                {t.data.steps.map((step) => (
                  <li key={step.id}>{step.label}</li>
                ))}
              </ol>
            ) : (
              <p>Этот запуск сохранён до появления личных шаблонов.</p>
            )}
          </details>
        );
      })}
      <div className="form-actions">
        <Button
          variant="secondary"
          disabled={!pages.length}
          onClick={() => {
            setBefore(pages.at(-1) ?? "");
            setPages((p) => p.slice(0, -1));
          }}
        >
          Новая история
        </Button>
        <Button
          variant="secondary"
          disabled={!q.data?.next}
          onClick={() => {
            setPages((p) => [...p, before]);
            setBefore(q.data?.next ?? "");
          }}
        >
          Более ранняя история
        </Button>
      </div>
    </Card>
  );
}
