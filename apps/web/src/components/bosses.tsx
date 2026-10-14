"use client";
import { useRef, useEffect, useState, useDeferredValue } from "react";
import { z } from "zod";
import { useQuery, useQueryClient, useInfiniteQuery } from "@tanstack/react-query";
import { useWatch } from "react-hook-form";
import { useAuth } from "@/lib/auth/provider";
import { listGoals, emptyGoalFilters } from "@/lib/goals";
import {
  useBosses,
  useMonthlyBoss,
  getMonthlyBoss,
  createBoss,
  actOnBoss,
  selectMonthlyBoss,
  getMonthlyHistory,
  type Boss,
  type MonthlyBoss,
} from "@/lib/bosses";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Card, Progress } from "./ui/card";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { Swords } from "lucide-react";
import { EmptyScene } from "./ui/experience";
export function Bosses() {
  const auth = useAuth(),
    query = useBosses(),
    monthly = useMonthlyBoss(),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  const [selected, setSelected] = useState<{ boss: Boss; action: "pause" | "resume" | "close" }>();
  const changed = () =>
    void cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
  if (!auth.session)
    return (
      <Card>
        <p>Бои видны только владельцу.</p>
        <SafeLink href="/auth/login">Войти</SafeLink>
      </Card>
    );
  if (!query.data || !monthly.data)
    return (
      <Card>
        <p role={query.isError || monthly.isError ? "alert" : "status"}>
          {query.isError || monthly.isError
            ? "Не удалось загрузить боссов."
            : "Загружаем ваши цели…"}
        </p>
        <Button
          onClick={() => {
            void query.refetch();
            void monthly.refetch();
          }}
        >
          Повторить
        </Button>
      </Card>
    );
  return (
    <>
      <Card>
        <h2>Новый босс по цели</h2>
        <p>
          Добавьте в цель обязательные этапы или задачи. HP отражает оставшуюся долю плана;
          дополнительного XP за бой нет.
        </p>
        <BossCreate changed={changed} />
      </Card>
      <Card>
        <h2>Главный босс месяца · {monthly.data.month}</h2>
        <p>Месяц по часовому поясу {monthly.data.timezone}. Смена сохраняется в журнале.</p>
        <MonthlyChoice current={monthly.data} bosses={query.data} changed={changed} />
      </Card>
      <div className="reward-grid">
        {query.data.map((b) => (
          <Card key={b.id} id={"boss-" + b.id} className="boss-card">
            <div className="boss-emblem" aria-hidden="true">
              <Swords size={38} />
            </div>
            <BossStatus boss={b} />
            <ol>
              {b.stages.map((s) => (
                <li key={s.id}>
                  <SafeLink href={s.source_url}>{s.title}</SafeLink> ·{" "}
                  {s.required ? "Обязательный" : "Дополнительный"} · {Math.round(s.progress)}%
                </li>
              ))}
            </ol>
            <p>
              <SafeLink href={"/app/goals/" + b.goal_id}>Цель, критерий и история этапов</SafeLink>
            </p>
            {b.status === "awaiting_confirmation" && (
              <p>
                HP исчерпано. Подтвердите критерий достижения в цели, чтобы завершить бой победой.
              </p>
            )}
            <div className="auth-actions">
              {(b.state === "active" ? ["pause", "close"] : ["resume"]).map((a) => (
                <Button
                  key={a}
                  variant="secondary"
                  onClick={() =>
                    setSelected({ boss: b, action: a as "pause" | "resume" | "close" })
                  }
                >
                  {a === "pause"
                    ? "Приостановить бой"
                    : a === "close"
                      ? "Закончить бой"
                      : "Возобновить бой"}
                </Button>
              ))}
            </div>
            {b.won_at && (
              <p className="ui-hint">
                Подтверждённая победа была записана; исправления источника сохраняются.
              </p>
            )}
          </Card>
        ))}
      </div>
      {query.data.length === 0 && (
        <Card>
          <EmptyScene
            title="Большая цель — маленькие шаги"
            description="Боссов пока нет. Выберите существующую цель и двигайтесь по её этапам."
            tone="gold"
          />
        </Card>
      )}
      <BossHistory />
      <Dialog
        title="Решение по бою"
        description="Прогресс цели сохранится. Решение и причина попадут в историю."
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && discard()) setSelected(undefined);
        }}
      >
        {selected && (
          <BossDecision
            key={selected.boss.id + selected.action}
            selected={selected}
            saved={() => {
              setSelected(undefined);
              changed();
            }}
          />
        )}
      </Dialog>
    </>
  );
}
function BossStatus({ boss: b }: { boss: Boss }) {
  const labels: Record<string, string> = {
    active: "Бой идёт",
    paused: "Бой на паузе",
    closed: "Бой завершён",
    victory: "Победа подтверждена",
    awaiting_confirmation: "Ожидает подтверждения цели",
    goal_paused: "Цель на паузе",
    goal_archived: "Цель в архиве",
  };
  return (
    <>
      <h2>{b.title}</h2>
      <p>{labels[b.status] ?? b.status}</p>
      <Progress value={b.hp} label={"HP босса " + b.title} />
      <p>
        Осталось HP: {Math.round(b.hp)} / 100 · прогресс цели {Math.round(b.progress)}%
      </p>
    </>
  );
}
function BossCreate({ changed }: { changed: () => void }) {
  const auth = useAuth(),
    [search, setSearch] = useState(""),
    [page, setPage] = useState(1),
    deferred = useDeferredValue(search),
    keyRef = useRef({ body: "", key: "" });
  const query = useQuery({
    queryKey: ["account", auth.session?.account.id, "boss-goals", page, deferred],
    queryFn: ({ signal }) => listGoals(page, { ...emptyGoalFilters, search: deferred }, signal),
    enabled: !!auth.session,
  });
  const flow = useValidatedForm({
    schema: z.object({ goal_id: z.uuid("Выберите цель") }),
    defaultValues: { goal_id: "" },
    fields: ["goal_id"],
    save: async (v) => {
      if (keyRef.current.body !== v.goal_id)
        keyRef.current = { body: v.goal_id, key: crypto.randomUUID() };
      await createBoss(v.goal_id, keyRef.current.key);
      changed();
      return { goal_id: "" };
    },
  });
  const value = useWatch({ control: flow.form.control, name: "goal_id" });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Поиск цели для босса"
        value={search}
        onChange={(e) => {
          setSearch(e.target.value);
          setPage(1);
        }}
      />
      <Select
        label="Цель босса"
        {...flow.form.register("goal_id")}
        error={flow.form.formState.errors.goal_id?.message}
      >
        <option value="">Выберите цель</option>
        {value && !query.data?.items.some((g) => g.id === value) && (
          <option value={value}>Ранее выбранная цель</option>
        )}
        {query.data?.items.map((g) => (
          <option key={g.id} value={g.id}>
            {g.title}
          </option>
        ))}
      </Select>
      {query.isError && <p role="alert">Не удалось найти цели.</p>}
      <div className="auth-actions">
        <Button variant="secondary" disabled={page === 1} onClick={() => setPage(page - 1)}>
          Предыдущие цели
        </Button>
        <Button
          variant="secondary"
          disabled={!query.data || page * 25 >= query.data.total}
          onClick={() => setPage(page + 1)}
        >
          Следующие цели
        </Button>
      </div>
      <Button type="submit">Создать босса цели</Button>
    </Form>
  );
}
function BossDecision({
  selected,
  saved,
}: {
  selected: { boss: Boss; action: "pause" | "resume" | "close" };
  saved: () => void;
}) {
  const keyRef = useRef({ body: "", key: "" });
  const flow = useValidatedForm({
    schema: z.object({ reason: z.string().trim().min(1).max(1000) }),
    defaultValues: { reason: "" },
    fields: ["reason"],
    save: async (v) => {
      if (keyRef.current.body !== v.reason)
        keyRef.current = { body: v.reason, key: crypto.randomUUID() };
      await actOnBoss(selected.boss, selected.action, v.reason, keyRef.current.key);
      saved();
    },
  });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <p>{selected.boss.title}</p>
      <Input
        label="Причина решения по бою"
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      <Button type="submit">Подтвердить решение по бою</Button>
    </Form>
  );
}
function MonthlyChoice({
  current,
  bosses,
  changed,
}: {
  current: MonthlyBoss;
  bosses: Boss[];
  changed: () => void;
}) {
  const monthRef = useRef(current.month),
    versionRef = useRef(current.version),
    keyRef = useRef({ body: "", key: "" });
  const flow = useValidatedForm({
    schema: z.object({ boss_id: z.uuid().nullable(), reason: z.string().trim().min(1).max(1000) }),
    defaultValues: { boss_id: current.boss_id, reason: "" },
    fields: ["boss_id", "reason"],
    save: async (v) => {
      const body = JSON.stringify({ v, version: versionRef.current, month: current.month });
      if (body !== keyRef.current.body) keyRef.current = { body, key: crypto.randomUUID() };
      const result = await selectMonthlyBoss(
        versionRef.current,
        monthRef.current,
        v.boss_id,
        v.reason,
        keyRef.current.key,
      );
      versionRef.current = result.version;
      changed();
    },
  });
  const value = useWatch({ control: flow.form.control, name: "boss_id" }),
    dirty = flow.form.formState.isDirty;
  useEffect(() => {
    if (!dirty && !flow.saving && !flow.error && current.version >= versionRef.current)
      versionRef.current = current.version;
  }, [current, dirty, flow.saving, flow.error]);
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <p>Сейчас: {current.boss?.title ?? "Не выбран"}</p>
      <Select
        label="Главный босс месяца"
        value={value ?? ""}
        onChange={(e) =>
          flow.form.setValue("boss_id", e.target.value || null, { shouldDirty: true })
        }
      >
        <option value="">Без главного босса</option>
        {bosses
          .filter((b) => b.state !== "closed" || b.id === current.boss_id)
          .map((b) => (
            <option key={b.id} value={b.id}>
              {b.title}
            </option>
          ))}
      </Select>
      <Input
        label="Причина выбора босса месяца"
        {...flow.form.register("reason")}
        error={flow.form.formState.errors.reason?.message}
      />
      {flow.form.formState.errors.root?.server?.type === "VERSION_CONFLICT" && (
        <Button
          onClick={() =>
            void getMonthlyBoss()
              .then((fresh) => {
                versionRef.current = fresh.version;
                monthRef.current = fresh.month;
                flow.form.clearErrors("root.server");
              })
              .catch(() =>
                flow.form.setError("root.server", {
                  type: "VERSION_CONFLICT",
                  message: "Не удалось обновить выбор. Повторите загрузку.",
                }),
              )
          }
        >
          Обновить версию выбора
        </Button>
      )}
      <Button type="submit">Сохранить босса месяца</Button>
    </Form>
  );
}
function BossHistory() {
  const auth = useAuth(),
    q = useInfiniteQuery({
      queryKey: ["account", auth.session?.account.id, "monthly-boss-history"],
      enabled: !!auth.session,
      initialPageParam: undefined as string | undefined,
      queryFn: ({ pageParam, signal }) => getMonthlyHistory(pageParam, signal),
      getNextPageParam: (last) => last.next_cursor ?? undefined,
    });
  return (
    <Card>
      <h2>История выбора по месяцам</h2>
      {q.isError && <Button onClick={() => void q.refetch()}>Повторить загрузку истории</Button>}
      <ol>
        {q.data?.pages
          .flatMap((p) => p.items)
          .map((r) => (
            <li key={r.id}>
              {r.month}: {r.title || "Без босса"} · {r.reason} · {r.timezone}
            </li>
          ))}
      </ol>
      {q.data?.pages[0]?.items.length === 0 && <p>История появится после первого выбора.</p>}
      {q.hasNextPage && (
        <Button disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          Более ранние выборы
        </Button>
      )}
    </Card>
  );
}
export function MonthlyBossSummary() {
  const q = useMonthlyBoss();
  if (!q.data) return null;
  return (
    <Card>
      <h2>Главный босс · {q.data.month}</h2>
      {q.data.boss ? (
        <>
          <BossStatus boss={q.data.boss} />
          <SafeLink href={"/app/goals/" + q.data.boss.goal_id}>Открыть цель босса</SafeLink>
        </>
      ) : (
        <p>Главный босс пока не выбран.</p>
      )}
      <SafeLink href="/app/bosses">Боссы и история выбора</SafeLink>
    </Card>
  );
}
