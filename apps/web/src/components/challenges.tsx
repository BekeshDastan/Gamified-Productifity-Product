"use client";
import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DateTime } from "luxon";
import { useSearchParams } from "next/navigation";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { SafeLink, useConfirmDiscard, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { questLabels } from "@/lib/quests";
import { listHabits } from "@/lib/habits";
import { listGoals, emptyGoalFilters } from "@/lib/goals";
import * as api from "@/lib/challenges";
import { Button } from "./ui/button";
import { Card, Progress } from "./ui/card";
import { Input, Select } from "./ui/field";
import { CheckinList } from "./challenge-checkins";
const metrics = {
  tasks: "Выполненные задачи",
  habits: "Полные нормы привычки",
  focus: "Рабочие минуты фокуса",
  reviews: "Вечерние разборы",
  stages: "Завершённые этапы цели",
  manual: "Самоотчёт",
};
export const units: Record<string, string> = {
  tasks: "задач",
  periods: "периодов",
  minutes: "минут",
  reviews: "разборов",
  stages: "этапов",
  count: "действий",
  pages: "страниц",
};
export function useChallengeAction() {
  const busy = useRef(false),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  return {
    pending,
    error,
    run: async (work: () => Promise<void>) => {
      if (busy.current) return;
      busy.current = true;
      setPending(true);
      setError("");
      try {
        await work();
      } catch (e) {
        setError(e instanceof Error ? e.message : "Не удалось сохранить. Повторите попытку.");
      } finally {
        busy.current = false;
        setPending(false);
      }
    },
  };
}
export function useChallengeKey() {
  const ref = useRef({ body: "", id: "" });
  return (value: unknown) => {
    const body = JSON.stringify(value);
    if (ref.current.body !== body || !ref.current.id)
      ref.current = { body, id: crypto.randomUUID() };
    return ref.current.id;
  };
}
export function Challenges() {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p role="status">Загружаем испытания…</p>;
  return (
    <Workspace
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
    />
  );
}
function Workspace({ account, timezone }: { account: string; timezone: string }) {
  const params = useSearchParams(),
    [id, setID] = useState(params.get("run_id") ?? ""),
    [after, setAfter] = useState(""),
    [creating, setCreating] = useState(false),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  const catalog = useQuery({
      queryKey: ["account", account, "challenge-catalog"],
      queryFn: ({ signal }) => api.challengeCatalog(signal),
    }),
    runs = useQuery({
      queryKey: ["account", account, "challenge-runs", after],
      queryFn: ({ signal }) => api.challengeRuns(after, signal),
    });
  const changed = () => void cache.invalidateQueries({ queryKey: ["account", account] });
  return (
    <div className="journal-layout">
      <div className="button-row">
        <Button
          onClick={() => {
            if (discard()) setCreating(!creating);
          }}
        >
          {creating ? "Закрыть создание" : "Создать испытание"}
        </Button>
        <Button variant="secondary" onClick={changed}>
          Обновить
        </Button>
      </div>
      {catalog.isError || runs.isError ? (
        <p role="alert">Не удалось загрузить испытания. Нажмите «Обновить».</p>
      ) : null}
      {creating && catalog.data && (
        <ChallengeCreate
          key={timezone}
          account={account}
          timezone={timezone}
          catalog={catalog.data.challenges}
          saved={(id) => {
            setID(id);
            setCreating(false);
            changed();
          }}
        />
      )}
      <QuestSuggestions account={account} />
      <Card>
        <h2>Мои испытания</h2>
        {runs.isPending && <p role="status">Загружаем запуски…</p>}
        {runs.data?.items.length === 0 && <p>Выберите небольшой результат и подходящий срок.</p>}
        <ul>
          {runs.data?.items.map((r) => (
            <li key={r.id}>
              <Button
                variant="ghost"
                aria-pressed={id === r.id}
                onClick={() => {
                  if (discard()) setID(r.id);
                }}
              >
                {r.rules.name}
              </Button>{" "}
              · {r.state === "closed" ? "Окончено" : "Открыто"}
            </li>
          ))}
        </ul>
        <div className="button-row">
          {after && (
            <Button variant="secondary" onClick={() => setAfter("")}>
              К новым запускам
            </Button>
          )}
          {runs.data?.next && (
            <Button variant="secondary" onClick={() => setAfter(runs.data!.next)}>
              Более ранние запуски
            </Button>
          )}
        </div>
      </Card>
      {id && <RunDetail key={id} account={account} id={id} changed={changed} />}
    </div>
  );
}
function ChallengeCreate({
  account,
  timezone,
  catalog,
  saved,
}: {
  account: string;
  timezone: string;
  catalog: api.ChallengeTemplate[];
  saved: (id: string) => void;
}) {
  const [template, setTemplate] = useState("custom"),
    [cfg, setCfg] = useState({
      metric: "tasks",
      unit: "tasks",
      kind: "cumulative",
      period: "day",
      target: 3,
      period_target: 1,
      days: 7,
      daily_cap: 20,
      allowed_misses: 0,
    } as api.ChallengeCreate["input"]),
    [name, setName] = useState(""),
    [start, setStart] = useState(DateTime.now().setZone(timezone).toISODate()!),
    [area, setArea] = useState(""),
    [goal, setGoal] = useState(""),
    [habit, setHabit] = useState(""),
    [rejoin, setRejoin] = useState(false),
    [dirty, setDirty] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(dirty);
  const labels = useQuery({
      queryKey: ["account", account, "quest-labels"],
      queryFn: ({ signal }) => questLabels(signal),
    }),
    habits = useQuery({
      queryKey: ["account", account, "challenge-habits"],
      queryFn: ({ signal }) => listHabits(false, signal),
    }),
    goals = useQuery({
      queryKey: ["account", account, "challenge-goals"],
      queryFn: ({ signal }) => listGoals(1, emptyGoalFilters, signal),
    });
  const end =
      DateTime.fromISO(start)
        .plus({ days: cfg.days - 1 })
        .toISODate() ?? "",
    steps = cfg.period === "week" ? Math.ceil(cfg.days / 7) : cfg.days,
    misses = cfg.kind === "periodic" ? Math.max(0, steps - cfg.target) : 0;
  const change = <K extends keyof typeof cfg>(field: K, value: (typeof cfg)[K]) => {
    setCfg((v) => ({ ...v, [field]: value }));
    setDirty(true);
  };
  const selected = catalog.find((t) => t.id === template);
  return (
    <Card>
      <h2>Правила нового испытания</h2>
      <form
        aria-busy={action.pending}
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const input = {
              ...cfg,
              name,
              starts_on: start,
              ends_on: end,
              area_id: area || null,
              goal_id: goal || null,
              habit_id: habit || null,
              rejoin,
              allowed_misses: misses,
            };
            api.config.parse(input);
            const operation = key(input);
            const run = await api.createChallenge(
              {
                operation_id: operation,
                template_id: template === "custom" ? null : template,
                input,
              },
              operation,
            );
            setDirty(false);
            saved(run.id);
          });
        }}
      >
        <Select
          label="Шаблон испытания"
          value={template}
          onChange={(e) => {
            const t = catalog.find((t) => t.id === e.target.value);
            setTemplate(e.target.value);
            if (t) {
              setCfg({ ...cfg, ...api.config.parse(t) });
              setName(t.name);
            }
            setArea("");
            setGoal("");
            setHabit("");
            setDirty(true);
          }}
        >
          <option value="custom">Своё испытание</option>
          {catalog.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </Select>
        {selected && (
          <>
            <p>
              {selected.description} {selected.audience}
            </p>
            <p className="ui-hint">
              {selected.conditions} {selected.safe_norm}
            </p>
          </>
        )}
        <Input
          label="Название испытания"
          value={name}
          required
          maxLength={200}
          onChange={(e) => {
            setName(e.target.value);
            setDirty(true);
          }}
        />
        <Select
          label="Источник прогресса"
          value={cfg.metric}
          onChange={(e) => {
            const metric = e.target.value as keyof typeof metrics;
            change("metric", metric);
            change(
              "unit",
              (
                {
                  tasks: "tasks",
                  habits: "periods",
                  focus: "minutes",
                  reviews: "reviews",
                  stages: "stages",
                  manual: "count",
                } as const
              )[metric],
            );
            change("daily_cap", metric === "reviews" ? 1 : metric === "focus" ? 240 : 20);
            setHabit("");
            setGoal("");
            if (metric === "reviews") setArea("");
          }}
        >
          {Object.entries(metrics).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Select>
        {cfg.metric === "manual" && (
          <Select
            label="Единица самоотчёта"
            value={cfg.unit}
            onChange={(e) => change("unit", e.target.value as "count" | "pages" | "minutes")}
          >
            <option value="count">Действия</option>
            <option value="pages">Страницы</option>
            <option value="minutes">Минуты</option>
          </Select>
        )}
        <Select
          label="Тип результата"
          value={cfg.kind}
          onChange={(e) => change("kind", e.target.value as "cumulative" | "periodic")}
        >
          <option value="cumulative">Накопить результат</option>
          <option value="periodic">Достичь нормы в нескольких периодах</option>
        </Select>
        <Input
          label="Целевой результат"
          type="number"
          min={1}
          max={100000}
          required
          value={cfg.target}
          onChange={(e) => change("target", e.target.valueAsNumber)}
        />
        {cfg.kind === "periodic" && (
          <>
            <Select
              label="Период нормы"
              value={cfg.period}
              onChange={(e) => change("period", e.target.value as "day" | "week")}
            >
              <option value="day">День</option>
              <option value="week">Неделя от начала испытания</option>
            </Select>
            <Input
              label="Норма каждого периода"
              type="number"
              min={1}
              required
              value={cfg.period_target}
              onChange={(e) => change("period_target", e.target.valueAsNumber)}
            />
          </>
        )}
        <Input
          label="Начало испытания"
          type="date"
          required
          value={start}
          onChange={(e) => {
            setStart(e.target.value);
            setDirty(true);
          }}
        />
        <Input
          label="Длительность, дней"
          type="number"
          min={1}
          max={365}
          value={cfg.days}
          required
          onChange={(e) => change("days", e.target.valueAsNumber)}
        />
        <Input
          label="Максимум зачёта за день"
          type="number"
          min={1}
          max={
            cfg.metric === "reviews"
              ? 1
              : cfg.metric === "focus" || cfg.unit === "minutes"
                ? 240
                : cfg.metric === "manual"
                  ? 1000
                  : 100
          }
          required
          value={cfg.daily_cap}
          onChange={(e) => change("daily_cap", e.target.valueAsNumber)}
        />
        {cfg.metric !== "reviews" && (
          <Select
            label="Направление испытания"
            value={area}
            onChange={(e) => {
              setArea(e.target.value);
              setDirty(true);
            }}
          >
            <option value="">Все направления</option>
            {labels.data?.areas
              .filter((a) => !a.archived)
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
          </Select>
        )}
        {["tasks", "focus", "stages"].includes(cfg.metric) && (
          <Select
            label="Цель испытания"
            required={cfg.metric === "stages"}
            value={goal}
            onChange={(e) => {
              setGoal(e.target.value);
              setDirty(true);
            }}
          >
            <option value="">{cfg.metric === "stages" ? "Выберите цель" : "Все цели"}</option>
            {goals.data?.items
              .filter((g) => !["cancelled", "achieved"].includes(g.state))
              .map((g) => (
                <option key={g.id} value={g.id}>
                  {g.title}
                </option>
              ))}
          </Select>
        )}
        {cfg.metric === "habits" && (
          <Select
            label="Привычка испытания"
            required
            value={habit}
            onChange={(e) => {
              setHabit(e.target.value);
              setDirty(true);
            }}
          >
            <option value="">Выберите привычку</option>
            {habits.data?.map((h) => (
              <option key={h.id} value={h.id}>
                {h.title}
              </option>
            ))}
          </Select>
        )}
        <label>
          <input
            type="checkbox"
            checked={rejoin}
            onChange={(e) => {
              setRejoin(e.target.checked);
              setDirty(true);
            }}
          />{" "}
          Разрешить выход и повторное вступление
        </label>
        <p>
          До {end} включительно · {timezone}. Цель: {cfg.target}{" "}
          {cfg.kind === "periodic" ? "успешных периодов" : units[cfg.unit]}; дневной предел{" "}
          {cfg.daily_cap} {units[cfg.unit]}. Допустимых пропусков: {misses}.
        </p>
        <p className="ui-hint">
          {cfg.metric === "manual"
            ? "Самоотчёт, без подтверждения реального выполнения."
            : "Учитываются уникальные записи после вступления; исправления источников меняют результат."}{" "}
          Правила фиксируются при создании. Награда при закрытии достигнутого результата — до 25 XP
          с учётом квоты. Пропуск и выход не штрафуют. Видимость — только вам.
        </p>
        {action.error && <p role="alert">{action.error}</p>}
        <Button type="submit" disabled={action.pending}>
          Создать с этими правилами
        </Button>
      </form>
    </Card>
  );
}
function RunDetail({ account, id, changed }: { account: string; id: string; changed: () => void }) {
  const run = useQuery({
      queryKey: ["account", account, "challenge-run", id],
      queryFn: ({ signal }) => api.challengeRun(id, signal),
    }),
    membership = useQuery({
      queryKey: ["account", account, "challenge-membership", id],
      queryFn: ({ signal }) => api.challengeMembership(id, signal),
    }),
    progress = useQuery({
      queryKey: ["account", account, "challenge-progress", id],
      queryFn: ({ signal }) => api.challengeProgress(id, signal),
      enabled: !!membership.data?.member,
      refetchInterval: 30000,
    });
  const [accepted, setAccepted] = useState(false),
    [reason, setReason] = useState(""),
    [decision, setDecision] = useState(false),
    [before, setBefore] = useState(""),
    action = useChallengeAction();
  const sources = useQuery({
    queryKey: ["account", account, "challenge-sources", id, before],
    queryFn: ({ signal }) => api.challengeSources(id, before, signal),
    enabled: !!membership.data?.member,
  });
  if (run.isPending || membership.isPending)
    return <p role="status">Загружаем правила и участие…</p>;
  if (!run.data || !membership.data)
    return (
      <Card>
        <p role="alert">
          Испытание недоступно. Обновите страницу; возможно, запись удалена или доступ отозван.
        </p>
      </Card>
    );
  const r = run.data,
    m = progress.data ?? membership.data.member,
    p = m?.progress;
  return (
    <Card>
      <h2>{r.rules.name}</h2>
      <h3>Зафиксированные правила</h3>
      <p>
        {metrics[r.rules.metric]} · {r.rules.target}{" "}
        {r.rules.kind === "periodic" ? "успешных периодов" : units[r.rules.unit]} ·{" "}
        {r.rules.starts_on} — {r.rules.ends_on} · {r.rules.timezone}.
      </p>
      <p>
        Дневной предел: {r.rules.daily_cap} {units[r.rules.unit]}.{" "}
        {r.rules.kind === "periodic" && (
          <>
            Норма: {r.rules.period_target} {units[r.rules.unit]} за{" "}
            {r.rules.period === "day" ? "день" : "неделю"}; допустимые пропуски:{" "}
            {r.rules.allowed_misses}.
          </>
        )}{" "}
        Повторное вступление: {r.rules.rejoin ? "разрешено" : "не разрешено"}. Только вам.
      </p>
      <p>{r.rules.source_policy}</p>
      <p className="ui-hint">
        {r.rules.confirmation === "self_report"
          ? "Самоотчёт; платформа не подтверждает реальное выполнение."
          : "Зачёт по записям приложения."}{" "}
        Награда: до {r.rules.reward_xp} XP с учётом квоты, версия {r.rules.economy_version}. Правила{" "}
        {r.rules.scoring_version}.
      </p>
      {!m || m.state === "left" ? (
        <>
          {r.state === "open" && (!m || r.rules.rejoin) && (
            <>
              <label>
                <input
                  type="checkbox"
                  checked={accepted}
                  onChange={(e) => setAccepted(e.target.checked)}
                />{" "}
                Я прочитал и принимаю эти правила
              </label>
              <Button
                disabled={!accepted || action.pending}
                onClick={() =>
                  void action.run(async () => {
                    await api.joinChallenge(r, m?.version ?? 0, crypto.randomUUID());
                    setAccepted(false);
                    changed();
                  })
                }
              >
                {m ? "Вступить снова" : "Вступить в испытание"}
              </Button>
            </>
          )}
        </>
      ) : null}
      {p && (
        <>
          <Progress value={p.percent} label="Прогресс испытания" />
          <p>
            {p.current} / {p.target} {units[p.unit] ?? p.unit}. Уникальных источников:{" "}
            {p.source_count}.
          </p>
          {m?.computed_at && (
            <p className="ui-hint">
              Пересчитано:{" "}
              {DateTime.fromISO(m.computed_at)
                .setZone(r.rules.timezone)
                .toFormat("dd.MM.yyyy HH:mm")}
            </p>
          )}
          {p.periods.length > 0 && (
            <div className="table-scroll">
              <table>
                <caption>Нормы по периодам</caption>
                <thead>
                  <tr>
                    <th>Период</th>
                    <th>Результат</th>
                    <th>Состояние</th>
                  </tr>
                </thead>
                <tbody>
                  {p.periods.map((v) => (
                    <tr key={v.start}>
                      <td>
                        {v.start} — {v.end}
                      </td>
                      <td>
                        {v.current} / {v.target} {units[r.rules.unit]}
                      </td>
                      <td>
                        {
                          {
                            open: "Открыт",
                            met: "Норма выполнена",
                            future: "Впереди",
                            missed: "Пропуск",
                          }[v.state]
                        }
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      {progress.isError && <p role="alert">Не удалось пересчитать прогресс. Нажмите «Обновить».</p>}
      {m && (
        <>
          <h3>Источники зачёта</h3>
          {sources.isError && <p role="alert">Источники недоступны. Нажмите «Обновить».</p>}
          <ul>
            {sources.data?.items.map((v) => (
              <li key={v.kind + v.key}>
                <SafeLink href={v.url}>{v.title}</SafeLink> · {v.amount} {units[r.rules.unit]} ·{" "}
                {DateTime.fromISO(v.at).setZone(r.rules.timezone).toFormat("dd.MM.yyyy HH:mm")}{" "}
                {v.confidence === "self_report" && "· самоотчёт"}
              </li>
            ))}
          </ul>
          {sources.data?.total === 0 && <p>Пока нет подходящих источников.</p>}
          <div className="button-row">
            {before && (
              <Button variant="secondary" onClick={() => setBefore("")}>
                К новым источникам
              </Button>
            )}
            {sources.data?.next && (
              <Button variant="secondary" onClick={() => setBefore(sources.data!.next)}>
                Более ранние источники
              </Button>
            )}
          </div>
        </>
      )}
      {m && r.rules.metric === "manual" && (
        <CheckinList account={account} run={r} member={m} changed={changed} />
      )}
      {m?.state === "finished" && (
        <p>
          Итог:{" "}
          {m.result === "achieved"
            ? "Цель достигнута"
            : m.result === "left"
              ? "Вы вышли из испытания"
              : "Цель не достигнута"}
          . Первый итог сохранён в истории.
        </p>
      )}
      {r.state === "open" && (
        <details>
          <summary>Управление участием</summary>
          <Input
            label="Личная причина (необязательно)"
            value={reason}
            maxLength={1000}
            onChange={(e) => setReason(e.target.value)}
          />
          <label>
            <input
              type="checkbox"
              checked={decision}
              onChange={(e) => setDecision(e.target.checked)}
            />{" "}
            Подтверждаю {m?.state === "joined" ? "выход из испытания" : "отмену запуска"}. Прогресс
            сохранится, награда за выход не начисляется.
          </label>
          <Button
            variant="secondary"
            disabled={!decision || action.pending}
            onClick={() =>
              void action.run(async () => {
                if (m?.state === "joined")
                  await api.leaveChallenge(id, m.version, reason, crypto.randomUUID());
                else await api.cancelChallenge(id, r.version, reason, crypto.randomUUID());
                setDecision(false);
                setReason("");
                changed();
              })
            }
          >
            {m?.state === "joined" ? "Выйти из испытания" : "Отменить запуск"}
          </Button>
        </details>
      )}
      {action.error && <p role="alert">{action.error}</p>}
    </Card>
  );
}
function QuestSuggestions({ account }: { account: string }) {
  const [period, setPeriod] = useState<"daily" | "weekly">("daily"),
    [minutes, setMinutes] = useState(15),
    [area, setArea] = useState(""),
    action = useChallengeAction(),
    cache = useQueryClient();
  const labels = useQuery({
      queryKey: ["account", account, "quest-labels"],
      queryFn: ({ signal }) => questLabels(signal),
    }),
    q = useQuery({
      queryKey: ["account", account, "quest-suggestions", period, minutes, area],
      queryFn: ({ signal }) => api.questSuggestions(period, minutes, area, signal),
      enabled: minutes >= 5 && minutes <= 240,
    });
  return (
    <Card>
      <h2>Квесты на день и неделю</h2>
      <p>
        Необязательные предложения из каталога. Принятие создаёт одну задачу в «Идеях»; в план её
        можно включить отдельно.
      </p>
      <Select
        label="Период предложений"
        value={period}
        onChange={(e) => setPeriod(e.target.value as "daily" | "weekly")}
      >
        <option value="daily">На день</option>
        <option value="weekly">На неделю</option>
      </Select>
      <Input
        label="Доступное время, минут"
        type="number"
        min={5}
        max={240}
        value={minutes}
        onChange={(e) => setMinutes(e.target.valueAsNumber)}
      />
      <Select
        label="Направление предложений"
        value={area}
        onChange={(e) => setArea(e.target.value)}
      >
        <option value="">Мои активные направления</option>
        {labels.data?.areas
          .filter((a) => !a.archived)
          .map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
      </Select>
      {q.isError && (
        <p role="alert">
          Не удалось загрузить предложения.{" "}
          <Button onClick={() => void q.refetch()}>Повторить</Button>
        </p>
      )}
      <ul>
        {q.data?.items.map((v) => (
          <li key={v.template.id}>
            <h3>{v.template.name}</h3>
            <p>
              {v.template.description} · около {v.template.estimated_minutes} мин.
            </p>
            <p className="ui-hint">
              {v.reason}. {v.template.safe_norm}
            </p>
            {v.quest_id ? (
              <SafeLink href={"/app/quests/" + v.quest_id}>Открыть принятую задачу</SafeLink>
            ) : (
              <Button
                variant="secondary"
                disabled={action.pending}
                onClick={() =>
                  void action.run(async () => {
                    await api.acceptQuestSuggestion(
                      {
                        period,
                        starts_on: q.data!.starts_on,
                        template_id: v.template.id,
                        available_minutes: minutes,
                        area_id: area || null,
                      },
                      crypto.randomUUID(),
                    );
                    await cache.invalidateQueries({ queryKey: ["account", account] });
                  })
                }
              >
                Принять «{v.template.name}»
              </Button>
            )}
          </li>
        ))}
      </ul>
      {q.data?.items.length === 0 && <p>Для выбранного времени пока нет предложений.</p>}
      {action.error && <p role="alert">{action.error}</p>}
    </Card>
  );
}
