"use client";
import { useEffect, useRef, useState } from "react";
import { useTheme } from "next-themes";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import {
  useSettings,
  getSettings,
  saveSettings,
  preferencesSchema,
  type Settings,
} from "@/lib/settings";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { ThemeSelect } from "./theme-select";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Form } from "./ui/form";
import { Sun, Moon, Monitor, Feather, Sparkles, Play, Volume2 } from "lucide-react";
import { ChoiceCards } from "./ui/choice-cards";
import { ChoiceChips } from "./ui/experience";

export function PersonalizationEffects() {
  const query = useSettings();
  const { setTheme } = useTheme();
  const data = query.data?.personalization;
  useEffect(() => {
    if (data) setTheme(data.theme);
  }, [data, setTheme]);
  useEffect(() => {
    if (!data) return;
    const root = document.documentElement;
    root.dataset.motion = data.motion;
    root.dataset.gameMode = data.game_mode;
    root.dataset.sound = data.sound ? "on" : "off";
    return () => {
      delete root.dataset.motion;
      delete root.dataset.gameMode;
      delete root.dataset.sound;
    };
  }, [data]);
  return null;
}
export function AccountThemeSelect() {
  const auth = useAuth();
  const query = useSettings();
  const cache = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const lock = useRef(false);
  if (!auth.session) return <ThemeSelect />;
  return (
    <div className="account-theme">
      <label className="theme-select">
        <span className="sr-only">Тема оформления</span>
        <select
          aria-label="Тема оформления"
          disabled={busy || !query.data}
          value={query.data?.personalization.theme ?? "system"}
          onChange={(event) => {
            const theme = event.target.value as "system" | "light" | "dark";
            if (lock.current) return;
            lock.current = true;
            setBusy(true);
            setError("");
            void getSettings()
              .then(async (current) => {
                const input = preferencesSchema.parse(current);
                input.personalization.theme = theme;
                return saveSettings(input, current.version);
              })
              .then((next) =>
                cache.setQueryData(["account", auth.session!.account.id, "settings"], next),
              )
              .catch(() => setError("Тема не сохранена. Повторите выбор."))
              .finally(() => {
                lock.current = false;
                setBusy(false);
              });
          }}
        >
          <option value="system">Системная тема</option>
          <option value="dark">Тёмная тема</option>
          <option value="light">Светлая тема</option>
        </select>
      </label>
      {error && (
        <span className="ui-error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}
const widgetNames = {
  today: "Сегодня",
  calendar: "Календарь",
  habits: "Привычки",
  focus: "Фокус",
  progress: "Прогресс",
};
export function AppearanceSettings() {
  const auth = useAuth();
  const query = useSettings();
  if (auth.session === undefined || (auth.session && query.isPending))
    return <p role="status">Загружаем оформление…</p>;
  if (!auth.session)
    return (
      <Card className="settings-card">
        <h2>Тема в этом браузере</h2>
        <ThemeSelect />
        <p className="ui-hint">
          <SafeLink className="text-link" href="/auth/login?next=/app/settings/appearance">
            Войдите
          </SafeLink>
          , чтобы сохранять оформление между устройствами. Интерфейс учитывает системное уменьшение
          движения.
        </p>
      </Card>
    );
  if (!query.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить оформление.</p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  return <AppearanceForm initial={query.data} ownerId={auth.session.account.id} />;
}
function AppearanceForm({ initial, ownerId }: { initial: Settings; ownerId: string }) {
  const cache = useQueryClient();
  const [version, setVersion] = useState(initial.version);
  const [saved, setSaved] = useState(false);
  const [preview, setPreview] = useState(0);
  const flow = useValidatedForm({
    schema: preferencesSchema,
    defaultValues: preferencesSchema.parse(initial),
    fields: [
      "personalization.theme",
      "personalization.motion",
      "personalization.sound",
      "personalization.game_mode",
      "personalization.widget_order",
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
  const order = flow.form.watch("personalization.widget_order");
  function move(index: number, delta: number) {
    const next = [...order];
    const item = next[index];
    const other = next[index + delta];
    if (!item || !other) return;
    next[index] = other;
    next[index + delta] = item;
    flow.form.setValue("personalization.widget_order", next, {
      shouldDirty: true,
      shouldValidate: true,
    });
  }
  return (
    <Card className="settings-card appearance-composer">
      <h2>Комфорт прежде всего</h2>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <ChoiceCards
          label="Тема аккаунта"
          value={flow.form.watch("personalization.theme")}
          options={[
            {
              value: "system",
              label: "Системная",
              description: "В ритме вашего устройства",
              icon: Monitor,
              tone: "blue",
            },
            {
              value: "light",
              label: "Светлая",
              description: "Воздух и мягкие цвета",
              icon: Sun,
              tone: "gold",
            },
            {
              value: "dark",
              label: "Тёмная",
              description: "Спокойное вечернее пространство",
              icon: Moon,
              tone: "violet",
            },
          ]}
          onChange={(value) =>
            flow.form.setValue("personalization.theme", value, { shouldDirty: true })
          }
        />
        <div
          className="appearance-preview"
          data-preview-theme={flow.form.watch("personalization.theme")}
          data-preview-motion={flow.form.watch("personalization.motion")}
        >
          <div className="preview-mini" key={preview} aria-hidden="true">
            <i />
            <span />
            <span />
            <span />
          </div>
          <div>
            <strong>Ваше пространство</strong>
            <p>Предпросмотр. Изменения применятся после сохранения.</p>
            <Button variant="secondary" onClick={() => setPreview((value) => value + 1)}>
              <Play size={15} aria-hidden="true" />
              Проверить анимацию
            </Button>
          </div>
        </div>
        <div className="settings-choice">
          <h3>Уменьшение движения</h3>
          <ChoiceChips
            label="Уменьшение движения"
            value={flow.form.watch("personalization.motion")}
            options={[
              { value: "system", label: "По настройке устройства", icon: <Sparkles size={16} /> },
              { value: "reduce", label: "Всегда уменьшать", icon: <Feather size={16} /> },
            ]}
            onChange={(value) =>
              flow.form.setValue("personalization.motion", value, { shouldDirty: true })
            }
          />
        </div>
        <div className="settings-choice">
          <h3>Игровой режим</h3>
          <ChoiceChips
            label="Игровой режим"
            value={flow.form.watch("personalization.game_mode")}
            options={[
              { value: "calm", label: "Спокойный" },
              { value: "standard", label: "Стандартный" },
            ]}
            onChange={(value) =>
              flow.form.setValue("personalization.game_mode", value, { shouldDirty: true })
            }
          />
        </div>
        <label className="auth-checkbox setting-toggle">
          <input type="checkbox" {...flow.form.register("personalization.sound")} />
          <Volume2 size={20} aria-hidden="true" />
          Разрешать звуки приложения
        </label>
        <p className="ui-hint">
          Звук окончания интервала можно включить в настройках фокус-сессии. Уменьшение движения
          отключает декоративные анимации; настройка устройства учитывается всегда.
        </p>
        <h3>Виджеты главной страницы</h3>
        <p className="ui-hint">Выберите состав и порядок вашей сводки.</p>
        <ol className="widget-order">
          {order.map((item, i) => (
            <li key={item}>
              <label className="auth-checkbox">
                <input
                  type="checkbox"
                  aria-label={`Показывать ${widgetNames[item]}`}
                  checked={
                    !(flow.form.watch("personalization.hidden_widgets") ?? []).includes(item)
                  }
                  onChange={(e) => {
                    const hidden = flow.form.getValues("personalization.hidden_widgets") ?? [];
                    flow.form.setValue(
                      "personalization.hidden_widgets",
                      e.target.checked ? hidden.filter((x) => x !== item) : [...hidden, item],
                      { shouldDirty: true },
                    );
                  }}
                />
                {widgetNames[item]}
              </label>
              <Button
                variant="ghost"
                aria-label={`Поднять ${widgetNames[item]}`}
                disabled={i === 0}
                onClick={() => move(i, -1)}
              >
                ↑
              </Button>
              <Button
                variant="ghost"
                aria-label={`Опустить ${widgetNames[item]}`}
                disabled={i === order.length - 1}
                onClick={() => move(i, 1)}
              >
                ↓
              </Button>
            </li>
          ))}
        </ol>
        <Button type="submit">Сохранить оформление</Button>
      </Form>
      {saved && <p role="status">Оформление сохранено.</p>}
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
