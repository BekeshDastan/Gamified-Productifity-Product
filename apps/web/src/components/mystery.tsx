"use client";
import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { getMystery, openMystery } from "@/lib/mystery";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { SafeLink } from "@/lib/forms/unsaved-changes";
export function MysteryRewards() {
  const auth = useAuth(),
    cache = useQueryClient(),
    busyRef = useRef(false),
    keys = useRef(new Map<string, string>());
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const q = useQuery({
    queryKey: ["account", auth.session?.account.id, "mystery"],
    enabled: !!auth.session,
    queryFn: ({ signal }) => getMystery(signal),
    refetchInterval: 15000,
  });
  async function open(id: string) {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const key = keys.current.get(id) ?? crypto.randomUUID();
      keys.current.set(id, key);
      const g = await openMystery(id, key);
      const name = g.weights.find((i) => i.id === g.item_id)?.name;
      setNotice(
        g.item_id
          ? `Получен предмет: ${name ?? g.item_id}.`
          : `Получена компенсация: ${g.compensation} монет.`,
      );
      await cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
    } catch {
      setError("Не удалось открыть подарок. Повтор сохранит единственный результат.");
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>Бесплатные сюрпризы</h2>
      <p>
        По одному подарку за достижения «Три намерения» (создать три цели) и «Свои ритмы» (создать
        три привычки). Подарки нельзя купить за монеты или деньги.
      </p>
      <p>
        Выбираем только ещё не полученные предметы. Вероятность предмета равна его весу, делённому
        на сумму весов оставшихся предметов. Если весь набор уже ваш, компенсация составляет{" "}
        {q.data?.compensation ?? 5} монет. XP не начисляется.
      </p>
      {q.isError && <p role="alert">Не удалось загрузить подарки.</p>}
      {!q.data && <Button onClick={() => void q.refetch()}>Повторить загрузку подарков</Button>}
      {q.data && !q.data.enabled && (
        <p>Новые сюрпризы отключены. Уже заработанные подарки остаются доступны.</p>
      )}
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {q.data?.grants.length === 0 && <p>Здесь появятся заработанные подарки.</p>}
      <ul>
        {q.data?.grants.map((g) => (
          <li key={g.id}>
            {g.source === "goal-created-3" ? "Три намерения" : "Свои ритмы"} ·{" "}
            {g.opened_at ? (
              g.item_id ? (
                `Получен: ${g.weights.find((i) => i.id === g.item_id)?.name ?? g.item_id}`
              ) : (
                `Компенсация: ${g.compensation} монет`
              )
            ) : (
              <Button disabled={busy} onClick={() => void open(g.id)}>
                Открыть бесплатный подарок
              </Button>
            )}
            <details>
              <summary>Веса предметов · {g.catalog_version}</summary>
              <ul>
                {g.weights.map((i) => (
                  <li key={i.id}>
                    {i.name}: {i.weight}
                  </li>
                ))}
              </ul>
            </details>
          </li>
        ))}
      </ul>
      <details>
        <summary>Правила текущего каталога · {q.data?.version}</summary>
        <ul>
          {q.data?.items.map((i) => (
            <li key={i.id}>
              {i.name}: вес {i.weight}
            </li>
          ))}
        </ul>
      </details>
      <SafeLink href="/app/character">Открыть инвентарь</SafeLink>
    </Card>
  );
}
