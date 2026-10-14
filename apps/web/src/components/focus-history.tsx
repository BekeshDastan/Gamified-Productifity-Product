"use client";
import { useState, useDeferredValue } from "react";
import { useSearchParams } from "next/navigation";
import { DateTime } from "luxon";
import { z } from "zod";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings, formatMoment } from "@/lib/settings";
import {
  getFocusHistory,
  getFocusSession,
  type FocusSession,
  type FocusHistoryFilters,
} from "@/lib/focus";
import { duration } from "@/lib/focus-clock";
import { listQuests, emptyFilters, questLabels } from "@/lib/quests";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Input, Select, DatePicker } from "./ui/field";
import { Button } from "./ui/button";
import { ChoiceChips, EmptyScene, FilterPanel, StatTile } from "./ui/experience";
import { Timer, CheckCheck, Hourglass, Coffee } from "lucide-react";
const states: Record<string, string> = {
  running: "Идёт",
  paused: "На паузе",
  awaiting_confirmation: "Ожидает подтверждения",
  finished: "Подтверждена",
  cancelled: "Отменена",
};
export function FocusHistory({ serverNow }: { serverNow: string }) {
  const auth = useAuth(),
    settings = useSettings(),
    params = useSearchParams(),
    selected = params.get("session_id"),
    selectedValid = !!selected && z.uuid().safeParse(selected).success;
  const today = DateTime.fromISO(serverNow).setZone(settings.data?.timezone ?? "UTC");
  const [filters, setFilters] = useState<FocusHistoryFilters>({
      from: today.startOf("month").toISODate()!,
      to: today.toISODate()!,
    }),
    [archivedTasks, setArchivedTasks] = useState(false),
    [search, setSearch] = useState(""),
    [page, setPage] = useState(1),
    term = useDeferredValue(search);
  const valid =
    z.iso.date().safeParse(filters.from).success &&
    z.iso.date().safeParse(filters.to).success &&
    filters.from <= filters.to &&
    DateTime.fromISO(filters.to).diff(DateTime.fromISO(filters.from), "days").days <= 365;
  const q = useInfiniteQuery({
      queryKey: ["account", auth.session?.account.id, "focus-history", filters],
      enabled: !!auth.session && valid,
      initialPageParam: undefined as string | undefined,
      queryFn: ({ pageParam, signal }) => getFocusHistory(filters, pageParam, signal),
      getNextPageParam: (last) => last.next_cursor ?? undefined,
      refetchInterval: 15000,
    }),
    detail = useQuery({
      queryKey: ["account", auth.session?.account.id, "focus-session", selected],
      enabled: !!auth.session && selectedValid,
      queryFn: ({ signal }) => getFocusSession(selected!, signal),
    });
  const tasks = useQuery({
      queryKey: [
        "account",
        auth.session?.account.id,
        "focus-history-tasks",
        term,
        page,
        archivedTasks,
      ],
      enabled: !!auth.session,
      queryFn: ({ signal }) =>
        listQuests(page, signal, { ...emptyFilters, search: term, archived: archivedTasks }),
    }),
    labels = useQuery({
      queryKey: ["account", auth.session?.account.id, "quest-labels"],
      enabled: !!auth.session,
      queryFn: ({ signal }) => questLabels(signal),
    });
  const update = (key: keyof FocusHistoryFilters, value: string) =>
    setFilters((v) => ({ ...v, [key]: value || undefined }));
  const rows = q.data?.pages.flatMap((p) => p.items) ?? [],
    summary = q.data?.pages[0];
  const groups = new Map<string, FocusSession[]>();
  for (const row of rows) {
    const day = DateTime.fromISO(row.started_at).setZone(row.timezone).toISODate()!;
    groups.set(day, [...(groups.get(day) ?? []), row]);
  }
  return (
    <>
      {selected && (
        <Card>
          <h2>Выбранная сессия</h2>
          {!selectedValid ? (
            <p role="alert">Некорректная ссылка на сессию.</p>
          ) : detail.data ? (
            <SessionDetails session={detail.data} />
          ) : (
            <p role={detail.isError ? "alert" : "status"}>
              {detail.isError ? "Сессия недоступна этому аккаунту." : "Загружаем сессию…"}
            </p>
          )}
          <SafeLink href="/app/focus">Вернуться к общей истории</SafeLink>
        </Card>
      )}
      <Card className="focus-history-panel">
        <div className="panel-heading">
          <span className="panel-icon">
            <Timer aria-hidden="true" />
          </span>
          <div>
            <p className="eyebrow">ВАШ РИТМ</p>
            <h2>История фокуса</h2>
          </div>
        </div>
        {summary && (
          <div className="stat-grid" role="group" aria-label="Итоги фокуса">
            <StatTile
              label="Сессии"
              value={summary.summary.sessions}
              detail={`Рабочие интервалы: ${duration(summary.summary.work_seconds)}`}
              icon={<Timer />}
            />
            <StatTile
              label="Подтверждённая работа"
              value={duration(summary.summary.confirmed_seconds)}
              tone="mint"
              icon={<CheckCheck />}
            />
            <StatTile
              label="Без подтверждения"
              value={duration(summary.summary.pending_seconds)}
              tone="gold"
              icon={<Hourglass />}
            />
            <StatTile
              label="Отмечено отдыхом"
              value={duration(summary.summary.rest_seconds)}
              detail={`В отменённых сессиях: ${duration(summary.summary.cancelled_seconds)}`}
              tone="blue"
              icon={<Coffee />}
            />
          </div>
        )}
        <div className="section-toolbar">
          <ChoiceChips
            label="Период истории фокуса"
            value={
              filters.to !== today.toISODate()
                ? 0
                : filters.from === today.toISODate()
                  ? 1
                  : filters.from === today.minus({ days: 6 }).toISODate()
                    ? 7
                    : filters.from === today.startOf("month").toISODate()
                      ? 30
                      : 0
            }
            options={[
              { value: 1, label: "Сегодня" },
              { value: 7, label: "7 дней" },
              { value: 30, label: "Месяц" },
            ]}
            onChange={(days) =>
              setFilters((v) => ({
                ...v,
                from:
                  days === 30
                    ? today.startOf("month").toISODate()!
                    : today.minus({ days: days - 1 }).toISODate()!,
                to: today.toISODate()!,
              }))
            }
          />
        </div>
        <FilterPanel>
          <div className="quest-grid">
            <DatePicker
              label="Фокус с даты"
              value={filters.from}
              onChange={(e) => setFilters((v) => ({ ...v, from: e.target.value }))}
            />
            <DatePicker
              label="Фокус по дату"
              value={filters.to}
              onChange={(e) => setFilters((v) => ({ ...v, to: e.target.value }))}
            />
          </div>
          <Input
            label="Поиск задачи в истории фокуса"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
          <label>
            <input
              type="checkbox"
              checked={archivedTasks}
              onChange={(e) => {
                setArchivedTasks(e.target.checked);
                setPage(1);
              }}
            />{" "}
            Искать архивные задачи в истории
          </label>
          <Select
            label="Задача в истории фокуса"
            value={filters.quest_id ?? ""}
            onChange={(e) => update("quest_id", e.target.value)}
          >
            <option value="">Все задачи</option>
            {filters.quest_id && !tasks.data?.items.some((t) => t.id === filters.quest_id) && (
              <option value={filters.quest_id}>Ранее выбранная задача</option>
            )}
            {tasks.data?.items.map((t) => (
              <option key={t.id} value={t.id}>
                {t.title}
              </option>
            ))}
          </Select>
          <div className="auth-actions">
            <Button disabled={page === 1} onClick={() => setPage((v) => v - 1)}>
              Предыдущие задачи для фильтра
            </Button>
            <Button
              disabled={!tasks.data || page * 25 >= tasks.data.total}
              onClick={() => setPage((v) => v + 1)}
            >
              Следующие задачи для фильтра
            </Button>
          </div>
          <Select
            label="Направление в истории фокуса"
            value={filters.area_id ?? ""}
            onChange={(e) => update("area_id", e.target.value)}
          >
            <option value="">Все направления</option>
            {labels.data?.areas.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
                {a.archived ? " · архив" : ""}
              </option>
            ))}
          </Select>
          <Select
            label="Состояние в истории фокуса"
            value={filters.state ?? ""}
            onChange={(e) => update("state", e.target.value)}
          >
            <option value="">Все состояния</option>
            {Object.entries(states).map(([v, label]) => (
              <option key={v} value={v}>
                {label}
              </option>
            ))}
          </Select>
        </FilterPanel>
        {!valid && <p role="alert">Укажите диапазон от 1 до 366 дней.</p>}
        {(tasks.isError || labels.isError) && (
          <p role="alert">
            Не удалось загрузить фильтры.{" "}
            <Button
              onClick={() => {
                void tasks.refetch();
                void labels.refetch();
              }}
            >
              Повторить фильтры
            </Button>
          </p>
        )}
        {q.isError && (
          <p role="alert">
            Не удалось загрузить историю.{" "}
            <Button onClick={() => void q.refetch()}>Повторить историю</Button>
          </p>
        )}
        {valid && !q.data && !q.isError && <p role="status">Загружаем историю фокуса…</p>}
        {summary && (
          <>
            <p className="ui-hint">
              Итоги на{" "}
              {formatMoment(
                summary.as_of,
                settings.data ?? { timezone: "UTC", language: "ru", time_format: "24h" },
              )}
              .
            </p>
          </>
        )}
        {valid && summary?.items.length === 0 && (
          <EmptyScene
            title="Здесь появится ваш ритм"
            description="В этом диапазоне сессий пока нет. Начните с пяти минут фокуса."
          />
        )}
        <div className="focus-history">
          {Array.from(groups, ([day, sessions]) => (
            <section key={day}>
              <h3 className="timeline-date">
                {DateTime.fromISO(day).setLocale("ru").toFormat("d MMMM, cccc")}
              </h3>
              {sessions.map((s) => (
                <article key={s.id} id={"session-" + s.id}>
                  <h4>{s.title}</h4>
                  <SessionDetails session={s} />
                </article>
              ))}
            </section>
          ))}
        </div>
        {q.hasNextPage && (
          <Button disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
            Более ранние фокус-сессии
          </Button>
        )}
      </Card>
    </>
  );
}
function SessionDetails({ session: s }: { session: FocusSession }) {
  const settings = useSettings(),
    p = {
      ...(settings.data ?? { language: "ru" as const, time_format: "24h" as const }),
      timezone: s.timezone,
    };
  const result =
    s.result === "progress"
      ? "Есть результат"
      : s.result === "no_progress"
        ? "Пока без результата"
        : s.result === "rest"
          ? "Отмечено отдыхом"
          : "Итог не подтверждён";
  return (
    <>
      <div className="session-badges">
        <span className="state-badge" data-state={s.state}>
          {states[s.state]}
        </span>
        <span>{result}</span>
      </div>
      <p className="session-duration">
        Рабочие интервалы: {duration(s.active_seconds)} · перерывы: {duration(s.break_seconds)}.
      </p>
      <p className="ui-hint">
        {formatMoment(s.started_at, p)} · {s.timezone}
      </p>
      {s.comment && <p className="quest-description">{s.comment}</p>}
      {s.quest_id && (
        <SafeLink href={"/app/quests/" + s.quest_id}>Связанная задача и источник времени</SafeLink>
      )}
      {s.goal_id && <SafeLink href={"/app/goals/" + s.goal_id}>Связанная цель</SafeLink>}
      <details>
        <summary>Интервалы сессии</summary>
        <ol className="session-segments">
          {s.segments.map((seg) => (
            <li key={seg.id}>
              {seg.kind === "work" ? "Работа" : "Перерыв"}: {duration(seg.seconds)} ·{" "}
              {formatMoment(seg.started_at, p)} —{" "}
              {seg.ended_at ? formatMoment(seg.ended_at, p) : "идёт"}
            </li>
          ))}
        </ol>
      </details>
    </>
  );
}
