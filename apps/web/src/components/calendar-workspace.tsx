"use client";
import { useRef, useState } from "react";
import type { EventApi } from "@fullcalendar/core";
import { z } from "zod";
import { fromWall, toWall } from "@/lib/calendar-time";
import { ApiError } from "@/lib/api/client";
import FullCalendar from "@fullcalendar/react";
import dayGrid from "@fullcalendar/daygrid";
import timeGrid from "@fullcalendar/timegrid";
import list from "@fullcalendar/list";
import multiMonth from "@fullcalendar/multimonth";
import interaction from "@fullcalendar/interaction";
import luxon from "@fullcalendar/luxon3";
import ru from "@fullcalendar/core/locales/ru";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import {
  calendarRange,
  calendarInput,
  createEvent,
  editCalendar,
  createSeries,
  getSeries,
  changeSeries,
  type CalendarSeries,
  type CalendarInput,
  type CalendarEvent,
} from "@/lib/calendar";
import { questLabels, statuses } from "@/lib/quests";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { QuestPicker } from "./quest-structure";
const views = {
  timeGridDay: "День",
  timeGridWeek: "Неделя",
  dayGridMonth: "Месяц",
  multiMonthYear: "Год",
  listMonth: "Список",
};
export default function CalendarWorkspace() {
  const auth = useAuth();
  const account = auth.session?.account.id;
  const settings = useSettings();
  const cache = useQueryClient();
  const discard = useConfirmDiscard();
  const ref = useRef<FullCalendar>(null);
  const [range, setRange] = useState({ from: "", to: "" });
  const [view, setView] = useState("dayGridMonth");
  const [area, setArea] = useState("");
  const [status, setStatus] = useState("");
  const [source, setSource] = useState("");
  const [editing, setEditing] = useState(false);
  const [selected, setSelected] = useState<CalendarEvent>();
  const [notice, setNotice] = useState("");
  const [mutationError, setMutationError] = useState("");
  const moving = useRef(false);
  const labels = useQuery({
    queryKey: ["account", account, "quest-labels"],
    enabled: !!account,
    queryFn: ({ signal }) => questLabels(signal),
  });
  const data = useQuery({
    queryKey: ["account", account, "calendar", range, settings.data?.timezone],
    enabled: !!account && !!range.from,
    queryFn: ({ signal }) => calendarRange(range.from, range.to, signal),
    refetchInterval: (q) => (q.state.data?.materializing ? 500 : false),
  });
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!account)
    return (
      <p>
        Для календаря <SafeLink href="/auth/login?next=/app/calendar">войдите в аккаунт</SafeLink>.
      </p>
    );
  if (settings.isPending || labels.isPending) return <p role="status">Загружаем настройки…</p>;
  if (!settings.data || !labels.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить настройки.</p>
        <Button
          onClick={() => {
            void settings.refetch();
            void labels.refetch();
          }}
        >
          Повторить
        </Button>
      </Card>
    );
  const zone = settings.data.timezone;
  const items = (data.data?.items ?? []).filter(
    (e) =>
      (!area || e.area_id === area) &&
      (!status || (e.cancelled ? "cancelled" : e.status) === status) &&
      (!source || e.source === source),
  );
  function changed() {
    setEditing(false);
    setSelected(undefined);
    setNotice("Событие сохранено.");
    void cache.invalidateQueries({ queryKey: ["account"] });
  }
  async function moved(info: { event: EventApi; revert: () => void }) {
    const current = info.event.extendedProps.record as CalendarEvent;
    if (moving.current || !discard()) {
      info.revert();
      return;
    }
    moving.current = true;
    setMutationError("");
    try {
      await editCalendar(
        current,
        {
          ...current,
          start: info.event.startStr,
          end: info.event.endStr,
          all_day: info.event.allDay,
        },
        "Перенос или изменение длительности в календаре",
        crypto.randomUUID(),
      );
      changed();
    } catch {
      info.revert();
      setMutationError(
        "Изменение не сохранено. Событие возвращено на прежнее место. Обновите диапазон и попробуйте снова.",
      );
    } finally {
      moving.current = false;
    }
  }
  return (
    <div className="quest-workspace calendar-workspace">
      <div className="button-row">
        <Button
          onClick={() => {
            if (discard()) {
              setEditing(!editing);
              setSelected(undefined);
            }
          }}
        >
          Добавить событие
        </Button>
        <span>Часовой пояс: {zone}</span>
      </div>
      {notice && <p role="status">{notice}</p>}
      {mutationError && (
        <p role="alert">
          {mutationError} <Button onClick={() => void data.refetch()}>Обновить диапазон</Button>
        </p>
      )}
      {editing && (
        <Card>
          <h2>Новое событие</h2>
          <EventForm key="new" zone={zone} areas={labels.data.areas} onSaved={changed} />
        </Card>
      )}
      {selected && (
        <Card>
          <h2>{selected.title}</h2>
          <p>
            {selected.start} — {selected.end}
          </p>
          <p>
            {selected.cancelled
              ? "Отменено"
              : (statuses[selected.status as keyof typeof statuses] ?? "Запланировано")}
          </p>
          {selected.reason && <p>{selected.reason}</p>}
          {!["deadline", "history"].includes(selected.source) && (
            <ExistingEventForm
              key={selected.id + ":" + selected.version}
              zone={zone}
              areas={labels.data.areas}
              current={selected}
              onSaved={changed}
            />
          )}
          {selected.quest_id && (
            <SafeLink className="text-link" href={"/app/quests/" + selected.quest_id}>
              Открыть задачу
            </SafeLink>
          )}
        </Card>
      )}
      <div className="quest-form-grid">
        <Select
          label="Вид календаря"
          value={view}
          onChange={(e) => {
            if (discard()) {
              setView(e.target.value);
              ref.current?.getApi().changeView(e.target.value);
            }
          }}
        >
          {Object.entries(views).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </Select>
        <Select
          label="Направление календаря"
          value={area}
          onChange={(e) => setArea(e.target.value)}
        >
          <option value="">Все направления</option>
          {labels.data.areas.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </Select>
        <Select label="Статус календаря" value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">Все статусы</option>
          <option value="planned">Запланировано</option>
          <option value="cancelled">Отменено</option>
          {Object.entries(statuses).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </Select>
        <Select
          label="Источник календаря"
          value={source}
          onChange={(e) => setSource(e.target.value)}
        >
          <option value="">Все источники</option>
          <option value="quest">Планы задач</option>
          <option value="block">Блоки задач</option>
          <option value="event">События</option>
          <option value="deadline">Дедлайны</option>
          <option value="history">Прежние планы</option>
        </Select>
      </div>
      <p className="ui-hint">
        ✓ Выполнено · ↷ Перенесено · − Пропущено · × Не получилось · ⊘ Отменено. «Срок» — дедлайн,
        «Прежний план» — дата до переноса; эти отметки доступны только для просмотра.
      </p>
      {(data.isFetching || data.data?.materializing) && <p role="status">Обновляем события…</p>}
      {data.isError && (
        <p role="alert">
          Не удалось загрузить диапазон.{" "}
          <Button onClick={() => void data.refetch()}>Повторить</Button>
        </p>
      )}
      <div className="calendar-scroll" aria-label="Сетка календаря" tabIndex={0}>
        <FullCalendar
          ref={ref}
          plugins={[dayGrid, timeGrid, list, multiMonth, interaction, luxon]}
          locale={ru}
          timeZone={zone}
          firstDay={settings.data.week_start % 7}
          initialView="dayGridMonth"
          headerToolbar={{ left: "prev,next today", center: "title", right: "" }}
          buttonText={{ today: "Сегодня" }}
          height="auto"
          allDayText="Без времени"
          noEventsText="В этом диапазоне нет событий"
          multiMonthMinWidth={260}
          eventInteractive={true}
          editable={true}
          eventDrop={(info) => void moved(info)}
          eventResize={(info) => void moved(info)}
          eventAllow={(_, event) => {
            const e = event?.extendedProps.record as CalendarEvent | undefined;
            return (
              !!e &&
              !["deadline", "history"].includes(e.source) &&
              !e.cancelled &&
              !["completed", "failed", "skipped"].includes(e.status)
            );
          }}
          datesSet={(info) =>
            setRange((old) => {
              const next = { from: info.startStr.slice(0, 10), to: info.endStr.slice(0, 10) };
              return old.from === next.from && old.to === next.to ? old : next;
            })
          }
          events={items.map((e) => ({
            id: e.id,
            title:
              (e.cancelled
                ? "⊘ "
                : ({ completed: "✓ ", postponed: "↷ ", skipped: "− ", failed: "× " }[e.status] ??
                  "")) + e.title,
            start: e.start,
            end: e.end,
            allDay: e.all_day,
            extendedProps: { record: e },
            durationEditable: !["quest", "deadline", "history"].includes(e.source),
            startEditable:
              !["deadline", "history"].includes(e.source) &&
              !e.cancelled &&
              !["completed", "failed", "skipped"].includes(e.status),
            classNames: ["calendar-status-" + e.status],
            borderColor: labels.data?.areas.find((a) => a.id === e.area_id)?.color,
          }))}
          eventClick={(info) => {
            if (discard()) {
              setSelected(info.event.extendedProps.record as CalendarEvent);
              setEditing(false);
            }
          }}
        />
      </div>
      <details>
        <summary>События диапазона — доступный список ({items.length})</summary>
        <ul className="quest-plain-list">
          {items.map((e) => (
            <li key={e.id}>
              <Button
                variant="ghost"
                onClick={() => {
                  if (discard()) {
                    setSelected(e);
                    setEditing(false);
                  }
                }}
              >
                {e.start.slice(0, 10)} · {e.title}
              </Button>
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}
function EventForm({
  zone,
  areas,
  onSaved,
  current,
  series,
}: {
  zone: string;
  areas: { id: string; name: string }[];
  onSaved: () => void;
  current?: CalendarEvent;
  series?: CalendarSeries;
}) {
  const key = useRef(crypto.randomUUID());
  const schema = calendarInput.extend({
    reason: z.string().max(1000),
    cancelled: z.boolean(),
    frequency: z.enum(["none", "daily", "weekly", "monthly"]),
    interval: z.number().int().min(1).max(52),
    weekdays: z.array(z.number()),
    ending: z.enum(["count", "until"]),
    count: z.number().int().min(1).max(10000),
    until: z.string(),
    kind: z.enum(["event", "task"]),
    scope: z.enum(["this", "future", "end"]),
  });
  const editor = useValidatedForm<z.infer<typeof schema>>({
    schema,
    defaultValues: {
      ...(current ?? {
        title: "",
        area_id: null,
        quest_id: null,
        all_day: true,
        start: "",
        end: "",
        timezone: zone,
      }),
      reason: "",
      cancelled: current?.cancelled ?? false,
      ...(current && !current.all_day
        ? {
            start: toWall(current.start, current.timezone),
            end: toWall(current.end, current.timezone),
          }
        : {}),
      frequency: series?.rule.frequency ?? "none",
      interval: series?.rule.interval ?? 1,
      weekdays: series?.rule.weekdays ?? [1, 3, 5],
      ending: series?.rule.until ? "until" : "count",
      count: series?.rule.count || 10,
      until: series?.rule.until ?? "",
      kind: series?.kind ?? "event",
      scope: "this",
    },
    fields: ["title", "area_id", "quest_id", "all_day", "start", "end", "timezone", "reason"],
    save: async (values) => {
      const input = calendarInput.parse(values);
      if (!input.all_day) {
        try {
          input.start = fromWall(input.start, input.timezone);
          input.end = fromWall(input.end, input.timezone);
        } catch {
          throw new ApiError(
            422,
            "VALIDATION_ERROR",
            "Выбранное местное время не существует. Выберите другой час.",
            undefined,
            { start: "Проверьте время и переход часов" },
          );
        }
      }
      const rule = {
        frequency: values.frequency === "none" ? ("daily" as const) : values.frequency,
        interval: values.interval,
        weekdays: values.frequency === "weekly" ? values.weekdays : [],
        count: values.ending === "count" ? values.count : 0,
        until: values.ending === "until" ? values.until : null,
      };
      if (current && series && values.scope !== "this")
        await changeSeries(
          series,
          {
            action: values.scope === "end" ? "end" : "split",
            occurrence_key: current.occurrence_key!,
            replacement:
              values.scope === "future"
                ? { event: { ...input, quest_id: null }, kind: values.kind, rule }
                : null,
            until: values.scope === "end" ? values.until : null,
            reason: values.reason,
          },
          key.current,
        );
      else if (current)
        await editCalendar(current, input, values.reason, key.current, values.cancelled);
      else if (values.frequency !== "none")
        await createSeries(
          {
            event: { ...input, quest_id: null },
            kind: values.kind,
            rule: {
              frequency: values.frequency,
              interval: values.interval,
              weekdays: values.frequency === "weekly" ? values.weekdays : [],
              count: values.ending === "count" ? values.count : 0,
              until: values.ending === "until" ? values.until : null,
            },
          },
          key.current,
        );
      else await createEvent(input, key.current);
      key.current = crypto.randomUUID();
      onSaved();
    },
  });
  const { form } = editor;
  const errors = form.formState.errors;
  return (
    <Form saving={editor.saving} error={editor.error} onSubmit={editor.onSubmit}>
      {series && (
        <Select label="Изменить повторения" {...form.register("scope")}>
          <option value="this">Только это повторение</option>
          <option value="future">Это и будущие</option>
          <option value="end">Последний день серии</option>
        </Select>
      )}
      {series && form.watch("scope") === "end" && (
        <Input label="Новый последний день серии" type="date" {...form.register("until")} />
      )}
      <Input label="Название события" {...form.register("title")} error={errors.title?.message} />
      <Select
        label="Направление события"
        value={form.watch("area_id") ?? ""}
        onChange={(e) => form.setValue("area_id", e.target.value || null, { shouldDirty: true })}
      >
        <option value="">Без направления</option>
        {areas.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
      </Select>
      {!current && (
        <QuestPicker
          label="Связанная задача"
          value={form.watch("quest_id") ?? ""}
          onChange={(v) => form.setValue("quest_id", v || null, { shouldDirty: true })}
        />
      )}
      <label className="check-row">
        <input
          type="checkbox"
          disabled={current?.source === "quest"}
          {...form.register("all_day", {
            onChange: () => {
              form.setValue("start", "", { shouldDirty: true });
              form.setValue("end", "", { shouldDirty: true });
            },
          })}
        />
        Без времени
      </label>
      <Input
        label="Начало события"
        type={form.watch("all_day") ? "date" : "datetime-local"}
        hint={
          form.watch("all_day")
            ? "Дата без часового сдвига"
            : "Местное время: " + form.getValues("timezone")
        }
        {...form.register("start")}
        error={errors.start?.message}
      />
      <Input
        label="Конец события"
        disabled={current?.source === "quest"}
        type={form.watch("all_day") ? "date" : "datetime-local"}
        hint="Конец не включён: для одного дня выберите следующий день."
        {...form.register("end")}
        error={errors.end?.message}
      />
      {current && (
        <Input
          label="Причина изменения"
          {...form.register("reason")}
          error={errors.reason?.message}
        />
      )}
      {current && current.source !== "quest" && (
        <label className="check-row">
          <input type="checkbox" {...form.register("cancelled")} />
          Отменить событие
        </label>
      )}
      {(!current || (series && form.watch("scope") === "future")) && (
        <>
          <Select label="Повторение" {...form.register("frequency")}>
            <option value="none">Не повторять</option>
            <option value="daily">Ежедневно</option>
            <option value="weekly">По дням недели</option>
            <option value="monthly">Ежемесячно по числу</option>
          </Select>
          {form.watch("frequency") !== "none" && (
            <>
              <Select label="Что повторять" {...form.register("kind")}>
                <option value="event">Событие</option>
                <option value="task">Отдельную задачу каждый раз</option>
              </Select>
              <Input
                label="Интервал повторения"
                type="number"
                {...form.register("interval", { valueAsNumber: true })}
              />
              {form.watch("frequency") === "weekly" && (
                <fieldset>
                  <legend>Дни недели</legend>
                  {["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"].map((d, i) => (
                    <label key={d} className="check-row">
                      <input
                        type="checkbox"
                        checked={form.watch("weekdays").includes(i + 1)}
                        onChange={(e) =>
                          form.setValue(
                            "weekdays",
                            e.target.checked
                              ? [...form.getValues("weekdays"), i + 1]
                              : form.getValues("weekdays").filter((v) => v !== i + 1),
                            { shouldDirty: true },
                          )
                        }
                      />
                      {d}
                    </label>
                  ))}
                </fieldset>
              )}
              <Select label="Конец повторения" {...form.register("ending")}>
                <option value="count">По числу повторений</option>
                <option value="until">По дате включительно</option>
              </Select>
              {form.watch("ending") === "count" ? (
                <Input
                  label="Число повторений"
                  type="number"
                  {...form.register("count", { valueAsNumber: true })}
                />
              ) : (
                <Input label="Последний день серии" type="date" {...form.register("until")} />
              )}
              <p className="ui-hint">
                На 31-е число короткий месяц пропускается. Время в пропущенном часе при переводе
                часов пропускается; двойное время означает более ранний момент. Серия ограничена 10
                годами. Часовой пояс серии: {zone}.
              </p>
            </>
          )}
        </>
      )}
      <Button type="submit">Сохранить событие</Button>
    </Form>
  );
}
function ExistingEventForm(props: {
  zone: string;
  areas: { id: string; name: string }[];
  current: CalendarEvent;
  onSaved: () => void;
}) {
  const account = useAuth().session?.account.id;
  const series = useQuery({
    queryKey: [
      "account",
      account,
      "calendar-series",
      props.current.series_id,
      props.current.series_version,
    ],
    enabled: !!props.current.series_id,
    queryFn: ({ signal }) => getSeries(props.current.series_id!, signal),
  });
  if (props.current.series_id && series.isPending)
    return <p role="status">Загружаем правило серии…</p>;
  if (props.current.series_id && !series.data)
    return (
      <p role="alert">
        Не удалось загрузить правило.{" "}
        <Button onClick={() => void series.refetch()}>Повторить</Button>
      </p>
    );
  return <EventForm {...props} series={series.data} />;
}
