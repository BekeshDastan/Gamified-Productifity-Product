"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DateTime } from "luxon";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { SafeLink, useConfirmDiscard, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { listGoals, emptyGoalFilters } from "@/lib/goals";
import { challengeRuns } from "@/lib/challenges";
import { getMonthlyBoss } from "@/lib/bosses";
import * as api from "@/lib/seasons";
import { useChallengeAction, useChallengeKey, units } from "./challenges";
import { RecoveryControls } from "./season-recovery";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
const states = {
  planned: "Запланирован",
  active: "Идёт",
  completed: "Завершён",
  cancelled: "Отменён",
};
export function Seasons() {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p role="status">Загружаем сезоны…</p>;
  return (
    <Workspace
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
    />
  );
}
function Workspace({ account, timezone }: { account: string; timezone: string }) {
  const [id, setID] = useState(""),
    [before, setBefore] = useState(""),
    [archived, setArchived] = useState(false),
    [creating, setCreating] = useState(false),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  const list = useQuery({
      queryKey: ["account", account, "seasons", before],
      queryFn: ({ signal }) => api.listSeasons(before, signal),
    }),
    detail = useQuery({
      queryKey: ["account", account, "season-overview", id],
      queryFn: ({ signal }) => api.getOverview(id, signal),
      enabled: !!id,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
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
          {creating ? "Закрыть создание" : "Создать сезон"}
        </Button>
        <Button
          variant="secondary"
          onClick={() => {
            if (discard()) {
              setArchived(!archived);
              setID("");
              setBefore("");
            }
          }}
        >
          {archived ? "Все текущие сезоны" : "Архив сезонов"}
        </Button>
        <Button
          variant="secondary"
          onClick={() => {
            if (discard()) changed();
          }}
        >
          Обновить
        </Button>
      </div>
      {list.isError || detail.isError ? (
        <p role="alert">Не удалось загрузить сезоны. Обновите данные.</p>
      ) : null}
      {creating && (
        <CreateSeason
          timezone={timezone}
          saved={(v) => {
            setCreating(false);
            setID(v.id);
            changed();
          }}
        />
      )}
      <Card>
        <h2>{archived ? "Архив" : "Мои сезоны"}</h2>
        {list.isPending ? <p role="status">Загружаем…</p> : null}
        {list.data?.items
          .filter((v) => !!v.archived_at === archived)
          .map((v) => (
            <div className="button-row" key={v.id}>
              <Button
                variant="secondary"
                onClick={() => {
                  if (discard()) setID(v.id);
                }}
              >
                {v.title}
              </Button>
              <span>
                {states[v.state]} · {v.starts_on} — {v.ends_on}
                {v.primary ? " · Основной" : ""}
              </span>
            </div>
          ))}
        {list.data && !list.data.items.some((v) => !!v.archived_at === archived) && (
          <p>На этой странице сезонов пока нет.</p>
        )}
        {list.data?.next && (
          <Button
            variant="secondary"
            onClick={() => {
              if (discard()) setBefore(list.data!.next);
            }}
          >
            Следующая страница сезонов
          </Button>
        )}
        {before && (
          <Button
            variant="secondary"
            onClick={() => {
              if (discard()) setBefore("");
            }}
          >
            Сначала
          </Button>
        )}
      </Card>
      {detail.data && (
        <SeasonDetail
          key={detail.data.season.id + ":" + detail.data.season.version}
          account={account}
          value={detail.data}
          changed={changed}
        />
      )}
      <RecoveryControls account={account} timezone={timezone} changed={changed} />
    </div>
  );
}
function CreateSeason({ timezone, saved }: { timezone: string; saved: (v: api.Season) => void }) {
  const today = DateTime.now().setZone(timezone).toISODate()!,
    [input, setInput] = useState({
      title: "",
      theme: "",
      desired_result: "",
      starts_on: today,
      ends_on: DateTime.fromISO(today).plus({ days: 27 }).toISODate()!,
      primary: true,
    }),
    [operationID] = useState(() => crypto.randomUUID()),
    [ack, setAck] = useState(false),
    [dirty, setDirty] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(dirty);
  return (
    <Card>
      <h2>Новый личный сезон</h2>
      <form
        className="form-grid"
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const body = { ...api.seasonInput.parse(input), operation_id: operationID };
            const v = await api.createSeason(body, key(body));
            setDirty(false);
            saved(v);
          });
        }}
      >
        <Input
          label="Название сезона"
          value={input.title}
          maxLength={200}
          required
          onChange={(e) => {
            setInput({ ...input, title: e.target.value });
            setDirty(true);
          }}
        />
        <Input
          label="Тема"
          required
          value={input.theme}
          maxLength={200}
          onChange={(e) => {
            setInput({ ...input, theme: e.target.value });
            setDirty(true);
          }}
        />
        <Input
          label="Желаемый результат"
          required
          value={input.desired_result}
          maxLength={2000}
          onChange={(e) => {
            setInput({ ...input, desired_result: e.target.value });
            setDirty(true);
          }}
        />
        <Input
          type="date"
          label="Начало сезона"
          value={input.starts_on}
          required
          onChange={(e) => {
            setInput({ ...input, starts_on: e.target.value });
            setDirty(true);
          }}
        />
        <Input
          type="date"
          label="Конец сезона включительно"
          value={input.ends_on}
          required
          onChange={(e) => {
            setInput({ ...input, ends_on: e.target.value });
            setDirty(true);
          }}
        />
        <label>
          <input
            type="checkbox"
            checked={input.primary}
            onChange={(e) => {
              setInput({ ...input, primary: e.target.checked });
              setDirty(true);
            }}
          />{" "}
          Основной сезон
        </label>
        <p>
          Часовой пояс: {timezone}. Создание сохраняет черновой сезон. Активация выполняется
          отдельным действием. Уровень и накопленный опыт сохраняются.
        </p>
        <label>
          <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
          Подтверждаю даты и результат
        </label>
        <Button type="submit" disabled={!ack || action.pending}>
          Сохранить сезон
        </Button>
        {action.error && <p role="alert">{action.error}</p>}
      </form>
    </Card>
  );
}
function SeasonDetail({
  account,
  value,
  changed,
}: {
  account: string;
  value: api.Overview;
  changed: () => void;
}) {
  const v = value.season,
    closed = v.state === "completed" || v.state === "cancelled",
    action = useChallengeAction(),
    key = useChallengeKey(),
    [reason, setReason] = useState(""),
    [ack, setAck] = useState(false);
  useUnsavedChanges(!closed && !!reason);
  const report = useQuery({
    queryKey: ["account", account, "season-summary", v.id],
    queryFn: ({ signal }) => api.getSummary(v.id, signal),
    enabled: closed,
  });
  const doAction = (name: string, fn: (k: string) => Promise<unknown>) =>
    void action.run(async () => {
      await fn(key({ id: v.id, version: v.version, name, reason }));
      setReason("");
      setAck(false);
      changed();
    });
  return (
    <Card>
      <h2>{v.title}</h2>
      <p>
        {v.theme} · {states[v.state]} · {v.starts_on} — {v.ends_on} ({v.timezone})
      </p>
      <p>{v.desired_result}</p>
      {v.state === "planned" && (
        <Button
          disabled={action.pending}
          onClick={() => doAction("activate", (k) => api.activateSeason(v, k))}
        >
          Подтвердить активацию сезона
        </Button>
      )}
      <h3>{closed ? "Источники на момент завершения" : "Цели и испытания"}</h3>
      {[...value.goals, ...value.challenges].map((s) => (
        <div key={s.id}>
          <SafeLink href={s.url}>{s.name || "Источник недоступен"}</SafeLink>
          {value.main_goal_id === s.id ? " · Главная цель" : ""}
          <p>
            {s.value === null
              ? "Данные недоступны"
              : `${s.value} / ${s.target} ${units[s.unit] ?? s.unit}`}{" "}
            ·{" "}
            {s.state === "unavailable"
              ? "Недоступно"
              : s.result === "achieved"
                ? "Достигнуто"
                : s.percent === null
                  ? "Нет данных участия"
                  : `${Math.round(s.percent)}%`}
          </p>
          {s.stages_total > 0 && (
            <p>
              Этапы: {s.stages_completed} / {s.stages_total}
            </p>
          )}
          <details>
            <summary>Исходное состояние при привязке</summary>
            <Baseline value={s.baseline} />
          </details>
        </div>
      ))}
      {!value.goals.length && !value.challenges.length && <p>Источники ещё не выбраны.</p>}
      {value.bosses.map((b) => (
        <p key={b.month}>
          {b.month}: {b.name} · {Math.round(b.progress)}%
          {b.source_changed
            ? " · Выбор месячного босса изменён; здесь сохранена прежняя связь"
            : ""}{" "}
          · <SafeLink href="/app/bosses">Открыть босса</SafeLink>
        </p>
      ))}
      {!closed && <SeasonLinks key={v.version} account={account} value={value} changed={changed} />}
      {!closed ? (
        <>
          <h3>Завершить сезон</h3>
          <p>
            Итог зафиксирует текущие результаты. Досрочно закрытый сезон нельзя возобновить.
            Незавершённые цели сохраняются, уровень и опыт за пропуск не уменьшаются.
            Неактивированный сезон будет отменён.
          </p>
          <Input
            label="Причина завершения"
            value={reason}
            maxLength={2000}
            onChange={(e) => setReason(e.target.value)}
          />
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
            Понимаю, что итог станет неизменяемым
          </label>
          <Button
            disabled={!ack || !reason.trim() || action.pending}
            onClick={() => doAction("close", (k) => api.closeSeason(v, reason, k))}
          >
            Зафиксировать итог и завершить
          </Button>
        </>
      ) : (
        <>
          <h3>Сохранённый итог</h3>
          {report.isPending && <p role="status">Загружаем итог…</p>}
          {report.isError && <p role="alert">Итог не загрузился. Обновите данные.</p>}
          {report.data && (
            <>
              <p>
                {report.data.early ? "Досрочное завершение" : "Сезон завершён по сроку"} ·{" "}
                {report.data.at.slice(0, 10)}
              </p>
              <p>{report.data.reason}</p>
              <p>
                Достигнуто целей: {report.data.goals_achieved}; испытаний:{" "}
                {report.data.challenges_achieved}.
              </p>
              <p>
                За период: {report.data.activity.tasks} задач, {report.data.activity.habit_norms}{" "}
                полных норм привычек, {Math.floor(report.data.activity.focus_seconds / 60)} минут
                фокуса, {report.data.activity.reviews} разборов.
              </p>
              <p>
                Изменение опыта: {report.data.activity.xp_delta}; монет:{" "}
                {report.data.activity.coins_delta}. Уровень при завершении:{" "}
                {report.data.activity.level_at_close}.
              </p>
              <p>
                Снимок сохраняет результат на дату завершения. Текущее состояние доступно по ссылкам
                на источники.
              </p>
            </>
          )}
          <Button
            variant="secondary"
            disabled={action.pending}
            onClick={() => doAction("archive", (k) => api.archiveSeason(v, !v.archived_at, k))}
          >
            {v.archived_at ? "Вернуть из архива" : "Подтвердить перенос в архив"}
          </Button>
        </>
      )}
      {action.error && <p role="alert">{action.error}</p>}
    </Card>
  );
}
function Baseline({ value }: { value: Record<string, unknown> | null }) {
  if (!value) return <p>Нет исходных данных.</p>;
  return (
    <p>
      {typeof value.value === "number" ? value.value : "Нет значения"} /{" "}
      {typeof value.target === "number" ? value.target : "—"}{" "}
      {typeof value.unit === "string" ? (units[value.unit] ?? value.unit) : ""}
    </p>
  );
}
function SeasonLinks({
  account,
  value,
  changed,
}: {
  account: string;
  value: api.Overview;
  changed: () => void;
}) {
  const [goals, setGoals] = useState(value.goals.map((g) => g.id)),
    [challenges, setChallenges] = useState(value.challenges.map((c) => c.id)),
    [main, setMain] = useState(value.main_goal_id ?? ""),
    [page, setPage] = useState(1),
    [before, setBefore] = useState(""),
    [dirty, setDirty] = useState(false),
    [ack, setAck] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(dirty);
  const gs = useQuery({
      queryKey: ["account", account, "season-goal-options", page],
      queryFn: ({ signal }) => listGoals(page, emptyGoalFilters, signal),
    }),
    cs = useQuery({
      queryKey: ["account", account, "season-challenge-options", before],
      queryFn: ({ signal }) => challengeRuns(before, signal),
    }),
    monthly = useQuery({
      queryKey: ["account", account, "monthly-boss"],
      queryFn: ({ signal }) => getMonthlyBoss(signal),
    });
  function toggle(id: string, checked: boolean, kind: "goal" | "challenge") {
    setDirty(true);
    setAck(false);
    if (kind === "goal") {
      setGoals(checked ? [...goals, id] : goals.filter((g) => g !== id));
      if (main === id && !checked) setMain("");
    } else setChallenges(checked ? [...challenges, id] : challenges.filter((c) => c !== id));
  }
  const mainOptions = [
    ...value.goals.map((g) => ({ id: g.id, title: g.name })),
    ...(gs.data?.items ?? []),
  ].filter((g, i, all) => all.findIndex((a) => a.id === g.id) === i && goals.includes(g.id));
  const m = monthly.data;
  return (
    <details>
      <summary>Выбрать источники и месячного босса</summary>
      <p>
        Показатели с разными единицами остаются отдельными. Выбрано целей: {goals.length},
        испытаний: {challenges.length}.
      </p>
      {gs.isError || cs.isError || monthly.isError ? (
        <p role="alert">Не удалось загрузить варианты. Обновите данные.</p>
      ) : null}
      <fieldset>
        <legend>Цели</legend>
        {gs.data?.items.map((g) => (
          <label key={g.id}>
            <input
              type="checkbox"
              checked={goals.includes(g.id)}
              onChange={(e) => toggle(g.id, e.target.checked, "goal")}
            />{" "}
            {g.title}
          </label>
        ))}
        <div className="button-row">
          <Button variant="secondary" disabled={page <= 1} onClick={() => setPage(page - 1)}>
            Предыдущие цели
          </Button>
          <Button
            variant="secondary"
            disabled={(gs.data?.items.length ?? 0) < 25}
            onClick={() => setPage(page + 1)}
          >
            Следующие цели
          </Button>
        </div>
      </fieldset>
      <Select
        label="Главная цель"
        value={main}
        onChange={(e) => {
          setMain(e.target.value);
          setDirty(true);
          setAck(false);
        }}
      >
        <option value="">Без главной цели</option>
        {mainOptions.map((g) => (
          <option key={g.id} value={g.id}>
            {g.title}
          </option>
        ))}
      </Select>
      <fieldset>
        <legend>Испытания</legend>
        {cs.data?.items.map((c) => (
          <label key={c.id}>
            <input
              type="checkbox"
              checked={challenges.includes(c.id)}
              onChange={(e) => toggle(c.id, e.target.checked, "challenge")}
            />{" "}
            {c.rules.name}
          </label>
        ))}
        {cs.data?.next && (
          <Button variant="secondary" onClick={() => setBefore(cs.data!.next)}>
            Следующие испытания
          </Button>
        )}
        {before && (
          <Button variant="secondary" onClick={() => setBefore("")}>
            Первые испытания
          </Button>
        )}
      </fieldset>
      {[...value.goals, ...value.challenges]
        .filter((s) => s.state === "unavailable")
        .map((s) => (
          <label key={s.id}>
            <input
              type="checkbox"
              checked={goals.includes(s.id) || challenges.includes(s.id)}
              onChange={(e) =>
                toggle(
                  s.id,
                  e.target.checked,
                  value.goals.some((g) => g.id === s.id) ? "goal" : "challenge",
                )
              }
            />{" "}
            Недоступный источник — снимок при привязке сохранён
          </label>
        ))}
      <Button
        disabled={!dirty || action.pending}
        onClick={() =>
          void action.run(async () => {
            const body = { goal_ids: goals, main_goal_id: main || null, challenge_ids: challenges };
            await api.linkSeason(value.season, body, key(body));
            setDirty(false);
            changed();
          })
        }
      >
        Сохранить источники
      </Button>
      <h4>Босс месяца</h4>
      <SafeLink href="/app/bosses">Выбрать босса на странице целей</SafeLink>
      {m?.boss_id ? (
        <>
          <p>
            {m.month}: {m.boss?.title}. Сначала сохраните связанную цель в источниках сезона.
          </p>
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
            Привязать текущий выбор к этому сезону
          </label>
          <Button
            disabled={dirty || !ack || action.pending}
            onClick={() =>
              void action.run(async () => {
                const body = {
                  boss_id: m.boss_id!,
                  month: m.month,
                  source_version: m.version,
                  confirm: true,
                };
                await api.linkMonthly(value.season, body, key(body));
                setAck(false);
                changed();
              })
            }
          >
            Сохранить босса сезона
          </Button>
        </>
      ) : (
        <p>Босс месяца ещё не выбран.</p>
      )}
      {action.error && <p role="alert">{action.error}</p>}
    </details>
  );
}
