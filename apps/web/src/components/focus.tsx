"use client";
import { useEffect, useRef, useState, useDeferredValue } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { ApiError } from "@/lib/api/client";
import {
  useFocus,
  startFocus,
  actOnFocus,
  broadcastFocus,
  type FocusSession,
  type FocusAction,
} from "@/lib/focus";
import { projectFocus, duration } from "@/lib/focus-clock";
import { listQuests, emptyFilters, questLabels } from "@/lib/quests";
import { listGoals, emptyGoalFilters } from "@/lib/goals";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/field";
import { Form } from "./ui/form";
import { FocusHistory } from "./focus-history";
import { FocusReview } from "./focus-review";
import { Dialog } from "./ui/dialog";

const labels: Record<string, string> = {
  running: "Таймер идёт",
  paused: "Пауза",
  awaiting_confirmation: "Ожидает подтверждения результата",
  finished: "Результат подтверждён",
  cancelled: "Отменена",
};
function useMonotonicTick() {
  const [mono, setMono] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => setMono(performance.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return mono;
}
function useIntervalBell(f: FocusSession | undefined, allowed: boolean) {
  const previous = useRef<{ id: string; state: string } | undefined>(undefined);
  useEffect(() => {
    const old = previous.current;
    previous.current = f ? { id: f.id, state: f.state } : undefined;
    if (
      !f ||
      !allowed ||
      !f.sound_enabled ||
      old?.id !== f.id ||
      old.state !== "running" ||
      f.state !== "paused" ||
      !f.phase.endsWith("_complete") ||
      document.visibilityState !== "visible"
    )
      return;
    const last = f.segments.at(-1);
    if (!last?.ended_at) return;
    try {
      const audio = new AudioContext(),
        osc = audio.createOscillator(),
        gain = audio.createGain();
      gain.gain.value = 0.025;
      osc.frequency.value = 660;
      osc.connect(gain);
      gain.connect(audio.destination);
      osc.start();
      osc.stop(audio.currentTime + 0.12);
      osc.onended = () => void audio.close();
    } catch {
      /* The visual boundary remains available if the browser blocks audio. */
    }
  }, [f, allowed]);
}
export function Focus() {
  const auth = useAuth(),
    cache = useQueryClient(),
    q = useFocus(),
    settings = useSettings();
  const [notice, setNotice] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [cancel, setCancel] = useState(false);
  const lock = useRef(false),
    key = useRef({ body: "", value: "" }),
    mono = useMonotonicTick(),
    boundary = useRef("");
  const refetch = q.refetch;
  const s = q.data?.session ?? undefined,
    clock =
      s && q.data
        ? projectFocus(s, q.data.sync.serverMs, mono - q.data.sync.receivedMono)
        : undefined;
  const changed = () => {
    void cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
    if (auth.session) broadcastFocus(auth.session.account.id);
  };
  useIntervalBell(s, !!settings.data?.personalization.sound);
  useEffect(() => {
    if (!clock?.boundaryPassed || !s) return;
    const version = s.id + ":" + s.version;
    if (boundary.current === version) return;
    boundary.current = version;
    void refetch();
  }, [clock?.boundaryPassed, s, refetch]);
  async function action(a: FocusAction) {
    if (!s || lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    const body = JSON.stringify({ id: s.id, version: s.version, a });
    if (key.current.body !== body) key.current = { body, value: crypto.randomUUID() };
    try {
      await actOnFocus(s, a, "", key.current.value);
      setCancel(false);
      changed();
    } catch (e) {
      setError(
        e instanceof ApiError ? e.message : "Не удалось сохранить действие. Повтор безопасен.",
      );
      void refetch();
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  if (!auth.session)
    return (
      <Card>
        <p>Войдите, чтобы восстановить свой таймер.</p>
        <SafeLink href="/auth/login">Войти</SafeLink>
      </Card>
    );
  if (!q.data)
    return (
      <Card>
        <p role={q.isError ? "alert" : "status"}>
          {q.isError ? "Не удалось загрузить таймер." : "Сверяем время с сервером…"}
        </p>
        <Button onClick={() => void refetch()}>Повторить загрузку таймера</Button>
      </Card>
    );
  return (
    <>
      <Card>
        <h2>Один шаг за раз</h2>
        <p>
          Паузы и перерывы исключаются из рабочего времени. До 4 часов работы и 8 часов от старта.
          После завершения подтвердите итог; задача остаётся под вашим управлением.
        </p>
        <p>За таймер отдельный XP не начисляется.</p>
      </Card>
      {q.isError && (
        <p role="alert">
          Связь с сервером прервана. Показано последнее сохранённое состояние; счётчик ограничен
          пределом сессии.
        </p>
      )}
      {Math.abs(q.data.sync.skewMs) > 5000 ? (
        <p role="status">
          Часы устройства отличаются от сервера примерно на{" "}
          {Math.round(Math.abs(q.data.sync.skewMs) / 1000)} сек. Таймер использует серверное время.
        </p>
      ) : (
        <p className="ui-hint">
          Время сверено с сервером. Погрешность связи: до{" "}
          {Math.ceil(q.data.sync.uncertaintyMs / 1000)} сек.
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
      {s && clock ? (
        <Card>
          <h2>{s.title}</h2>
          <p role="status">
            {labels[s.state]}
            {s.phase === "break" ? " · Перерыв" : ""}
          </p>
          <p>
            Работа:{" "}
            <output aria-label="Активное рабочее время" aria-live="off">
              {duration(clock.active)}
            </output>{" "}
            · перерывы {duration(clock.rest)}
          </p>
          {s.state === "running" && (
            <p>
              До границы интервала:{" "}
              <output aria-label="Осталось до границы интервала" aria-live="off">
                {duration(clock.remaining)}
              </output>
            </p>
          )}
          {s.state === "paused" && s.phase_remaining_seconds !== null && (
            <p>Осталось после паузы: {duration(s.phase_remaining_seconds)}</p>
          )}
          {s.phase === "work_complete" && (
            <p>Рабочий интервал завершён. Можно закончить или продолжить по своему выбору.</p>
          )}
          {s.phase === "break_complete" && (
            <p>Перерыв завершён. Следующий рабочий интервал начнётся по вашему выбору.</p>
          )}
          {clock.boundaryPassed && <p>Интервал достиг границы. Сверяем состояние с сервером…</p>}
          <p>
            Цикл {s.cycle} ·{" "}
            {s.mode === "pomodoro"
              ? `Pomodoro ${s.work_minutes}/${s.break_minutes} мин.`
              : s.mode === "quick"
                ? "Короткий старт: 5 минут"
                : "Свободная сессия"}
          </p>
          {s.quest_id && (
            <SafeLink href={"/app/quests/" + s.quest_id}>Открыть связанную задачу</SafeLink>
          )}
          {s.goal_id && (
            <SafeLink href={"/app/goals/" + s.goal_id}>Открыть связанную цель</SafeLink>
          )}
          <div className="auth-actions">
            {s.state === "running" && (
              <Button disabled={busy || clock.boundaryPassed} onClick={() => void action("pause")}>
                Пауза
              </Button>
            )}
            {s.state === "paused" && (s.phase === "work" || s.phase === "break") && (
              <Button disabled={busy} onClick={() => void action("resume")}>
                Продолжить таймер
              </Button>
            )}
            {s.state === "paused" && s.phase === "work_complete" && s.mode === "pomodoro" && (
              <Button disabled={busy} onClick={() => void action("break")}>
                Начать перерыв
              </Button>
            )}
            {s.state === "paused" && s.phase.endsWith("_complete") && s.mode === "pomodoro" && (
              <Button disabled={busy} onClick={() => void action("next")}>
                Следующий рабочий интервал
              </Button>
            )}
            {s.state === "paused" && s.phase === "work_complete" && s.mode === "quick" && (
              <Button disabled={busy} onClick={() => void action("extend")}>
                Продолжить ещё пять минут
              </Button>
            )}
            {(s.state === "running" || s.state === "paused") && (
              <Button disabled={busy || clock.boundaryPassed} onClick={() => void action("finish")}>
                Завершить и подвести итог
              </Button>
            )}
            <Button disabled={busy} variant="secondary" onClick={() => setCancel(true)}>
              Отменить сессию
            </Button>
            <Button disabled={busy} variant="secondary" onClick={() => void refetch()}>
              Сверить с сервером
            </Button>
          </div>
          {s.state === "awaiting_confirmation" && (
            <FocusReview
              key={s.id}
              session={s}
              saved={() => {
                setNotice("Итог сохранён. Статус задачи остался под вашим управлением.");
                changed();
              }}
            />
          )}
        </Card>
      ) : (
        <Card>
          <h2>Новая сессия</h2>
          <FocusStartForm changed={changed} />
        </Card>
      )}
      <FocusHistory serverNow={q.data.server_now} />
      <Dialog
        title="Отмена фокус-сессии"
        description="Интервалы останутся в истории. Время не войдёт в подтверждённый итог задачи."
        open={cancel}
        onOpenChange={(open) => {
          if (!busy) setCancel(open);
        }}
      >
        <Button disabled={busy} variant="danger" onClick={() => void action("cancel")}>
          Подтвердить отмену сессии
        </Button>
      </Dialog>
    </>
  );
}
const source = z
  .string()
  .refine((v) => v === "" || z.uuid().safeParse(v).success, "Выберите запись");
const startSchema = z.object({
  mode: z.enum(["free", "pomodoro", "quick"]),
  title: z.string().trim().max(200),
  quest_id: source,
  goal_id: source,
  area_id: source,
  work_minutes: z.number().int().min(5).max(120),
  break_minutes: z.number().int().min(1).max(60),
  sound_enabled: z.boolean(),
});
function FocusStartForm({ changed }: { changed: () => void }) {
  const auth = useAuth(),
    settings = useSettings(),
    keys = useRef({ body: "", op: "", key: "" });
  const [taskSearch, setTaskSearch] = useState(""),
    [goalSearch, setGoalSearch] = useState(""),
    [taskPage, setTaskPage] = useState(1),
    [goalPage, setGoalPage] = useState(1),
    taskTerm = useDeferredValue(taskSearch),
    goalTerm = useDeferredValue(goalSearch);
  const tasks = useQuery({
      queryKey: ["account", auth.session?.account.id, "focus-tasks", taskTerm, taskPage],
      queryFn: ({ signal }) => listQuests(taskPage, signal, { ...emptyFilters, search: taskTerm }),
    }),
    goals = useQuery({
      queryKey: ["account", auth.session?.account.id, "focus-goals", goalTerm, goalPage],
      queryFn: ({ signal }) =>
        listGoals(goalPage, { ...emptyGoalFilters, search: goalTerm }, signal),
    }),
    areas = useQuery({
      queryKey: ["account", auth.session?.account.id, "quest-labels"],
      queryFn: ({ signal }) => questLabels(signal),
    });
  const flow = useValidatedForm<z.infer<typeof startSchema>>({
    schema: startSchema,
    defaultValues: {
      mode: "free",
      title: "",
      quest_id: "",
      goal_id: "",
      area_id: "",
      work_minutes: 25,
      break_minutes: 5,
      sound_enabled: false,
    },
    fields: [
      "title",
      "mode",
      "quest_id",
      "goal_id",
      "area_id",
      "work_minutes",
      "break_minutes",
      "sound_enabled",
    ],
    save: async (v) => {
      const body = JSON.stringify(v);
      if (keys.current.body !== body)
        keys.current = { body, op: crypto.randomUUID(), key: crypto.randomUUID() };
      try {
        await startFocus(
          {
            ...v,
            operation_id: keys.current.op,
            quest_id: v.quest_id || null,
            goal_id: v.goal_id || null,
            area_id: v.area_id || null,
            work_minutes: v.mode === "quick" ? 5 : v.work_minutes,
            sound_enabled: v.sound_enabled && !!settings.data?.personalization.sound,
          },
          keys.current.key,
        );
      } finally {
        changed();
      }
    },
  });
  const selectedTask = useWatch({ control: flow.form.control, name: "quest_id" }),
    selectedGoal = useWatch({ control: flow.form.control, name: "goal_id" });
  const mode = useWatch({ control: flow.form.control, name: "mode" }),
    f = flow.form,
    errors = f.formState.errors;
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Select label="Режим фокуса" {...f.register("mode")}>
        <option value="free">Свободная сессия</option>
        <option value="pomodoro">Pomodoro</option>
        <option value="quick">Начать с пяти минут</option>
      </Select>
      <Input
        label="Название сессии"
        hint="Необязательно. При выборе задачи используем её название."
        {...f.register("title")}
        error={errors.title?.message}
      />
      {mode === "pomodoro" && (
        <>
          <Input
            label="Рабочий интервал, минут"
            type="number"
            min={5}
            max={120}
            {...f.register("work_minutes", { valueAsNumber: true })}
            error={errors.work_minutes?.message}
          />
          <Input
            label="Перерыв, минут"
            type="number"
            min={1}
            max={60}
            {...f.register("break_minutes", { valueAsNumber: true })}
            error={errors.break_minutes?.message}
          />
        </>
      )}
      <Input
        label="Поиск задачи для фокуса"
        value={taskSearch}
        onChange={(e) => {
          setTaskSearch(e.target.value);
          setTaskPage(1);
        }}
      />
      <Select
        label="Задача для фокуса"
        {...f.register("quest_id")}
        error={errors.quest_id?.message}
      >
        <option value="">Без задачи</option>
        {selectedTask && !tasks.data?.items.some((t) => t.id === selectedTask) && (
          <option value={selectedTask}>Ранее выбранная задача</option>
        )}
        {tasks.data?.items
          .filter((q) => !["completed", "failed", "skipped"].includes(q.status) && !q.archived_at)
          .map((q) => (
            <option key={q.id} value={q.id}>
              {q.title}
            </option>
          ))}
      </Select>
      <div className="auth-actions">
        <Button disabled={taskPage === 1} onClick={() => setTaskPage((p) => p - 1)}>
          Предыдущие задачи
        </Button>
        <Button
          disabled={!tasks.data || taskPage * 25 >= tasks.data.total}
          onClick={() => setTaskPage((p) => p + 1)}
        >
          Следующие задачи
        </Button>
      </div>
      <Input
        label="Поиск цели для фокуса"
        value={goalSearch}
        onChange={(e) => {
          setGoalSearch(e.target.value);
          setGoalPage(1);
        }}
      />
      <Select label="Цель для фокуса" {...f.register("goal_id")} error={errors.goal_id?.message}>
        <option value="">Без цели</option>
        {selectedGoal && !goals.data?.items.some((g) => g.id === selectedGoal) && (
          <option value={selectedGoal}>Ранее выбранная цель</option>
        )}
        {goals.data?.items
          .filter((g) => !g.archived_at && !["achieved", "cancelled"].includes(g.state))
          .map((g) => (
            <option key={g.id} value={g.id}>
              {g.title}
            </option>
          ))}
      </Select>
      <div className="auth-actions">
        <Button disabled={goalPage === 1} onClick={() => setGoalPage((p) => p - 1)}>
          Предыдущие цели
        </Button>
        <Button
          disabled={!goals.data || goalPage * 25 >= goals.data.total}
          onClick={() => setGoalPage((p) => p + 1)}
        >
          Следующие цели
        </Button>
      </div>
      <Select label="Направление фокуса" {...f.register("area_id")} error={errors.area_id?.message}>
        <option value="">Из задачи или цели</option>
        {areas.data?.areas
          .filter((a) => !a.archived)
          .map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
      </Select>
      {(tasks.isError || goals.isError || areas.isError) && (
        <p role="alert">
          Не удалось загрузить связи.{" "}
          <Button
            onClick={() => {
              void tasks.refetch();
              void goals.refetch();
              void areas.refetch();
            }}
          >
            Повторить загрузку связей
          </Button>
        </p>
      )}
      <label>
        <input
          type="checkbox"
          {...f.register("sound_enabled")}
          disabled={!settings.data?.personalization.sound}
        />{" "}
        Звук окончания интервала
      </label>
      {!settings.data?.personalization.sound && (
        <p className="ui-hint">
          Звук выключен в профиле.{" "}
          <SafeLink href="/app/settings/appearance">Настроить звук</SafeLink>
        </p>
      )}
      <Button type="submit">Начать фокус-сессию</Button>
    </Form>
  );
}
export function FocusSummary() {
  const q = useFocus();
  return (
    <>
      <h2>Фокус</h2>
      {q.data?.session ? (
        <p>
          {q.data.session.title} · {labels[q.data.session.state]} · работа{" "}
          {duration(q.data.session.active_seconds)}
        </p>
      ) : (
        <p>Выберите один шаг и подходящий режим.</p>
      )}
      {q.isError && <p role="alert">Не удалось сверить таймер.</p>}
      <SafeLink href="/app/focus">
        {q.data?.session ? "Вернуться к таймеру" : "Открыть фокус"}
      </SafeLink>
    </>
  );
}
