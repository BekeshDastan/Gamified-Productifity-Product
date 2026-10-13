"use client";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { accountQueryKey } from "@/lib/api/query-provider";
import { ApiError } from "@/lib/api/client";
import { useConfirmDiscard, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { draftKey } from "@/lib/private-drafts";
import {
  emptyJournal,
  journalInput,
  journalEntry,
  listJournal,
  getJournalEntry,
  saveJournalEntry,
  type JournalInput,
  type JournalEntry,
} from "@/lib/journal";
import { emptyGoalFilters, listGoals } from "@/lib/goals";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { useSettings } from "@/lib/settings";
import { DateTime } from "luxon";
import { z } from "zod";

const draftSchema = z.object({
  input: journalInput.extend({ title: z.string().max(200) }),
  current: journalEntry.nullable(),
});
export function Journal() {
  const auth = useAuth(),
    settings = useSettings();
  return auth.session && settings.data ? (
    <JournalOwner
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
    />
  ) : (
    <p>Проверяем вход…</p>
  );
}
function JournalOwner({ account, timezone }: { account: string; timezone: string }) {
  const cache = useQueryClient(),
    discard = useConfirmDiscard();
  const [input, setInput] = useState<JournalInput>(() =>
    emptyJournal(DateTime.now().setZone(timezone).toISODate()!),
  );
  const [current, setCurrent] = useState<JournalEntry | null>(null),
    [baseline, setBaseline] = useState(() => JSON.stringify(input));
  const [draft, setDraft] = useState<z.infer<typeof draftSchema> | null>(null),
    [keepDraft, setKeepDraft] = useState(true);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const [filters, setFilters] = useState({
      from: "",
      to: "",
      tag: "",
      archived: "false",
      after: "",
    }),
    [pages, setPages] = useState<string[]>([]);
  const [goalSearch, setGoalSearch] = useState(""),
    [goalPage, setGoalPage] = useState(1);
  const operation = useRef({ body: "", key: "" });
  const lock = useRef(false);
  const dirty = JSON.stringify(input) !== baseline;
  useUnsavedChanges(dirty);
  const storageKey = draftKey(account, "journal");
  const q = useQuery({
    queryKey: accountQueryKey(account, "journal", filters),
    queryFn: ({ signal }) => listJournal(filters, signal),
  });
  const goals = useQuery({
    queryKey: accountQueryKey(account, "journal-goals", goalSearch, goalPage),
    queryFn: ({ signal }) =>
      listGoals(goalPage, { ...emptyGoalFilters, search: goalSearch }, signal),
  });
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      try {
        const raw = sessionStorage.getItem(storageKey);
        if (raw && raw.length < 400000) {
          const parsed = draftSchema.safeParse(JSON.parse(raw));
          if (active && parsed.success) setDraft(parsed.data);
        }
      } catch {
        /* No draft if browser storage is disabled. */
      }
    });
    return () => {
      active = false;
    };
  }, [storageKey]);
  useEffect(() => {
    if (!dirty) return;
    try {
      if (keepDraft) sessionStorage.setItem(storageKey, JSON.stringify({ input, current }));
      else sessionStorage.removeItem(storageKey);
    } catch {
      /* The unsaved-change warning remains active. */
    }
  }, [dirty, keepDraft, input, current, storageKey]);
  function change<K extends keyof JournalInput>(key: K, value: JournalInput[K]) {
    setInput((old) => ({ ...old, [key]: value }));
  }
  function clearDraft() {
    try {
      sessionStorage.removeItem(storageKey);
    } catch {}
    setDraft(null);
  }
  async function save() {
    if (lock.current) return;
    const valid = journalInput.safeParse(input);
    if (!valid.success) {
      setError("Проверьте дату, заголовок, текст и теги. Заголовок обязателен.");
      return;
    }
    lock.current = true;
    setBusy(true);
    setError("");
    const body = JSON.stringify({ input: valid.data, id: current?.id, version: current?.version });
    if (operation.current.body !== body) operation.current = { body, key: crypto.randomUUID() };
    try {
      const saved = await saveJournalEntry(valid.data, current, operation.current.key);
      setCurrent(saved);
      const next = journalInput.parse(saved);
      setInput(next);
      setBaseline(JSON.stringify(next));
      clearDraft();
      setNotice("Запись сохранена. Её видите только вы.");
      await cache.invalidateQueries({ queryKey: accountQueryKey(account, "journal") });
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 409
          ? "Запись изменилась на другом устройстве. Ваш текст сохранён в форме; загрузите свежую версию перед повтором."
          : "Не удалось сохранить запись. Текст остался в форме; можно повторить.",
      );
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  async function edit(entry: JournalEntry) {
    if (!discard()) return;
    setBusy(true);
    try {
      const fresh = await getJournalEntry(entry.id);
      setCurrent(fresh);
      const next = journalInput.parse(fresh);
      setInput(next);
      setBaseline(JSON.stringify(next));
      setError("");
      setNotice("");
      clearDraft();
    } catch {
      setError("Не удалось загрузить запись");
    } finally {
      setBusy(false);
    }
  }
  function nextFilters(next: Partial<typeof filters>) {
    setFilters((old) => ({ ...old, ...next, after: "" }));
    setPages([]);
  }
  return (
    <div className="journal-layout">
      <Card>
        <h2>Только для вас</h2>
        <p>
          Личные записи не входят в общую ленту и отчёты другим людям. Черновик хранится в этой
          вкладке и удаляется при выходе или смене аккаунта.
        </p>
      </Card>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {draft && (
        <Card>
          <h2>Найден несохранённый черновик</h2>
          <p>Восстановить текст, оставшийся в этой вкладке?</p>
          <Button
            disabled={busy}
            onClick={() => {
              if (!discard()) return;
              setInput(draft.input);
              setCurrent(draft.current);
              setBaseline(
                JSON.stringify(
                  draft.current
                    ? journalInput.parse(draft.current)
                    : emptyJournal(draft.input.date),
                ),
              );
              setDraft(null);
            }}
          >
            Восстановить черновик
          </Button>
          <Button variant="secondary" onClick={clearDraft}>
            Удалить черновик
          </Button>
        </Card>
      )}
      <Card>
        <h2>{current ? "Редактирование записи" : "Новая запись"}</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <fieldset disabled={busy} className="ui-form-fields">
            <label className="ui-field">
              Дата записи
              <input
                className="ui-input"
                type="date"
                value={input.date}
                onChange={(e) => change("date", e.target.value)}
                required
              />
            </label>
            <label className="ui-field">
              Заголовок записи
              <input
                className="ui-input"
                value={input.title}
                maxLength={200}
                onChange={(e) => change("title", e.target.value)}
                required
              />
            </label>
            <label className="ui-field" htmlFor="journal-entry-body">
              Текст записи
            </label>
            <textarea
              id="journal-entry-body"
              className="ui-input journal-body"
              value={input.body}
              maxLength={50000}
              onChange={(e) => change("body", e.target.value)}
            />
            <p className="ui-hint">
              Поддерживаются заголовки #, выделение **текст**, списки, блоки кода и ссылки https.
              HTML и изображения не исполняются.
            </p>
            <label className="ui-field">
              Теги записи через запятую
              <input
                className="ui-input"
                value={input.tags.join(",")}
                onChange={(e) => change("tags", e.target.value ? e.target.value.split(",") : [])}
              />
            </label>
            <label className="ui-field">
              Поиск цели для записи
              <input
                className="ui-input"
                value={goalSearch}
                onChange={(e) => {
                  setGoalSearch(e.target.value);
                  setGoalPage(1);
                }}
              />
            </label>
            <label className="ui-field">
              Цель записи
              <select
                className="ui-input"
                value={input.goal_id ?? ""}
                onChange={(e) => change("goal_id", e.target.value || null)}
              >
                <option value="">Без цели</option>
                {input.goal_id && !goals.data?.items.some((g) => g.id === input.goal_id) && (
                  <option value={input.goal_id}>Сохранённая цель</option>
                )}
                {goals.data?.items.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}
                  </option>
                ))}
              </select>
            </label>
            <div className="form-actions">
              <Button
                type="button"
                variant="secondary"
                disabled={goalPage === 1}
                onClick={() => setGoalPage((n) => n - 1)}
              >
                Предыдущие цели
              </Button>
              <Button
                type="button"
                variant="secondary"
                disabled={!goals.data || goalPage * 25 >= goals.data.total}
                onClick={() => setGoalPage((n) => n + 1)}
              >
                Следующие цели
              </Button>
            </div>
            <label>
              <input
                type="checkbox"
                checked={input.archived}
                onChange={(e) => change("archived", e.target.checked)}
              />{" "}
              В архиве
            </label>
            <br />
            <label>
              <input
                type="checkbox"
                checked={keepDraft}
                onChange={(e) => setKeepDraft(e.target.checked)}
              />{" "}
              Сохранять черновик в этой вкладке
            </label>
            <div className="form-actions">
              <Button type="submit" disabled={busy}>
                Сохранить запись
              </Button>
              <Button
                type="button"
                variant="secondary"
                disabled={busy}
                onClick={() => {
                  if (!discard()) return;
                  const next = emptyJournal(input.date);
                  setCurrent(null);
                  setInput(next);
                  setBaseline(JSON.stringify(next));
                  clearDraft();
                  setError("");
                  setNotice("");
                }}
              >
                Новая запись
              </Button>
            </div>
          </fieldset>
        </form>
        {current && (
          <article
            aria-label="Предпросмотр сохранённой записи"
            className="journal-markdown"
            dangerouslySetInnerHTML={{ __html: current.html }}
          />
        )}
      </Card>
      <Card>
        <h2>Мои записи</h2>
        <label className="ui-field">
          Записи с даты
          <input
            className="ui-input"
            type="date"
            value={filters.from}
            onChange={(e) => nextFilters({ from: e.target.value })}
          />
        </label>
        <label className="ui-field">
          Записи по дату
          <input
            className="ui-input"
            type="date"
            value={filters.to}
            onChange={(e) => nextFilters({ to: e.target.value })}
          />
        </label>
        <label className="ui-field">
          Фильтр по тегу
          <input
            className="ui-input"
            value={filters.tag}
            onChange={(e) => nextFilters({ tag: e.target.value })}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={filters.archived === "true"}
            onChange={(e) => nextFilters({ archived: String(e.target.checked) })}
          />{" "}
          Архив записей
        </label>
        {q.isPending && <p>Загружаем записи…</p>}
        {q.isError && (
          <p role="alert">
            Не удалось загрузить записи.{" "}
            <Button onClick={() => void q.refetch()}>Повторить загрузку</Button>
          </p>
        )}
        {q.data?.items.length === 0 && <p>За выбранный период записей пока нет.</p>}
        {q.data?.items.map((entry) => (
          <article key={entry.id}>
            <h3>{entry.title}</h3>
            <p>
              {entry.date} · {entry.tags.join(", ") || "Без тегов"}
            </p>
            <Button disabled={busy} variant="secondary" onClick={() => void edit(entry)}>
              Открыть запись «{entry.title}»
            </Button>
          </article>
        ))}
        <div className="form-actions">
          <Button
            variant="secondary"
            disabled={!pages.length}
            onClick={() => {
              const previous = pages.at(-1) ?? "";
              setPages((p) => p.slice(0, -1));
              setFilters((f) => ({ ...f, after: previous }));
            }}
          >
            Предыдущие записи
          </Button>
          <Button
            variant="secondary"
            disabled={!q.data?.next}
            onClick={() => {
              setPages((p) => [...p, filters.after]);
              setFilters((f) => ({ ...f, after: q.data?.next ?? "" }));
            }}
          >
            Следующие записи
          </Button>
        </div>
      </Card>
    </div>
  );
}
