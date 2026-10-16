"use client";
import { useState } from "react";
import { useFieldArray } from "react-hook-form";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { accountQueryKey } from "@/lib/api/query-provider";
import {
  preferencesSchema,
  getSettings,
  saveSettings,
  useSettings,
  formatMoment,
  type Settings,
} from "@/lib/settings";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { ErrorState, LoadingState } from "./ui/states";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { CalendarDays, Coffee } from "lucide-react";
const days = ["Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"];
export function ScheduleEditor() {
  const auth = useAuth();
  const query = useSettings();
  if (auth.session === undefined || (auth.session && query.isPending))
    return <LoadingState label="Загружаем настройки…" />;
  if (!auth.session)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/settings/schedule">
        Войти для настройки расписания
      </SafeLink>
    );
  if (!query.data) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return <ScheduleForm initial={query.data} ownerId={auth.session.account.id} />;
}
function ScheduleForm({ initial, ownerId }: { initial: Settings; ownerId: string }) {
  const cache = useQueryClient();
  const [version, setVersion] = useState(initial.version);
  const [saved, setSaved] = useState(false);
  const [suggested, setSuggested] = useState("");
  const flow = useValidatedForm({
    schema: preferencesSchema,
    defaultValues: preferencesSchema.parse(initial),
    fields: ["timezone", "language", "week_start", "time_format", "work_intervals", "rest_days"],
    save: async (values) => {
      setSaved(false);
      const result = await saveSettings(values, version);
      setVersion(result.version);
      cache.setQueryData(accountQueryKey(ownerId, "settings"), result);
      setSaved(true);
      return preferencesSchema.parse(result);
    },
  });
  const intervals = useFieldArray({ control: flow.form.control, name: "work_intervals" });
  const rest = flow.form.watch("rest_days");
  const work = flow.form.watch("work_intervals");
  return (
    <Card className="settings-card schedule-composer">
      <h2>Время в вашем ритме</h2>
      <p className="ui-hint">
        Дата последнего сохранения:{" "}
        <time dateTime={initial.updated_at}>{formatMoment(initial.updated_at, initial)}</time> ·{" "}
        {initial.timezone}
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <div className="schedule-week-preview" aria-label="Предпросмотр рабочей недели">
          {days.map((day, index) => (
            <div key={day} data-rest={rest.includes(index + 1)}>
              <strong>{day.slice(0, 2)}</strong>
              {rest.includes(index + 1) ? (
                <>
                  <Coffee size={19} aria-hidden="true" />
                  <small>Отдых</small>
                </>
              ) : (
                <>
                  <CalendarDays size={19} aria-hidden="true" />
                  <small>
                    {work.filter((item) => item.day === index + 1).length
                      ? work
                          .filter((item) => item.day === index + 1)
                          .map((item) => `${item.start}–${item.end}`)
                          .join(" · ")
                      : "Свободно"}
                  </small>
                </>
              )}
            </div>
          ))}
        </div>
        <Input
          label="Часовой пояс"
          placeholder="Например, Asia/Almaty"
          hint="IANA-название. Смена применяется к будущему расписанию; история сохраняет свой пояс."
          error={flow.form.formState.errors.timezone?.message}
          {...flow.form.register("timezone")}
        />
        <Button
          variant="secondary"
          onClick={() => {
            const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
            setSuggested(zone);
            flow.form.setValue("timezone", zone, { shouldDirty: true, shouldValidate: true });
          }}
        >
          Использовать пояс устройства
        </Button>
        {suggested && (
          <p role="status">Предложен пояс: {suggested}. Нажмите «Сохранить расписание».</p>
        )}
        <Select label="Язык дат и времени" {...flow.form.register("language")}>
          <option value="ru">Русский</option>
          <option value="en">English</option>
        </Select>
        <Select
          label="Начало недели"
          {...flow.form.register("week_start", { valueAsNumber: true })}
        >
          <option value="1">Понедельник</option>
          <option value="7">Воскресенье</option>
        </Select>
        <Select label="Формат времени" {...flow.form.register("time_format")}>
          <option value="24h">24 часа</option>
          <option value="12h">12 часов</option>
        </Select>
        <fieldset className="settings-days">
          <legend>Дни отдыха</legend>
          {days.map((day, i) => (
            <label className="auth-checkbox" key={day}>
              <input
                type="checkbox"
                checked={rest.includes(i + 1)}
                onChange={(event) =>
                  flow.form.setValue(
                    "rest_days",
                    event.target.checked ? [...rest, i + 1] : rest.filter((d) => d !== i + 1),
                    { shouldDirty: true },
                  )
                }
              />
              {day}
            </label>
          ))}
        </fieldset>
        <h3>Рабочие интервалы</h3>
        {!intervals.fields.length && (
          <div className="schedule-presets">
            <p>Начните с готового ритма и подстройте его под себя.</p>
            <div className="button-row">
              {[
                { end: "12:00", label: "Утро по будням" },
                { end: "18:00", label: "Рабочий день 9–18" },
              ].map((preset) => (
                <Button
                  key={preset.end}
                  variant="secondary"
                  disabled={[1, 2, 3, 4, 5].every((day) => rest.includes(day))}
                  onClick={() =>
                    intervals.replace(
                      [1, 2, 3, 4, 5]
                        .filter((day) => !rest.includes(day))
                        .map((day) => ({ day, start: "09:00", end: preset.end })),
                    )
                  }
                >
                  {preset.label}
                </Button>
              ))}
            </div>
          </div>
        )}
        <p className="ui-hint">
          Время местное. Начало и конец — в пределах одного дня; интервалы в дни отдыха не
          сохраняются.
        </p>
        {intervals.fields.map((field, i) => (
          <div className="schedule-row" key={field.id}>
            <Select
              label={`День ${i + 1}`}
              {...flow.form.register(`work_intervals.${i}.day`, { valueAsNumber: true })}
            >
              {days.map((day, d) => (
                <option value={d + 1} key={day}>
                  {day}
                </option>
              ))}
            </Select>
            <Input
              label={`Начало ${i + 1}`}
              error={flow.form.formState.errors.work_intervals?.[i]?.start?.message}
              type="time"
              {...flow.form.register(`work_intervals.${i}.start`)}
            />
            <Input
              label={`Конец ${i + 1}`}
              error={flow.form.formState.errors.work_intervals?.[i]?.end?.message}
              type="time"
              {...flow.form.register(`work_intervals.${i}.end`)}
            />
            <Button
              variant="ghost"
              aria-label={`Удалить интервал ${i + 1}`}
              onClick={() => intervals.remove(i)}
            >
              Удалить
            </Button>
          </div>
        ))}
        {flow.form.formState.errors.work_intervals?.message && (
          <p role="alert" className="ui-error">
            {flow.form.formState.errors.work_intervals.message}
          </p>
        )}
        <Button
          variant="secondary"
          disabled={intervals.fields.length >= 28}
          onClick={() => intervals.append({ day: 1, start: "09:00", end: "18:00" })}
        >
          Добавить интервал
        </Button>
        <Button type="submit">Сохранить расписание</Button>
      </Form>
      {saved && !flow.form.formState.isDirty && <p role="status">Расписание сохранено.</p>}
      {flow.error && (
        <Button
          variant="ghost"
          onClick={() => {
            void getSettings()
              .then((fresh) => {
                flow.form.reset(preferencesSchema.parse(fresh));
                setVersion(fresh.version);
                cache.setQueryData(accountQueryKey(ownerId, "settings"), fresh);
                setSaved(false);
              })
              .catch(() =>
                flow.form.setError("root.server", { message: "Не удалось обновить настройки." }),
              );
          }}
        >
          Обновить данные
        </Button>
      )}
    </Card>
  );
}
