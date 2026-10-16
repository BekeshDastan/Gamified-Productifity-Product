"use client";
import { useState } from "react";
import { LockKeyhole, Users, Activity, Award, Search, ShieldCheck } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import {
  useSettings,
  saveSettings,
  getSettings,
  preferencesSchema,
  type Settings,
} from "@/lib/settings";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { ErrorState, LoadingState } from "./ui/states";
import { Form } from "./ui/form";
import { ChoiceCards } from "./ui/choice-cards";
import { Button } from "./ui/button";
export function PrivacySettings() {
  const auth = useAuth();
  const query = useSettings();
  if (auth.session === undefined || (auth.session && query.isPending))
    return <LoadingState label="Загружаем приватность…" />;
  if (!auth.session)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/settings/privacy">
        Войти для настройки приватности
      </SafeLink>
    );
  if (!query.data) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return <PrivacyForm initial={query.data} ownerId={auth.session.account.id} />;
}
function PrivacyForm({ initial, ownerId }: { initial: Settings; ownerId: string }) {
  const cache = useQueryClient();
  const [version, setVersion] = useState(initial.version);
  const [saved, setSaved] = useState(false);
  const flow = useValidatedForm({
    schema: preferencesSchema,
    defaultValues: preferencesSchema.parse(initial),
    fields: [
      "privacy.profile_visibility",
      "privacy.share_activity",
      "privacy.share_achievements",
      "privacy.discoverable",
    ],
    save: async (values) => {
      setSaved(false);
      const next = await saveSettings(values, version);
      setVersion(next.version);
      cache.setQueryData(["account", ownerId, "settings"], next);
      setSaved(true);
      return preferencesSchema.parse(next);
    },
  });
  const privacy = flow.form.watch("privacy");
  return (
    <Card className="settings-card privacy-settings">
      <div className="support-card-heading">
        <span className="support-icon" data-tone="mint">
          <ShieldCheck aria-hidden="true" />
        </span>
        <div>
          <p className="eyebrow">ЛИЧНОЕ ПРОСТРАНСТВО</p>
          <h2>Ваши данные — ваше решение</h2>
        </div>
      </div>
      <div className="privacy-status">
        <LockKeyhole size={20} aria-hidden="true" />
        <div>
          <strong>Личные записи остаются вашими</strong>
          <p>Задачи, цели, дневник и история остаются личными.</p>
        </div>
      </div>
      <p className="ui-hint">
        Социальные функции ещё не подключены. Здесь можно заранее выбрать предпочтения; публикация
        потребует отдельного действия.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <ChoiceCards
          label="Предпочтительная видимость профиля"
          value={privacy.profile_visibility}
          onChange={(value) =>
            flow.form.setValue("privacy.profile_visibility", value, { shouldDirty: true })
          }
          options={[
            {
              value: "private",
              label: "Только я",
              description: "Сохранить профиль личным",
              icon: LockKeyhole,
              tone: "mint",
            },
            {
              value: "friends",
              label: "Подтверждённые друзья",
              description: "Псевдоним для взаимных приглашений",
              icon: Users,
              tone: "blue",
            },
          ]}
        />
        <div className="privacy-switches">
          {(
            [
              [
                "share_activity",
                "Разрешать делиться активностью с друзьями",
                "Активность",
                "Каждая сводка включается отдельно в карточке друга",
                Activity,
                "blue",
              ],
              [
                "share_achievements",
                "Разрешать делиться достижениями с друзьями",
                "Достижения",
                "Публикация каждого достижения — отдельное действие",
                Award,
                "gold",
              ],
              [
                "discoverable",
                "Разрешать находить мой профиль по нику",
                "Поиск по нику",
                "Дружба пока доступна только по личному коду",
                Search,
                "violet",
              ],
            ] as const
          ).map(([field, label, title, description, Icon, tone]) => (
            <label className="preference-switch" key={field}>
              <span className="support-icon" data-tone={tone}>
                <Icon size={20} aria-hidden="true" />
              </span>
              <span>
                <strong>{title}</strong>
                <small>{description}</small>
              </span>
              <input
                type="checkbox"
                aria-label={label}
                {...flow.form.register(`privacy.${field}`)}
              />
              <span className="switch-track" aria-hidden="true" />
            </label>
          ))}
        </div>
        <p className="ui-hint">
          Email не будет показан другим пользователям. Задачи, цели, дневник и история остаются
          личными.
        </p>
        <Button type="submit">Сохранить приватность</Button>
      </Form>
      {saved && !flow.form.formState.isDirty && (
        <p className="preference-saved" role="status">
          Приватность сохранена.
        </p>
      )}
      {flow.form.formState.isDirty && (
        <p className="ui-hint" role="status">
          Есть несохранённые изменения.
        </p>
      )}
      {flow.error && (
        <Button
          variant="ghost"
          onClick={() =>
            void getSettings()
              .then((next) => {
                flow.form.reset(preferencesSchema.parse(next));
                setVersion(next.version);
                cache.setQueryData(["account", ownerId, "settings"], next);
                setSaved(false);
              })
              .catch(() =>
                flow.form.setError("root.server", { message: "Не удалось обновить настройки." }),
              )
          }
        >
          Обновить данные
        </Button>
      )}
    </Card>
  );
}
