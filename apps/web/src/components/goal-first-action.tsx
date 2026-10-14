"use client";
import { useState, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { goalTemplates } from "@/lib/goal-templates";
import { emptyQuest } from "@/lib/quests";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { goalSchema, type Goal } from "@/lib/goals";
import { SafeLink, useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { useChallengeAction } from "./challenges";
import { Card } from "./ui/card";
import { Input, Select } from "./ui/field";
import { Button } from "./ui/button";
export function GoalFirstAction({ goal, onChanged }: { goal: Goal; onChanged: () => void }) {
  const account = useAuth().session?.account.id,
    [title, setTitle] = useState(""),
    [minutes, setMinutes] = useState(15),
    [preview, setPreview] = useState<{ title: string; minutes: number } | null>(null),
    [ack, setAck] = useState(false),
    [saved, setSaved] = useState(false),
    key = useRef(""),
    action = useChallengeAction();
  const catalog = useQuery({
    queryKey: ["account", account, "first-action-catalog"],
    queryFn: ({ signal }) => goalTemplates(signal),
  });
  useUnsavedChanges(!!preview);
  if (goal.state !== "active" || goal.archived_at) return null;
  return (
    <Card>
      <h2>Первый шаг из шаблона</h2>
      <p>
        Для цели без следующего действия. Выберите пример и отредактируйте его; задача и связь с
        целью сохранятся вместе после подтверждения.
      </p>
      <Select
        label="Пример первого шага"
        disabled={!!preview || saved}
        value=""
        onChange={(e) => {
          const t = catalog.data?.find((t) => t.id === e.target.value)?.tasks[0];
          if (t) {
            setTitle(t.title);
            setMinutes(t.estimated_minutes);
          }
        }}
      >
        <option value="">Выберите пример</option>
        {catalog.data?.map((t) => (
          <option key={t.id} value={t.id}>
            {t.name}
          </option>
        ))}
      </Select>
      <Input
        label="Название первого действия"
        value={title}
        disabled={!!preview || saved}
        onChange={(e) => setTitle(e.target.value)}
      />
      <Input
        label="Оценка первого действия, минут"
        type="number"
        min={5}
        max={525600}
        value={minutes}
        disabled={!!preview || saved}
        onChange={(e) => setMinutes(Number(e.target.value))}
      />
      <Button
        disabled={!!preview || saved || !title.trim() || minutes < 5}
        onClick={() => {
          setPreview({ title: title.trim(), minutes });
          key.current = crypto.randomUUID();
          setAck(false);
        }}
      >
        Показать первый шаг
      </Button>
      {preview && (
        <section>
          <h3>Предпросмотр первого действия</h3>
          <p>
            Цель: {goal.title}. Новая самостоятельная обязательная задача: {preview.title}; оценка{" "}
            {preview.minutes} минут, приоритет обычный, сложность 1. Плановая дата и дедлайн не
            задаются; задачу можно разместить отдельно.
          </p>
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
            Подтверждаю создание и связь первого действия
          </label>
          <div className="button-row">
            <Button
              disabled={!ack || action.pending}
              onClick={() =>
                void action.run(async () => {
                  await requireAccount();
                  await withAuthLock(async () => {
                    const a = await authClient();
                    const r = await a.POST("/api/v1/navigator/goals/{id}/first-action", {
                      params: {
                        path: { id: goal.id },
                        header: {
                          ...authParams().header,
                          "If-Match": `"${goal.version}"`,
                          "Idempotency-Key": key.current,
                        },
                      },
                      body: {
                        task: {
                          ...emptyQuest,
                          title: preview.title,
                          estimated_minutes: preview.minutes,
                          status: "todo",
                        },
                        confirm: true,
                      },
                    });
                    goalSchema.parse(r.data?.data?.goal);
                  });
                  setPreview(null);
                  setSaved(true);
                  onChanged();
                })
              }
            >
              Создать первый шаг
            </Button>
            <Button variant="secondary" onClick={() => setPreview(null)} disabled={action.pending}>
              Отменить первый шаг
            </Button>
          </div>
        </section>
      )}
      {action.error && <p role="alert">{action.error}</p>}
      {saved && (
        <p role="status">
          Первое действие добавлено. <SafeLink href="/app/quests">Открыть задачи</SafeLink>
        </p>
      )}
    </Card>
  );
}
