"use client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import * as api from "@/lib/navigator";
import { useChallengeAction } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { EmptyScene } from "./ui/experience";
import { Compass } from "lucide-react";
const ruleTitles: Record<string, string> = {
  R01: "Первый шаг без откладывания",
  R02: "Посильный план",
  R03: "Время до дедлайна",
  R04: "Комфортный ритм привычки",
  R05: "Мягкое возвращение",
  R06: "План по силам",
  R07: "Начать с пяти минут",
  R08: "Взгляд на прошедшую неделю",
  R09: "Точнее оценивать время",
  R10: "Следующий шаг к цели",
  R11: "Семь периодов подряд",
  R12: "Место в календаре",
  R13: "Пересмотреть срок цели",
  R14: "Главная цель месяца",
  R15: "Подвести итог фокуса",
};
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
      <Card className="navigator-insights">
        <h2>Подсказки по вашим записям</h2>
        <details className="quiet-details">
          <summary>Откуда берутся подсказки</summary>
          <p>
            Правила используют числа и состояния задач. Открытие действия само по себе ничего не
            меняет. Одновременно показываем до трёх подсказок.
          </p>
        </details>
        {!data.data.complete && (
          <p role="status">
            Часть истории ещё обновляется. Подсказки по неполным данным появятся после пересчёта.
          </p>
        )}
        {!data.data.items.length && (
          <EmptyScene
            title="Вы выбираете направление"
            description="Сейчас подходящих подсказок нет. Продолжайте в своём темпе."
          />
        )}
        <div className="navigator-card-grid">
          {data.data.items.map((c) => (
            <article key={c.id} className="navigator-card">
              <Compass size={28} aria-hidden="true" />
              <h3>{ruleTitles[c.rule.id] ?? "Следующий шаг"}</h3>
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
                  Повторяем такую подсказку не чаще раза в {c.rule.cooldown_seconds / 86400} дней.
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
                  aria-label={`Скрыть подсказку: ${ruleTitles[c.rule.id] ?? "Следующий шаг"}`}
                  disabled={action.pending}
                  onClick={() =>
                    void action.run(async () => {
                      await api.decideCard(c, "hide");
                      await changed();
                    })
                  }
                >
                  Скрыть
                </Button>
                <Button
                  variant="secondary"
                  aria-label={`Отложить на день: ${ruleTitles[c.rule.id] ?? "Следующий шаг"}`}
                  disabled={action.pending}
                  onClick={() =>
                    void action.run(async () => {
                      await api.decideCard(c, "snooze");
                      await changed();
                    })
                  }
                >
                  Завтра
                </Button>
                <Button
                  variant="secondary"
                  aria-label={`Отключить подсказки: ${ruleTitles[c.rule.id] ?? "Следующий шаг"}`}
                  disabled={action.pending}
                  onClick={() =>
                    void action.run(async () => {
                      const s = data.data!.settings.find((s) => s.rule_id === c.rule.id);
                      await api.setRule(c.rule.id, true, s?.version ?? 0);
                      await changed();
                    })
                  }
                >
                  Не показывать
                </Button>
              </div>
            </article>
          ))}
        </div>
        {action.error && <p role="alert">{action.error}</p>}
      </Card>
      <Card>
        <details>
          <summary>Какие подсказки показывать</summary>
          <p>Отключённые правила перестают показывать подсказки. Можно включить их снова.</p>
          {data.data.rules.map((r) => {
            const s = data.data!.settings.find((s) => s.rule_id === r.id);
            return (
              <p key={r.id}>
                {ruleTitles[r.id] ?? "Другие подсказки"} · {s?.disabled ? "Отключено" : "Включено"}{" "}
                <Button
                  variant="secondary"
                  aria-label={`${s?.disabled ? "Включить" : "Отключить"} подсказки: ${ruleTitles[r.id] ?? "Другие подсказки"}`}
                  disabled={action.pending}
                  onClick={() =>
                    void action.run(async () => {
                      await api.setRule(r.id, !s?.disabled, s?.version ?? 0);
                      await changed();
                    })
                  }
                >
                  {s?.disabled ? "Включить" : "Отключить"}
                </Button>
              </p>
            );
          })}
        </details>
      </Card>
    </>
  );
}
