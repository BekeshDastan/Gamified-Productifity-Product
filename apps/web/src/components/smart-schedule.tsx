"use client";
import { useState } from "react";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { listQuests } from "@/lib/quests";
import { SafeLink, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import * as api from "@/lib/smart-schedule";
import { useChallengeAction } from "./challenges";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { ChoiceChips, EmptyScene } from "./ui/experience";
import { CalendarClock, Radar, Clock3, CircleCheck } from "lucide-react";
export function SmartSchedule() {
  const auth = useAuth(),
    settings = useSettings(),
    id = auth.session?.account.id;
  if (!id || !settings.data) return null;
  return (
    <ScheduleWorkspace
      key={id + settings.data.timezone}
      account={id}
      timezone={settings.data.timezone}
    />
  );
}
function ScheduleWorkspace({ account, timezone }: { account: string; timezone: string }) {
  const today = DateTime.now().setZone(timezone),
    [from, setFrom] = useState(today.toISODate()!),
    [end, setEnd] = useState(today.plus({ days: 6 }).toISODate()!),
    [between, setBetween] = useState(5),
    [page, setPage] = useState(1),
    [selected, setSelected] = useState<api.ScheduleInput["tasks"]>([]),
    [breakStart, setBreakStart] = useState(""),
    [breakEnd, setBreakEnd] = useState(""),
    [preview, setPreview] = useState<api.SchedulePreview | null>(null),
    [operation, setOperation] = useState(""),
    [ack, setAck] = useState(false),
    [notice, setNotice] = useState(""),
    action = useChallengeAction(),
    cache = useQueryClient();
  useUnsavedChanges(!!preview);
  const tasks = useQuery({
      queryKey: ["account", account, "schedule-tasks", page],
      queryFn: ({ signal }) => listQuests(page, signal),
    }),
    radar = useQuery({
      queryKey: ["account", account, "deadline-radar"],
      queryFn: ({ signal }) => api.getRadar(signal),
      refetchInterval: 60000,
    });
  const names: Record<string, string> = {
    history: "Завершённая или архивная задача",
    blocked: "Сначала завершите зависимость",
    container: "Контейнер: выберите конечные шаги",
    existing_block: "Уже есть календарный блок",
    already_planned: "Уже назначена дата: пересмотрите её в обычном плане",
    unknown_estimate: "Сначала укажите оценку времени",
    below_minimum: "Нужен блок от 15 минут или отдельный старт на 5 минут",
    capacity: "Нет подходящего слота до срока",
  };
  return (
    <>
      <Card className="schedule-studio">
        <div className="panel-heading">
          <span className="panel-icon">
            <CalendarClock aria-hidden="true" />
          </span>
          <div>
            <p className="eyebrow">НАЙДЁМ МЕСТО ДЛЯ ВАЖНОГО</p>
            <h2>Предложить расписание</h2>
          </div>
        </div>
        <details className="quiet-details">
          <summary>Как подбираем время</summary>
          <p>
            Используются рабочие окна и доступные минуты из планов дня. Фиксированные события
            сохраняются. <SafeLink href="/app/planning">Настроить окна дня</SafeLink>. Уже
            назначенные задачи пересматриваются в обычном плане.
          </p>
        </details>
        <ChoiceChips
          label="Быстрый период расписания"
          value={
            from === today.toISODate() && end === today.toISODate()
              ? 1
              : from === today.toISODate() && end === today.plus({ days: 6 }).toISODate()
                ? 7
                : 0
          }
          options={[
            { value: 1, label: "Сегодня" },
            { value: 7, label: "Ближайшие 7 дней" },
          ]}
          disabled={!!preview}
          onChange={(days) => {
            setFrom(today.toISODate()!);
            setEnd(today.plus({ days: days - 1 }).toISODate()!);
          }}
        />
        <div className="schedule-range">
          <Input
            label="Первый день расписания"
            type="date"
            value={from}
            disabled={!!preview}
            onChange={(e) => setFrom(e.target.value)}
          />
          <Input
            label="Последний день расписания включительно"
            type="date"
            value={end}
            disabled={!!preview}
            onChange={(e) => setEnd(e.target.value)}
          />
          <Input
            label="Перерыв между блоками, минут"
            type="number"
            min={0}
            max={60}
            value={between}
            disabled={!!preview}
            onChange={(e) => setBetween(Number(e.target.value))}
          />
        </div>
        <details>
          <summary>Добавить обязательный перерыв</summary>
          <p>
            Укажите местное время в зоне {timezone}. Перерыв будет показан в предложении и сохранён
            только при подтверждении.
          </p>
          <Input
            label="Начало обязательного перерыва"
            type="datetime-local"
            value={breakStart}
            disabled={!!preview}
            onChange={(e) => setBreakStart(e.target.value)}
          />
          <Input
            label="Конец обязательного перерыва"
            type="datetime-local"
            value={breakEnd}
            disabled={!!preview}
            onChange={(e) => setBreakEnd(e.target.value)}
          />
        </details>
        <h3>Выберите задачи</h3>
        {tasks.isError && (
          <p role="alert">
            Не удалось загрузить задачи.{" "}
            <Button onClick={() => void tasks.refetch()}>Повторить</Button>
          </p>
        )}
        {tasks.isPending && <p role="status">Загружаем задачи…</p>}
        <div className="schedule-task-grid">
          {tasks.data?.items
            .filter((q) => !q.archived_at && !["completed", "failed", "skipped"].includes(q.status))
            .map((q) => {
              const value = selected.find((v) => v.id === q.id);
              return (
                <fieldset
                  key={q.id}
                  className="schedule-task"
                  data-selected={!!value}
                  disabled={!!preview || action.pending}
                >
                  <legend className="sr-only">{q.title}</legend>
                  <label className="schedule-task-choice">
                    <input
                      type="checkbox"
                      aria-label={`Выбрать «${q.title}»`}
                      checked={!!value}
                      onChange={(e) =>
                        setSelected(
                          e.target.checked
                            ? [
                                ...selected,
                                { id: q.id, allow_split: false, five_minute_start: false },
                              ]
                            : selected.filter((v) => v.id !== q.id),
                        )
                      }
                    />{" "}
                    <strong>{q.title}</strong>
                    <CircleCheck size={20} aria-hidden="true" />
                  </label>
                  <p className="task-time-chip">
                    <Clock3 size={13} aria-hidden="true" />
                    {q.estimated_minutes > 0
                      ? `${q.estimated_minutes} минут`
                      : "Оценка не задана"}{" "}
                    · <SafeLink href={"/app/quests/" + q.id}>Изменить задачу</SafeLink>
                  </p>
                  {value && (
                    <>
                      <label>
                        <input
                          type="checkbox"
                          checked={value.allow_split}
                          onChange={(e) =>
                            setSelected(
                              selected.map((v) =>
                                v.id === q.id ? { ...v, allow_split: e.target.checked } : v,
                              ),
                            )
                          }
                        />{" "}
                        Разрешаю дробить «{q.title}» на блоки от 15 минут
                      </label>
                      <label>
                        <input
                          type="checkbox"
                          checked={value.five_minute_start}
                          onChange={(e) =>
                            setSelected(
                              selected.map((v) =>
                                v.id === q.id ? { ...v, five_minute_start: e.target.checked } : v,
                              ),
                            )
                          }
                        />{" "}
                        Только первый старт на 5 минут для «{q.title}»
                      </label>
                    </>
                  )}
                </fieldset>
              );
            })}
        </div>
        {tasks.data?.items.length === 0 && (
          <EmptyScene
            title="Добавьте первый шаг"
            description="Задачи с оценкой времени можно распределить по свободным окнам."
          >
            <SafeLink href="/app/quests">Открыть задачи</SafeLink>
          </EmptyScene>
        )}
        <div className="button-row">
          <Button
            variant="secondary"
            disabled={page === 1 || !!preview}
            onClick={() => setPage(page - 1)}
          >
            Предыдущие задачи
          </Button>
          <Button
            variant="secondary"
            disabled={!tasks.data || page * tasks.data.page_size >= tasks.data.total || !!preview}
            onClick={() => setPage(page + 1)}
          >
            Следующие задачи
          </Button>
        </div>
        <p>Выбрано: {selected.length}.</p>
        <Button
          disabled={!selected.length || !!preview || action.pending}
          onClick={() =>
            void action.run(async () => {
              setNotice("");
              const breaks = [];
              if (breakStart || breakEnd) {
                const a = DateTime.fromISO(breakStart, { zone: timezone }),
                  b = DateTime.fromISO(breakEnd, { zone: timezone });
                if (!a.isValid || !b.isValid || b <= a)
                  throw Error("Укажите начало и конец перерыва.");
                breaks.push({ start: a.toUTC().toISO()!, end: b.toUTC().toISO()! });
              }
              const v = await api.previewSchedule({
                from,
                to: DateTime.fromISO(end).plus({ days: 1 }).toISODate()!,
                tasks: selected,
                breaks,
                between_minutes: between,
              });
              setPreview(v);
              setOperation(crypto.randomUUID());
              setAck(false);
            })
          }
        >
          Рассчитать предложение расписания
        </Button>
        {preview && (
          <section className="schedule-preview">
            <h3>Предпросмотр расписания</h3>
            <p>
              Действует до{" "}
              {DateTime.fromISO(preview.expires_at).setZone(timezone).toFormat("HH:mm:ss")}. Часовой
              пояс: {timezone}. Дедлайны сохраняются.
            </p>
            {preview.placements.map((p, i) => (
              <p className="schedule-placement" key={i}>
                <SafeLink href={"/app/quests/" + p.task_id}>{p.title}</SafeLink> ·{" "}
                {DateTime.fromISO(p.start).setZone(timezone).toFormat("dd.LL HH:mm")} —{" "}
                {DateTime.fromISO(p.end).setZone(timezone).toFormat("HH:mm")} · {p.minutes} минут{" "}
                {p.partial_start ? "· только старт, задача остаётся незавершённой" : ""}
                {DateTime.fromISO(p.break_end) > DateTime.fromISO(p.end)
                  ? ` · затем перерыв до ${DateTime.fromISO(p.break_end).setZone(timezone).toFormat("HH:mm")}`
                  : ""}
              </p>
            ))}
            {preview.input.breaks.map((b, i) => (
              <p key={"break" + i}>
                Обязательный перерыв:{" "}
                {DateTime.fromISO(b.start).setZone(timezone).toFormat("dd.LL HH:mm")} —{" "}
                {DateTime.fromISO(b.end).setZone(timezone).toFormat("HH:mm")}
              </p>
            ))}
            {preview.unscheduled.map((q) => (
              <p key={q.task_id}>
                <SafeLink href={"/app/quests/" + q.task_id}>Неразмещённая задача</SafeLink>:{" "}
                {names[q.reason] ?? q.reason}; оценка{" "}
                {q.reason === "unknown_estimate" ? "не задана" : `${q.required_minutes} минут`},
                свободно до срока {q.free_minutes}, арифметический дефицит {q.deficit_minutes}. Слот
                также должен подходить по длине.
              </p>
            ))}
            {preview.warnings.map((w) => (
              <p key={w}>{w}</p>
            ))}
            <details>
              <summary>Исходные версии и фиксированные события</summary>
              {preview.days.map((d) => (
                <div key={d.date}>
                  <h4>
                    {d.date} · план версии {d.plan_version}
                  </h4>
                  {d.blocks.map((b) => (
                    <p key={b.id}>
                      {b.title} · версия {b.version} ·{" "}
                      {b.all_day
                        ? "Весь день"
                        : DateTime.fromISO(b.start).setZone(timezone).toFormat("HH:mm")}
                    </p>
                  ))}
                </div>
              ))}
            </details>
            <label>
              <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
              Подтверждаю сохранение предложенных блоков и дополнений планов
            </label>
            <div className="button-row">
              <Button
                disabled={!ack || !preview.placements.length || action.pending}
                onClick={() =>
                  void action.run(async () => {
                    setAck(false);
                    const result = await api.applySchedule({
                      operation_id: operation,
                      preview_id: preview.id,
                      confirm: true,
                    });
                    setPreview(null);
                    setSelected([]);
                    setNotice(`Сохранено календарных блоков: ${result.blocks.length}.`);
                    await cache.invalidateQueries({ queryKey: ["account", account] });
                  })
                }
              >
                Применить предложение расписания
              </Button>
              <Button
                variant="secondary"
                disabled={action.pending}
                onClick={() => {
                  setPreview(null);
                  setAck(false);
                }}
              >
                Отменить предложение расписания
              </Button>
            </div>
          </section>
        )}
        {action.error && (
          <p role="alert">
            {action.error} Если исходные записи изменились, отмените предложение и рассчитайте
            новое.
          </p>
        )}
        {notice && (
          <p role="status">
            {notice} <SafeLink href="/app/calendar">Открыть календарь</SafeLink>
          </p>
        )}
      </Card>
      <Card className="radar-panel">
        <section id="deadline-radar">
          <div className="panel-heading">
            <span className="panel-icon">
              <Radar aria-hidden="true" />
            </span>
            <div>
              <p className="eyebrow">ДЕРЖИМ СРОКИ В ПОЛЕ ЗРЕНИЯ</p>
              <h2>Deadline Radar</h2>
            </div>
          </div>
          {radar.isError && (
            <p role="alert">
              Не удалось проверить сроки.{" "}
              <Button onClick={() => void radar.refetch()}>Повторить</Button>
            </p>
          )}
          {radar.isPending && <p role="status">Проверяем сроки и рабочие окна…</p>}
          {radar.data && (
            <>
              <div className="radar-buckets">
                {radar.data.buckets.map((b, i) => (
                  <details key={b.id} className="radar-bucket" data-urgency={i}>
                    <summary>
                      <span>{b.label}</span>
                      <strong>{b.items.length}</strong>
                      <small>{b.items.length ? "Показать задачи" : "Задач нет"}</small>
                    </summary>
                    <div className="radar-numbers">
                      <span>
                        Нужно<strong>{b.required_minutes} мин</strong>
                      </span>
                      <span>
                        Свободно<strong>{b.free_minutes} мин</strong>
                      </span>
                      <span>
                        Дефицит<strong>{b.deficit_minutes} мин</strong>
                      </span>
                    </div>
                    <p className="ui-hint">
                      С более срочными: {b.cumulative_minutes} мин · Без оценки:{" "}
                      {b.unknown_estimates}
                    </p>
                    <ul>
                      {b.items.map((q) => (
                        <li key={q.id}>
                          <SafeLink href={q.url}>{q.title}</SafeLink> ·{" "}
                          {DateTime.fromISO(q.deadline).setZone(timezone).toFormat("dd.LL HH:mm")} ·{" "}
                          {q.estimated_minutes === null
                            ? "Сначала укажите оценку"
                            : `${q.estimated_minutes} минут`}
                        </li>
                      ))}
                    </ul>
                  </details>
                ))}
              </div>
              {radar.data.buckets.every((b) => !b.items.length) && (
                <EmptyScene
                  title="На горизонте спокойно"
                  description="Незавершённых обязательных задач со сроком нет."
                  tone="mint"
                />
              )}
              <details className="quiet-details">
                <summary>Как рассчитывается нагрузка</summary>
                <p>{radar.data.notice}</p>
              </details>
            </>
          )}
        </section>
      </Card>
    </>
  );
}
