"use client";
import { useRef, useState } from "react";
import { DateTime } from "luxon";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { getWeekReview, previewWeekReview, type WeekPreview } from "@/lib/journal";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
export function WeeklyReset() {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p>Загружаем недельный обзор…</p>;
  return (
    <Weekly
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
      start={settings.data.week_start}
    />
  );
}
function Weekly({
  account,
  timezone,
  start,
}: {
  account: string;
  timezone: string;
  start: number;
}) {
  const today = DateTime.now().setZone(timezone),
    discard = useConfirmDiscard();
  const [date, setDate] = useState(() =>
      today.minus({ days: (today.weekday - start + 7) % 7 }).toISODate()!,
    ),
    [preview, setPreview] = useState<WeekPreview | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const key = useRef({ date: "", key: "" });
  const q = useQuery({
    queryKey: accountQueryKey(account, "weekly-review", date),
    queryFn: ({ signal }) => getWeekReview(date, signal),
  });
  return (
    <div className="journal-layout">
      <Card>
        <h2>Weekly Reset</h2>
        <p>
          Сначала разберите незавершённое. Просрочки не удаляются автоматически, исходные даты и
          дедлайны сохраняются в предпросмотре.
        </p>
        <label className="ui-field">
          Начало недели обзора
          <input
            className="ui-input"
            type="date"
            value={date}
            onChange={(e) => {
              if (e.target.value && discard()) {
                setDate(e.target.value);
                setPreview(null);
                setError("");
              }
            }}
          />
        </label>
        <p>
          Неделя начинается {start === 7 ? "в воскресенье" : "в понедельник"}. Часовой пояс:{" "}
          {timezone}.
        </p>
        {q.isError && (
          <p role="alert">
            Не удалось загрузить обзор. <Button onClick={() => void q.refetch()}>Повторить</Button>
          </p>
        )}
        {error && <p role="alert">{error}</p>}
        <Button
          disabled={busy || !q.data}
          onClick={async () => {
            if (!discard()) return;
            setBusy(true);
            setError("");
            if (key.current.date !== date) key.current = { date, key: crypto.randomUUID() };
            try {
              setPreview(await previewWeekReview(date, key.current.key));
              key.current = { date: "", key: "" };
            } catch {
              setError("Не удалось подготовить обзор. Повторите.");
            } finally {
              setBusy(false);
            }
          }}
        >
          Подготовить Weekly Reset
        </Button>
      </Card>
      {(preview?.snapshot ?? q.data?.snapshot) && (
        <WeekSources snapshot={(preview?.snapshot ?? q.data?.snapshot)!} />
      )}{" "}
      {preview && (
        <Card>
          <h2>Предпросмотр подготовлен</h2>
          <p>
            Задачи и планы пока не изменены. Предпросмотр действует до{" "}
            {new Date(preview.expires_at).toLocaleTimeString("ru-RU")}.
          </p>
        </Card>
      )}
    </div>
  );
}
function WeekSources({ snapshot }: { snapshot: WeekPreview["snapshot"] }) {
  return (
    <Card>
      <h2>Незавершённое и следующая неделя</h2>
      <p>
        Следующая неделя: {snapshot.next_week}. Доступно задач: {snapshot.items.length}
        {snapshot.more ? " (показаны первые 500)" : ""}.
      </p>
      {snapshot.items.length === 0 && <p>Незавершённых задач нет.</p>}
      <ul>
        {snapshot.items.map((item) => (
          <li key={item.id}>
            <SafeLink href={"/app/quests/" + item.id}>{item.title}</SafeLink>
            <p>
              Исходная дата: {item.planned_date ?? "не задана"} · Дедлайн:{" "}
              {item.deadline
                ? new Date(item.deadline).toLocaleString("ru-RU", { timeZone: snapshot.timezone })
                : "не задан"}{" "}
              · Оценка: {item.estimated_minutes || "не задана"} мин. · Приоритет: {item.priority}
            </p>
            {item.reason && <p>Последняя причина: {item.reason}</p>}
          </li>
        ))}
      </ul>
      <h3>Причины из вечерних разборов</h3>
      {snapshot.reasons.length === 0 ? (
        <p>За эту неделю разборов пока нет.</p>
      ) : (
        <ul>
          {snapshot.reasons.map((reason) => (
            <li key={reason.reason}>
              {reason.reason}: {reason.count}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
