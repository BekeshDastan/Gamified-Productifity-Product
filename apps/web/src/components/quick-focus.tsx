"use client";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import { startFocus, broadcastFocus } from "@/lib/focus";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import type { Quest } from "@/lib/quests";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
export function QuickFocusStart({ quest }: { quest: Quest }) {
  const auth = useAuth(),
    cache = useQueryClient(),
    router = useRouter(),
    discard = useConfirmDiscard(),
    operation = useRef<{ id: string; key: string } | null>(null),
    locked = useRef(false);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function start() {
    if (locked.current || !discard()) return;
    locked.current = true;
    setBusy(true);
    setError("");
    operation.current ??= { id: crypto.randomUUID(), key: crypto.randomUUID() };
    try {
      await startFocus(
        {
          operation_id: operation.current.id,
          mode: "quick",
          title: "",
          quest_id: quest.id,
          goal_id: null,
          area_id: null,
          work_minutes: 5,
          break_minutes: 5,
          sound_enabled: false,
        },
        operation.current.key,
      );
      await cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
      if (auth.session) broadcastFocus(auth.session.account.id);
      router.push("/app/focus");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Не удалось начать сессию. Повтор безопасен.");
    } finally {
      locked.current = false;
      setBusy(false);
    }
  }
  if (quest.archived_at || ["completed", "failed", "skipped"].includes(quest.status)) return null;
  return (
    <Card>
      <h2>Начать с малого</h2>
      <p>
        Пять минут для этой задачи. Затем можно закончить или продолжить. Статус задачи изменяется
        вашим отдельным решением.
      </p>
      <Button disabled={busy} onClick={() => void start()}>
        Начать с пяти минут
      </Button>
      {error && <p role="alert">{error}</p>}
      <SafeLink href="/app/focus">Открыть текущий таймер</SafeLink>
    </Card>
  );
}
