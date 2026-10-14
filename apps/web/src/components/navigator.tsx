"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import * as api from "@/lib/navigator";
import { useChallengeAction } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
export const factNames: Record<string, string> = {
  postponements: "Переносов",
  window_days: "Дней в окне",
  planned_minutes: "Запланировано минут",
  available_minutes: "Доступно минут",
  deficit_minutes: "Дефицит минут",
  hours_to_deadline: "Часов до срока",
  estimated_minutes: "Оценка, минут",
  free_minutes: "Свободно минут",
  missed_periods: "Закрытых пропущенных периодов подряд",
  inactive_days: "Дней без подтверждённых действий",
  returned: "Возвращение в приложение",
  low_energy: "Низкая энергия выбрана",
  visible_seconds: "Секунд активного просмотра",
  first_steps: "Первых шагов",
  review_missing: "Обзор отсутствует",
  samples: "Завершений в выборке",
  actual_estimate_ratio: "Факт / оценка",
  next_actions: "Следующих действий",
  streak: "Периодов подряд",
  overlap_minutes: "Минут пересечения",
  overdue: "Срок прошёл",
  progress: "Прогресс, %",
  boss_missing: "Босс месяца не выбран",
  pending_sessions: "Итогов без подтверждения",
};
export function NavigatorCards() {
  const auth = useAuth(),
    id = auth.session?.account.id,
    cache = useQueryClient(),
    action = useChallengeAction();
  const data = useQuery({
    queryKey: ["account", id, "navigator"],
    queryFn: ({ signal }) => api.getNavigator(signal),
    enabled: !!id,
    refetchInterval: 60000,
  });
  const changed = () => cache.invalidateQueries({ queryKey: ["account", id, "navigator"] });
  if (!id) return null;
  if (!data.data)
    return (
      <Card>
        <p role={data.isError ? "alert" : "status"}>
          {data.isError ? "Не удалось загрузить подсказки." : "Собираем исходные числа…"}
        </p>
        <Button onClick={() => void data.refetch()}>Повторить</Button>
      </Card>
    );
  return (
    <>
      <Card>
        <h2>Подсказки по вашим записям</h2>
        <p>
          Правила используют числа и состояния задач. Открытие действия само по себе ничего не
          меняет. Одновременно показываем до трёх подсказок.
        </p>
        {!data.data.complete && (
          <p role="status">
            Часть истории ещё обновляется. Подсказки по неполным данным появятся после пересчёта.
          </p>
        )}
        {!data.data.items.length && <p>Сейчас подходящих подсказок нет.</p>}
        {data.data.items.map((c) => (
          <article key={c.id} style={{ marginBlock: "1rem" }}>
            <h3>{c.rule.id} · Подсказка</h3>
            <p>{c.message}</p>
            <details>
              <summary>Почему появилась эта подсказка</summary>
              <dl>
                {Object.entries(c.fact.numbers).map(([key, value]) => (
                  <div key={key}>
                    <dt>{factNames[key] ?? key}</dt>
                    <dd>{value.toLocaleString("ru-RU", { maximumFractionDigits: 2 })}</dd>
                  </div>
                ))}
              </dl>
              <p>
                Версия правила: {c.rule.version}. Интервал повторения:{" "}
                {c.rule.cooldown_seconds / 86400} дней.
              </p>
              <ul>
                {c.fact.sources.map((s, i) => (
                  <li key={i}>
                    <SafeLink href={s.url}>Открыть исходную запись {i + 1}</SafeLink>
                    {s.version > 0 ? ` · версия ${s.version}` : ""}
                  </li>
                ))}
              </ul>
            </details>
            <div className="button-row">
              <SafeLink href={c.action_url}>Перейти к действию</SafeLink>
              <Button
                variant="secondary"
                disabled={action.pending}
                onClick={() =>
                  void action.run(async () => {
                    await api.decideCard(c, "hide");
                    await changed();
                  })
                }
              >
                Скрыть {c.rule.id}
              </Button>
              <Button
                variant="secondary"
                disabled={action.pending}
                onClick={() =>
                  void action.run(async () => {
                    await api.decideCard(c, "snooze");
                    await changed();
                  })
                }
              >
                Отложить {c.rule.id} на день
              </Button>
              <Button
                variant="secondary"
                disabled={action.pending}
                onClick={() =>
                  void action.run(async () => {
                    const s = data.data!.settings.find((s) => s.rule_id === c.rule.id);
                    await api.setRule(c.rule.id, true, s?.version ?? 0);
                    await changed();
                  })
                }
              >
                Отключить {c.rule.id}
              </Button>
            </div>
          </article>
        ))}
        {action.error && <p role="alert">{action.error}</p>}
      </Card>
      <Card>
        <details>
          <summary>Настройки правил R01–R15</summary>
          <p>Отключённые правила перестают показывать подсказки. Можно включить их снова.</p>
          {data.data.rules.map((r) => {
            const s = data.data!.settings.find((s) => s.rule_id === r.id);
            return (
              <p key={r.id}>
                {r.id} · {s?.disabled ? "Отключено" : "Включено"}{" "}
                <Button
                  variant="secondary"
                  disabled={action.pending}
                  onClick={() =>
                    void action.run(async () => {
                      await api.setRule(r.id, !s?.disabled, s?.version ?? 0);
                      await changed();
                    })
                  }
                >
                  {s?.disabled ? "Включить" : "Отключить"} правило {r.id}
                </Button>
              </p>
            );
          })}
        </details>
      </Card>
    </>
  );
}
