"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DateTime } from "luxon";
import * as api from "@/lib/seasons";
import { SafeLink, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { useChallengeAction, useChallengeKey } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input } from "./ui/field";
export function RecoveryControls({
  account,
  timezone,
  changed,
}: {
  account: string;
  timezone: string;
  changed: () => void;
}) {
  const [before, setBefore] = useState(""),
    offer = useQuery({
      queryKey: ["account", account, "comeback"],
      queryFn: ({ signal }) => api.getComeback(signal),
    }),
    status = useQuery({
      queryKey: ["account", account, "recovery-status"],
      queryFn: ({ signal }) => api.getRecoveryStatus(signal),
    }),
    history = useQuery({
      queryKey: ["account", account, "recovery-history", before],
      queryFn: ({ signal }) => api.getRecoveryHistory(before, signal),
    });
  return (
    <>
      <Card>
        <h2>Вернуться в ритм</h2>
        <p>
          После трёх дней без результата можно начать с трёх небольших шагов. Отказ и пропуски не
          уменьшают накопленный опыт.
        </p>
        {offer.isPending && <p role="status">Проверяем доступность…</p>}
        {offer.isError || status.isError || history.isError ? (
          <p role="alert">Не удалось загрузить восстановление. Обновите данные.</p>
        ) : null}
        {offer.data?.offer?.state === "offered" ? (
          <Comeback
            key={offer.data.offer.id + ":" + offer.data.offer.version}
            value={offer.data}
            changed={changed}
          />
        ) : (
          offer.data && (
            <p>
              {offer.data.cooldown_until
                ? `Следующее предложение доступно после ${DateTime.fromISO(offer.data.cooldown_until).setZone(timezone).toFormat("dd.LL.yyyy HH:mm")}.`
                : "Сейчас новый вызов не требуется. Продолжайте в удобном темпе."}
            </p>
          )
        )}
        <h3>История возвращений</h3>
        {history.data?.items.map((o) => (
          <p key={o.id}>
            {o.offered_at.slice(0, 10)} ·{" "}
            {
              {
                offered: "Предложено",
                accepted: "Принято",
                declined: "Отказ без штрафа",
                withdrawn: "Активность возобновлена",
                expired: "Предложение истекло",
              }[o.state]
            }
            {o.result?.goal_id && (
              <>
                {" "}
                · <SafeLink href={`/app/goals/${o.result.goal_id}`}>Цель возвращения</SafeLink>
              </>
            )}
            {o.result?.run_id && (
              <>
                {" "}
                ·{" "}
                <SafeLink href={`/app/challenges?run_id=${o.result.run_id}`}>
                  Испытание возвращения
                </SafeLink>
              </>
            )}
          </p>
        ))}
        {history.data && !history.data.items.length && <p>История пока пуста.</p>}
        {history.data?.next && (
          <Button variant="secondary" onClick={() => setBefore(history.data!.next)}>
            Предыдущие возвращения
          </Button>
        )}
        <h3>Серии привычек и новый старт</h3>
        <p>
          Минимальный шаг помогает продолжить. Серия растёт при полной норме; прошлые пропуски
          остаются в истории.
        </p>
        {status.data && !status.data.complete && (
          <p role="status">История привычек ещё пересчитывается. Обновите данные.</p>
        )}
        {status.data?.habits.map((h) => (
          <div key={h.id}>
            <SafeLink href={h.url}>{h.title}</SafeLink>
            <p>
              Текущая серия: {h.stats.current}; лучшая: {h.stats.best}; пропущено периодов:{" "}
              {h.stats.missed}.
            </p>
            {h.current_amount !== null && (
              <p>
                Текущий период: {h.current_amount} / {h.full_target}. Минимальный шаг: {h.minimum}
                {h.minimum_met ? " — выполнен" : " — ещё впереди"}.
              </p>
            )}
            <details>
              <summary>История периодов</summary>
              <ul>
                {h.periods.map((p) => (
                  <li key={p.id}>
                    {p.start.slice(0, 10)}:{" "}
                    {p.excluded
                      ? "Исключённый период"
                      : p.state === "missed"
                        ? "Пропуск"
                        : p.state === "success"
                          ? "Полная норма"
                          : "Период открыт"}{" "}
                    · {p.amount} / {p.target}
                  </li>
                ))}
              </ul>
            </details>
          </div>
        ))}
        {status.data?.comebacks.map((c) => (
          <p key={c.offer_id}>
            <SafeLink href={c.url}>Прогресс возвращения</SafeLink>:{" "}
            {c.member.progress?.current ?? 0} / {c.member.progress?.target ?? 3} задач. Серии
            привычек считаются отдельно.
          </p>
        ))}
      </Card>
      <LowEnergy timezone={timezone} changed={changed} />
    </>
  );
}
function Comeback({ value, changed }: { value: api.Eligibility; changed: () => void }) {
  const offer = value.offer!,
    [steps, setSteps] = useState(offer.steps),
    [ack, setAck] = useState(false),
    [ackZone, setAckZone] = useState(""),
    [dirty, setDirty] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(dirty);
  const decide = (accept: boolean) =>
    void action.run(async () => {
      const body = {
        accept,
        timezone: value.timezone,
        steps: accept ? steps.map((s) => api.recoveryStep.parse(s)) : [],
        area_id: null,
      };
      await api.decideComeback(offer, body, key(body));
      setDirty(false);
      setAck(false);
      changed();
    });
  return (
    <section>
      <h3>Три посильных шага</h3>
      <p>
        Принятие создаст одну личную цель, три задачи без даты в списке «Потом» и испытание на{" "}
        {offer.challenge_rules.days} дней: выполнить {offer.challenge_rules.target} связанные
        задачи, не более {offer.challenge_rules.daily_cap} в день. Награда за результат: до{" "}
        {offer.reward_xp} XP с учётом общих лимитов наград. Повторный вход разрешён. Данные частные.
      </p>
      <p>
        Начало — день принятия в зоне {value.timezone}. Отметки задач проверяются автоматически.
        Награда начисляется один раз за достигнутый результат при закрытии испытания.
      </p>
      {steps.map((s, i) => (
        <fieldset key={s.id}>
          <legend>Шаг {i + 1}</legend>
          <Input
            label={`Название шага ${i + 1}`}
            value={s.title}
            maxLength={200}
            required
            onChange={(e) => {
              setSteps(steps.map((v, n) => (n === i ? { ...v, title: e.target.value } : v)));
              setDirty(true);
              setAck(false);
            }}
          />
          <Input
            label={`Описание шага ${i + 1}`}
            value={s.description}
            maxLength={2000}
            onChange={(e) => {
              setSteps(steps.map((v, n) => (n === i ? { ...v, description: e.target.value } : v)));
              setDirty(true);
              setAck(false);
            }}
          />
          <Input
            type="number"
            label={`Минут на шаг ${i + 1}`}
            min={1}
            max={15}
            value={s.estimated_minutes}
            onChange={(e) => {
              setSteps(
                steps.map((v, n) =>
                  n === i ? { ...v, estimated_minutes: Number(e.target.value) } : v,
                ),
              );
              setDirty(true);
              setAck(false);
            }}
          />
        </fieldset>
      ))}
      <label>
        <input
          type="checkbox"
          checked={ack && ackZone === value.timezone}
          onChange={(e) => {
            setAck(e.target.checked);
            setAckZone(value.timezone);
          }}
        />{" "}
        Согласен с шагами, сроком, правилами и созданием связанных задач
      </label>
      <div className="button-row">
        <Button
          disabled={!ack || ackZone !== value.timezone || action.pending}
          onClick={() => decide(true)}
        >
          Принять возвращение
        </Button>
        <Button variant="secondary" disabled={action.pending} onClick={() => decide(false)}>
          Отказаться без штрафа
        </Button>
      </div>
      {action.error && (
        <p role="alert">
          {action.error} При изменении часового пояса обновите данные и подтвердите правила заново.
          Ваши правки шагов сохраняются.
        </p>
      )}
    </section>
  );
}
const reasons: Record<string, string> = {
  deadline: "Ближайший или просроченный срок",
  fixed_commitment: "Связано с фиксированным событием",
  unknown_estimate: "Нет оценки времени",
  no_longer_pending: "Завершена, перенесена или недоступна",
  optional: "Гибкая задача",
};
function LowEnergy({ timezone, changed }: { timezone: string; changed: () => void }) {
  const [date, setDate] = useState(DateTime.now().setZone(timezone).toISODate()!),
    [minutes, setMinutes] = useState(15),
    [preview, setPreview] = useState<api.LowEnergy | null>(null),
    [selected, setSelected] = useState<string[]>([]),
    [reason, setReason] = useState(""),
    [ack, setAck] = useState(false),
    [unknown, setUnknown] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(!!preview && !preview.applied_at);
  function invalidate() {
    setPreview(null);
    setAck(false);
    setUnknown(false);
  }
  const flexible =
      preview?.candidates
        .filter((c) => selected.includes(c.id))
        .reduce((sum, c) => sum + c.estimated_minutes, 0) ?? 0,
    protectedMinutes =
      preview?.candidates
        .filter((c) => c.required)
        .reduce((sum, c) => sum + c.estimated_minutes, 0) ?? 0;
  return (
    <Card>
      <h2>День с низкой энергией</h2>
      <p>
        Сократите гибкую часть существующего плана. Фиксированные события и задачи с ближайшим
        сроком сохраняются. Удалённые из текущего списка задачи сохраняют свои даты и сроки;
        первоначальный подтверждённый план остаётся в истории.
      </p>
      <Input
        type="date"
        label="День для облегчения"
        value={date}
        onChange={(e) => {
          setDate(e.target.value);
          invalidate();
        }}
      />
      <Input
        type="number"
        label="Минут на гибкие задачи"
        value={minutes}
        min={0}
        max={240}
        onChange={(e) => {
          setMinutes(Number(e.target.value));
          invalidate();
        }}
      />
      <Button
        variant="secondary"
        disabled={action.pending}
        onClick={() =>
          void action.run(async () => {
            const body = { date, minimum_minutes: minutes };
            const v = await api.previewLowEnergy(
              body,
              key({ ...body, refresh: crypto.randomUUID() }),
            );
            setPreview(v);
            setSelected(v.candidates.filter((c) => c.proposed && !c.required).map((c) => c.id));
            setAck(false);
            setUnknown(false);
          })
        }
      >
        Показать облегчённый план
      </Button>
      {preview && (
        <section>
          <h3>Предпросмотр на {preview.date}</h3>
          <p>
            Зона: {preview.timezone}. Действует до{" "}
            {DateTime.fromISO(preview.expires_at).setZone(preview.timezone).toFormat("HH:mm")}.
            Фиксированные задачи: {protectedMinutes} мин; выбранная гибкая часть: {flexible} /{" "}
            {preview.minimum_minutes} мин. Фиксированные обязательства не входят в этот бюджет.
          </p>
          <h4>События сохраняются</h4>
          {preview.commitments.length ? (
            <ul>
              {preview.commitments.map((e) => (
                <li key={e.id}>
                  {e.title} ·{" "}
                  {e.all_day
                    ? "Весь день"
                    : `${DateTime.fromISO(e.start).setZone(preview.timezone).toFormat("HH:mm")} — ${DateTime.fromISO(e.end).setZone(preview.timezone).toFormat("HH:mm")}`}
                </li>
              ))}
            </ul>
          ) : (
            <p>Фиксированных событий нет.</p>
          )}
          <h4>Задачи плана</h4>
          {preview.candidates.map((c) => (
            <label key={c.id}>
              <input
                type="checkbox"
                checked={c.required || selected.includes(c.id)}
                disabled={c.required || !c.eligible || !!preview.applied_at}
                onChange={(e) => {
                  setSelected(
                    e.target.checked ? [...selected, c.id] : selected.filter((id) => id !== c.id),
                  );
                  setAck(false);
                }}
              />{" "}
              {c.title} · {c.estimated_minutes || "Неизвестно"} мин ·{" "}
              {reasons[c.reason] ?? "Гибкая задача"}
            </label>
          ))}
          <p>
            Сокращение списка не увеличивает процент выполнения исходного плана. Если фиксированные
            обязательства не помещаются в рабочие окна, их нужно отдельно пересогласовать в
            календаре.
          </p>
          <SafeLink href="/app/planning">Открыть полный план и доступные окна</SafeLink>
          {preview.applied_at ? (
            <p role="status">
              Изменение сохранено как дополнение. Первоначальный план и события сохранены.
            </p>
          ) : (
            <>
              <Input
                label="Причина облегчения"
                value={reason}
                maxLength={2000}
                onChange={(e) => setReason(e.target.value)}
              />
              <label>
                <input
                  type="checkbox"
                  checked={unknown}
                  onChange={(e) => setUnknown(e.target.checked)}
                />{" "}
                Понимаю неопределённость задач без оценки времени
              </label>
              <label>
                <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
                Подтверждаю выбранные задачи и сохранение фиксированных обязательств
              </label>
              <Button
                disabled={
                  !ack || !reason.trim() || action.pending || flexible > preview.minimum_minutes
                }
                onClick={() =>
                  void action.run(async () => {
                    const body = {
                      confirm: true,
                      reason,
                      optional_ids: selected,
                      confirm_unknown: unknown,
                    };
                    setPreview(
                      await api.applyLowEnergy(preview.id, body, key({ id: preview.id, ...body })),
                    );
                    setAck(false);
                    changed();
                  })
                }
              >
                Применить облегчённый план
              </Button>
            </>
          )}
        </section>
      )}
      {action.error && (
        <p role="alert">{action.error} При изменении источников составьте новый предпросмотр.</p>
      )}
    </Card>
  );
}
