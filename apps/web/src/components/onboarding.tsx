"use client";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { QuickTask } from "./quick-task";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { useAuth } from "@/lib/auth/provider";
import { ApiError, createApiClient } from "@/lib/api/client";
import { requireAccount } from "@/lib/profile";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { ProfileEditor } from "./profile-editor";
import { ScheduleEditor } from "./schedule-editor";
import { AreasEditor } from "./areas-editor";
import { Card, Progress } from "./ui/card";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { Form } from "./ui/form";
const starters = z.object({
  goal: z.string().max(200),
  task: z.string().max(200),
  habit: z.string().max(200),
});
const schema = z.object({
  step: z.number().int().min(0).max(6),
  starters,
  completed_at: z.iso.datetime({ offset: true }).nullable(),
  version: z.number().int().positive(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
type State = z.infer<typeof schema>;
const names = [
  "Профиль",
  "Ваше время",
  "Направления",
  "Первая цель",
  "Первая задача",
  "Первая привычка",
  "Всё готово",
];
export function useOnboarding() {
  const auth = useAuth();
  const id = auth.session?.account.id;
  const query = useQuery({
    queryKey: ["account", id, "onboarding"],
    enabled: !!id,
    queryFn: async () => {
      await requireAccount();
      const r = await createApiClient().GET("/api/v1/me/onboarding");
      return schema.parse(r.data?.data);
    },
  });
  return query;
}
export function Onboarding() {
  const auth = useAuth(),
    id = auth.session?.account.id,
    query = useOnboarding();
  if (auth.session === undefined || (id && query.isPending))
    return <p role="status">Загружаем настройку…</p>;
  if (!id)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/onboarding">
        Войти для начальной настройки
      </SafeLink>
    );
  if (!query.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить прогресс.</p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  return (
    <>
      <FirstAction initial={query.data} />
      <OptionalSetup key={id} initial={query.data} ownerId={id} />
    </>
  );
}
function OptionalSetup({ initial, ownerId }: { initial: State; ownerId: string }) {
  const [open, setOpen] = useState(initial.step > 0 && !initial.completed_at);
  return (
    <details
      className="section-disclosure"
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary>Подробная настройка — по желанию</summary>
      <div className="disclosure-content">
        <Setup initial={initial} ownerId={ownerId} />
      </div>
    </details>
  );
}
async function finishSetup(state: State) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const result = await api.POST("/api/v1/me/onboarding/complete", {
      params: { header: { ...authParams().header, "If-Match": `"${state.version}"` } },
      body: {},
    });
    return schema.parse(result.data?.data);
  });
}
export function StartHint() {
  const query = useOnboarding();
  if (!query.data || query.data.completed_at) return null;
  return (
    <aside className="next-step">
      <div>
        <h2>Начните с одного небольшого дела</h2>
        <p>
          Нажмите «Добавить задачу». Когда закончите дело, отметьте его галочкой — результат
          сохранится.
        </p>
      </div>
      <SafeLink className="text-link" href="/app/onboarding">
        Первые шаги
      </SafeLink>
    </aside>
  );
}
function FirstAction({ initial }: { initial: State }) {
  const router = useRouter(),
    cache = useQueryClient(),
    auth = useAuth(),
    discard = useConfirmDiscard();
  const [created, setCreated] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const lock = useRef(false);
  async function finish() {
    if (lock.current || !discard()) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      const next = await finishSetup(initial);
      cache.setQueryData(["account", auth.session?.account.id, "onboarding"], next);
      router.push("/app");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Не удалось сохранить настройку. Попробуйте снова.",
      );
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  return (
    <Card className="first-action">
      <p className="eyebrow">ЗАПИСАТЬ → СДЕЛАТЬ → ЗАМЕТИТЬ РЕЗУЛЬТАТ</p>
      <h2>
        {initial.completed_at
          ? "Вы уже готовы к работе"
          : created
            ? "Первый шаг уже в вашем списке"
            : "Что хотите сделать сегодня?"}
      </h2>
      <p>
        {initial.completed_at
          ? "Задачи, привычки и план доступны в меню. Подробную настройку можно открыть ниже."
          : "Выберите небольшое дело: прочитать несколько страниц, написать письмо или выйти на прогулку. Всё остальное можно настроить позже."}
      </p>
      {!initial.completed_at && <QuickTask first onCreated={() => setCreated(true)} />}
      {created && (
        <p>Откройте «Сегодня» и отметьте задачу, когда выполните. Опыт начисляется за результат.</p>
      )}
      <div className="button-row">
        {initial.completed_at ? (
          <Button asChild>
            <SafeLink href="/app">Перейти к моему дню</SafeLink>
          </Button>
        ) : (
          <Button
            disabled={busy}
            variant={created ? "primary" : "secondary"}
            onClick={() => void finish()}
          >
            {busy ? "Сохраняем…" : created ? "Перейти к моему дню" : "Пропустить знакомство"}
          </Button>
        )}
      </div>
      {error && (
        <p role="alert" className="ui-error">
          {error}{" "}
          <Button
            variant="ghost"
            onClick={() =>
              void cache.invalidateQueries({
                queryKey: ["account", auth.session?.account.id, "onboarding"],
              })
            }
          >
            Обновить прогресс
          </Button>
        </p>
      )}
    </Card>
  );
}
function Setup({ initial, ownerId }: { initial: State; ownerId: string }) {
  const cache = useQueryClient();
  const [state, setState] = useState(initial);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const confirmDiscard = useConfirmDiscard();
  async function change(step: number, drafts = state.starters, complete = false) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      await requireAccount();
      const next = await withAuthLock(async () => {
        const api = await authClient();
        const params = { header: { ...authParams().header, "If-Match": `"${state.version}"` } };
        const r = complete
          ? await api.POST("/api/v1/me/onboarding/complete", { params, body: {} })
          : await api.PATCH("/api/v1/me/onboarding", { params, body: { step, starters: drafts } });
        return schema.parse(r.data?.data);
      });
      setState(next);
      cache.setQueryData(["account", ownerId, "onboarding"], next);
    } catch (error) {
      setError(error instanceof ApiError ? error.message : "Не удалось сохранить прогресс.");
      throw error;
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  const step = state.step;
  const kind = step === 3 ? "goal" : step === 4 ? "task" : "habit";
  return (
    <>
      <Card>
        <Progress label="Начальная настройка" value={state.completed_at ? 7 : step} max={7} />
        <h2>{names[step]}</h2>
        <p className="ui-hint">
          Любой шаг можно пропустить. Сохранённые настройки останутся в аккаунте.
        </p>
      </Card>
      <fieldset className="onboarding-content" disabled={busy}>
        {step === 0 && <ProfileEditor />}
        {step === 1 && <ScheduleEditor />}
        {step === 2 && <AreasEditor />}
        {step >= 3 && step <= 5 && (
          <StarterForm
            key={kind}
            kind={kind}
            value={state.starters[kind]}
            save={async (title) => {
              await change(step + 1, { ...state.starters, [kind]: title });
            }}
          />
        )}
        {step === 6 && (
          <Card>
            <h2>{state.completed_at ? "Настройка завершена" : "Ваше пространство готово"}</h2>
            <p>
              Профиль, время и направления сохранены. Стартовые черновики доступны при повторном
              открытии настройки.
            </p>
            <ul>
              {(["goal", "task", "habit"] as const).map((k, i) => (
                <li key={k}>
                  {["Цель", "Задача", "Привычка"][i]}: {state.starters[k] || "Шаг пропущен"}
                </li>
              ))}
            </ul>
            <p className="ui-hint">
              Первую задачу и цель можно открыть и дополнить в соответствующих разделах. Идея
              привычки доступна в разделе привычек для выбора нормы и расписания.
            </p>
            {!state.completed_at && (
              <Button onClick={() => void change(6, state.starters, true).catch(() => {})}>
                Завершить настройку
              </Button>
            )}
            <SafeLink href="/app" className="text-link">
              Перейти в приложение
            </SafeLink>
          </Card>
        )}
      </fieldset>
      <div className="auth-actions">
        {step > 0 && (
          <Button
            disabled={busy}
            variant="secondary"
            onClick={() => {
              if (confirmDiscard()) void change(step - 1).catch(() => {});
            }}
          >
            Назад
          </Button>
        )}
        {step < 3 && (
          <Button
            disabled={busy}
            onClick={() => {
              if (confirmDiscard()) void change(step + 1).catch(() => {});
            }}
          >
            Продолжить
          </Button>
        )}
        {step < 6 && (
          <Button
            disabled={busy}
            variant="ghost"
            onClick={() => {
              if (confirmDiscard()) void change(step + 1).catch(() => {});
            }}
          >
            Пропустить шаг
          </Button>
        )}
        {step === 6 && (
          <Button
            disabled={busy}
            variant="secondary"
            onClick={() => void change(0).catch(() => {})}
          >
            Открыть настройку снова
          </Button>
        )}
      </div>
      {busy && <p role="status">Сохраняем прогресс…</p>}
      {error && (
        <p role="alert" className="ui-error">
          {error}{" "}
          <Button
            variant="ghost"
            onClick={() =>
              void createApiClient()
                .GET("/api/v1/me/onboarding")
                .then((r) => setState(schema.parse(r.data?.data)))
                .catch(() => setError("Не удалось обновить прогресс."))
            }
          >
            Обновить прогресс
          </Button>
        </p>
      )}
    </>
  );
}
function StarterForm({
  kind,
  value,
  save,
}: {
  kind: "goal" | "task" | "habit";
  value: string;
  save: (value: string) => Promise<void>;
}) {
  const flow = useValidatedForm({
    schema: z.object({ title: z.string().max(200, "До 200 символов") }),
    defaultValues: { title: value },
    fields: ["title"],
    save: async (values) => {
      await save(values.title);
      return values;
    },
  });
  return (
    <Card className="settings-card">
      <h2>Стартовый черновик</h2>
      <p className="ui-hint">
        Сохраните идею для {kind === "goal" ? "цели" : kind === "task" ? "задачи" : "привычки"}. Это
        черновик названия; награды и расписание пока не рассчитываются.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Название черновика"
          maxLength={200}
          error={flow.form.formState.errors.title?.message}
          {...flow.form.register("title")}
        />
        <Button type="submit">Сохранить и продолжить</Button>
      </Form>
    </Card>
  );
}
