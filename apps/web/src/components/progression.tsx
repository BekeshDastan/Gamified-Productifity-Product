"use client";
import { DateTime } from "luxon";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { useProgression, getProgressionHistory, type Reward } from "@/lib/progression";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { useToast } from "./ui/toast";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { StatTile, EmptyScene } from "./ui/experience";
import { Coins, Sparkles, Trophy, ArrowUpRight } from "lucide-react";
const signed = (v: number) => (v >= 0 ? "+" : "") + v;
export function useRewardNotice() {
  const toast = useToast(),
    auth = useAuth(),
    cache = useQueryClient();
  return (reward?: Reward) => {
    void cache.invalidateQueries({
      queryKey: ["account", auth.session?.account.id],
      refetchType: "none",
    });
    if (!reward || reward.kind === "unchanged") return;
    const delta = `${signed(reward.xp_delta)} XP · ${signed(reward.coins_delta)} монет`;
    toast({
      title:
        reward.kind === "not_awarded"
          ? "История сохранена"
          : reward.xp_delta < 0
            ? `Корректировка: ${delta}`
            : reward.xp_delta === 0
              ? "Выполнение учтено"
              : reward.level_after > reward.level_before
                ? `Уровень ${reward.level_after} · ${delta}`
                : delta,
      description: reward.reason,
      tone: reward.xp_delta > 0 ? "success" : undefined,
    });
  };
}
export function ProgressionSummary({ compact = false }: { compact?: boolean }) {
  const data = useProgression(),
    settings = useSettings();
  if (!data.data)
    return (
      <div>
        <h2>Опыт и монеты</h2>
        <p role={data.isError ? "alert" : "status"}>
          {data.isError ? "Не удалось загрузить прогресс." : "Загружаем прогресс…"}
        </p>
        <Button onClick={() => void data.refetch()}>Повторить</Button>
      </div>
    );
  const d = data.data;
  return (
    <div className={`progression-summary${compact ? "" : " progression-full"}`}>
      {!compact && (
        <div className="level-crest" aria-hidden="true">
          <i />
          <i />
          <Trophy size={50} />
          <span>{d.level.number}</span>
        </div>
      )}
      <div className="level-content">
        {!compact && <p className="eyebrow">КАЖДЫЙ ШАГ ИМЕЕТ ЗНАЧЕНИЕ</p>}
        <h2>Уровень {d.level.number}</h2>
        {compact && (
          <p>
            Всего: <strong>{d.total_xp} XP</strong>
          </p>
        )}
        <progress
          aria-label="Опыт до следующего уровня"
          value={d.level.inside}
          max={d.level.inside + d.level.to_next}
        />
        <p>
          {d.level.inside} XP на этом уровне · до следующего {d.level.to_next} XP
        </p>
        {compact && (
          <p>
            Доступно: <strong>{d.available_coins} монет</strong>
          </p>
        )}
      </div>
      {d.coin_correction > 0 && (
        <p>
          Корректировка: {d.coin_correction} виртуальных монет. Новые монеты сначала покрывают
          корректировку.
        </p>
      )}
      {!compact && (
        <div className="stat-grid progression-stats">
          <StatTile
            label="Накопленный опыт"
            value={d.total_xp}
            detail="XP за ваши действия"
            icon={<Sparkles />}
          />
          <StatTile
            label="Кошелёк"
            value={d.available_coins}
            detail="Монеты для личных наград"
            tone="gold"
            icon={<Coins />}
          />
          <StatTile
            label="До следующего уровня"
            value={d.level.to_next}
            detail="XP до нового открытия"
            tone="mint"
            icon={<Trophy />}
          />
        </div>
      )}
      {!compact && (
        <div className="progression-shortcuts">
          <SafeLink href="/app/quests">
            Сделать следующий шаг <ArrowUpRight size={17} />
          </SafeLink>
          <SafeLink href="/app/rewards">
            Выбрать награду <ArrowUpRight size={17} />
          </SafeLink>
          <SafeLink href="/app/character">
            Мой персонаж <ArrowUpRight size={17} />
          </SafeLink>
        </div>
      )}
      {!compact && (
        <details className="quiet-details">
          <summary>Лимиты опыта за 24 часа</summary>
          <p className="ui-hint">
            За скользящие 24 часа: задачи {d.quota.task_used} / {d.quota.task_cap} XP · привычки{" "}
            {d.quota.habit_used} / {d.quota.habit_cap} XP. Общая квота {d.quota.total_used} /{" "}
            {d.quota.total_cap} XP.
          </p>
        </details>
      )}
      {compact ? (
        <SafeLink href="/app/progress">История и правила наград</SafeLink>
      ) : (
        <details>
          <summary>Как начисляются награды</summary>
          <p>
            Листовые задачи по сложности 1–5: {d.rules.task_xp.join(", ")} XP. Полная норма периода
            привычки: {d.rules.habit_xp} XP. Испытание: {d.rules.challenge_xp} XP; его квота{" "}
            {d.quota.challenge_used} / {d.quota.challenge_cap} XP.
          </p>
          <p>
            Монеты считаются сервером: целая часть от фактически выданного XP /{" "}
            {d.rules.coins_divisor}. Частичная квота уменьшает начисление; остаток не переносится.
            Создание, перенос, контейнеры и просмотр страницы XP не дают.
          </p>
          <p>
            Исправление выполнения компенсирует исходные суммы. Восстановление возвращает только
            исходное право и не расходует квоту снова. Новая награда за историческую привычку
            доступна до конца её периода + 24 часа включительно.
          </p>
          <p className="ui-hint">
            Пояс аккаунта не сбрасывает квоту. Текущее окно начинается{" "}
            {DateTime.fromISO(d.quota.window_start)
              .setZone(settings.data?.timezone ?? "UTC")
              .setLocale(settings.data?.language ?? "ru")
              .toLocaleString(DateTime.DATETIME_SHORT)}
            . Версия правил: {d.economy_version}.
          </p>
        </details>
      )}
    </div>
  );
}
export function Progression() {
  const auth = useAuth(),
    settings = useSettings();
  const history = useInfiniteQuery({
    queryKey: ["account", auth.session?.account.id, "progression-history"],
    enabled: !!auth.session?.account.id,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => getProgressionHistory(pageParam, signal),
    getNextPageParam: (p) => p.next_cursor ?? undefined,
    staleTime: 0,
  });
  return (
    <>
      <Card className="progression-hero">
        <ProgressionSummary />
      </Card>
      <Card>
        <h2>История XP и монет</h2>
        <Button variant="secondary" onClick={() => void history.refetch()}>
          Обновить историю
        </Button>
        {history.isPending && <p role="status">Загружаем историю…</p>}
        {history.isError && (
          <p role="alert">
            История недоступна. <Button onClick={() => void history.refetch()}>Повторить</Button>
          </p>
        )}
        {history.data?.pages[0]?.items.length === 0 && (
          <EmptyScene
            title="Первая награда впереди"
            description="Завершите задачу или выполните норму привычки — здесь появится история вашего прогресса."
            tone="gold"
          >
            <SafeLink href="/app/quests">К моим задачам</SafeLink>
          </EmptyScene>
        )}
        <ol className="progression-history">
          {history.data?.pages
            .flatMap((p) => p.items)
            .map((e) => (
              <li key={e.id}>
                <h3>
                  {e.source_url ? (
                    <SafeLink href={e.source_url}>{e.source_title}</SafeLink>
                  ) : (
                    e.source_title
                  )}
                </h3>
                <p>
                  <strong>
                    {
                      {
                        grant: e.xp_delta === 0 ? "Выполнение без нового XP" : "Награда",
                        reversal: "Корректировка",
                        restoration: "Восстановление",
                        spend: "Расход монет",
                        refund: "Возврат монет",
                        bonus: "Компенсация набора",
                      }[e.kind]
                    }
                  </strong>{" "}
                  · {signed(e.xp_delta)} XP · {signed(e.coins_delta)} монет
                </p>
                <p>{e.reason}</p>
                <p className="ui-hint">
                  {DateTime.fromISO(e.occurred_at)
                    .setZone(settings.data?.timezone ?? "UTC")
                    .setLocale(settings.data?.language ?? "ru")
                    .toLocaleString({
                      year: "numeric",
                      month: "short",
                      day: "numeric",
                      hour: "numeric",
                      minute: "2-digit",
                      hour12: settings.data?.time_format === "12h",
                    })}
                  {e.area_name && ` · ${e.area_name}`}
                </p>
                {!["spend", "refund", "bonus"].includes(e.source_kind) && (
                  <details>
                    <summary>Исходное право на награду</summary>
                    <p>
                      Первое начисление: {e.awarded_xp} из {e.requested_xp} XP.
                      {e.source_kind === "task" &&
                        ` Сложность при первом выполнении: ${e.difficulty}.`}{" "}
                      Версия правил: {e.economy_version}.
                    </p>
                  </details>
                )}
              </li>
            ))}
        </ol>
        {history.hasNextPage && (
          <Button
            variant="secondary"
            disabled={history.isFetchingNextPage}
            onClick={() => void history.fetchNextPage()}
          >
            Более ранние начисления
          </Button>
        )}
      </Card>
    </>
  );
}
