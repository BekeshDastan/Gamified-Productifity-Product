"use client";
import { useState } from "react";
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
import { Form } from "./ui/form";
import { Select } from "./ui/field";
import { Button } from "./ui/button";
export function PrivacySettings() {
  const auth = useAuth();
  const query = useSettings();
  if (auth.session === undefined || (auth.session && query.isPending))
    return <p role="status">Загружаем приватность…</p>;
  if (!auth.session)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/settings/privacy">
        Войти для настройки приватности
      </SafeLink>
    );
  if (!query.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить приватность.</p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
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
  return (
    <Card className="settings-card">
      <h2>Ваши данные — ваше решение</h2>
      <p className="ui-hint">
        Сейчас профиль и все записи доступны только вам. Социальные функции ещё не подключены; выбор
        ниже сохраняет предпочтения для них. Публикация потребует отдельного действия.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Select
          label="Предпочтительная видимость профиля"
          error={flow.form.formState.errors.privacy?.profile_visibility?.message}
          {...flow.form.register("privacy.profile_visibility")}
        >
          <option value="private">Только я</option>
          <option value="friends">Подтверждённые друзья, когда появится раздел</option>
        </Select>
        <label className="auth-checkbox">
          <input type="checkbox" {...flow.form.register("privacy.share_activity")} />
          Разрешать делиться активностью с друзьями
        </label>
        <label className="auth-checkbox">
          <input type="checkbox" {...flow.form.register("privacy.share_achievements")} />
          Разрешать делиться достижениями с друзьями
        </label>
        <label className="auth-checkbox">
          <input type="checkbox" {...flow.form.register("privacy.discoverable")} />
          Разрешать находить мой профиль по нику
        </label>
        <p className="ui-hint">
          Email не будет показан другим пользователям. Задачи, цели, дневник и история остаются
          личными.
        </p>
        <Button type="submit">Сохранить приватность</Button>
      </Form>
      {saved && <p role="status">Приватность сохранена.</p>}
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
