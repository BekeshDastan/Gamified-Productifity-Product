"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell } from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { SafeLink, useUnsavedChanges, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import * as api from "@/lib/notifications";
import { connectPush } from "@/lib/notification-push";
import { useChallengeAction } from "./challenges";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
const names: Record<string, string> = {
  deadline: "Сроки задач",
  habit: "Привычки",
  achievement: "Достижения",
  invite: "Приглашения",
  report: "Отчёты",
  focus_result: "Итоги фокуса",
};
const states: Record<string, string> = {
  queued: "В очереди",
  retry_wait: "Ожидает повторной попытки",
  sending: "Отправляется",
  provider_accepted: "Принято сервисом отправки",
  delivery_unknown: "Результат отправки неизвестен",
  failed: "Не удалось отправить",
  cancelled: "Отменено",
};
function useNotificationsScope() {
  const id = useAuth().session?.account.id;
  const cache = useQueryClient();
  return {
    id,
    key: ["account", id, "notifications"],
    changed: () => cache.invalidateQueries({ queryKey: ["account", id, "notifications"] }),
  };
}
export function NotificationBell() {
  const { id, key } = useNotificationsScope();
  const q = useQuery({
    queryKey: [...key, "inbox", ""],
    queryFn: ({ signal }) => api.inbox("", signal),
    enabled: !!id,
    refetchInterval: 30000,
  });
  if (!id) return null;
  const n = q.data?.unread;
  return (
    <SafeLink
      href="/app/notifications"
      aria-label={`Уведомления${n !== undefined ? `, непрочитанных: ${n}` : ""}`}
      className="notification-bell"
    >
      <Bell size={20} aria-hidden="true" />
      {n !== undefined && n > 0 && <span aria-hidden="true">{n > 99 ? "99+" : n}</span>}
    </SafeLink>
  );
}
export function Notifications() {
  const { id, key, changed } = useNotificationsScope(),
    action = useChallengeAction();
  const [before, setBefore] = useState("");
  const q = useQuery({
    queryKey: [...key, "inbox", before],
    queryFn: ({ signal }) => api.inbox(before, signal),
    enabled: !!id,
    refetchInterval: 5000,
  });
  const reminders = useQuery({
    queryKey: [...key, "reminders"],
    queryFn: ({ signal }) => api.reminders(signal),
    enabled: !!id,
    refetchInterval: 15000,
  });
  return (
    <>
      <h1 className="page-title">Уведомления</h1>
      <p className="page-intro">
        Личная история событий и напоминаний. Вы сами выбираете каналы и время.
      </p>
      <SafeLink href="/app/settings/notifications">Каналы и тихие часы</SafeLink>
      <Card className="notification-card">
        <h2>Входящие</h2>
        <p>
          Непрочитанных сигналов: {q.data?.unread ?? "…"}. История сохраняется и при отключённом
          сигнале.
        </p>
        <Button
          disabled={!q.data || action.pending}
          onClick={() =>
            void action.run(async () => {
              await api.markAll(q.data!.computed_at);
              await changed();
            })
          }
        >
          Отметить просмотренные прочитанными
        </Button>
        <Button variant="secondary" onClick={() => void changed()}>
          Обновить список
        </Button>
        {action.error && <p role="alert">{action.error}</p>}
        {q.isError && <p role="alert">Не удалось загрузить уведомления. Повторите обновление.</p>}
        {q.isPending && <p role="status">Загружаем историю…</p>}
        {q.data?.items.length === 0 && <p>Пока нет уведомлений.</p>}
      </Card>
      {q.data?.items.map((n) => (
        <NotificationRow
          key={n.id}
          item={n}
          reminder={reminders.data?.find((r) => r.id === n.reminder_id)}
          changed={changed}
        />
      ))}
      {before && <Button onClick={() => setBefore("")}>К новым уведомлениям</Button>}
      {q.data?.next && <Button onClick={() => setBefore(q.data!.next)}>Следующие 25</Button>}
    </>
  );
}
function NotificationRow({
  item: n,
  reminder,
  changed,
}: {
  item: api.NotificationItem;
  reminder?: api.Reminder;
  changed: () => Promise<unknown>;
}) {
  const { key } = useNotificationsScope(),
    action = useChallengeAction();
  const [expanded, setExpanded] = useState(false),
    [source, setSource] = useState<{
      available: boolean;
      url: string | null;
      message: string;
    } | null>(null);
  const deliveries = useQuery({
    queryKey: [...key, "deliveries", n.id],
    queryFn: ({ signal }) => api.deliveries(n.id, signal),
    enabled: expanded,
    refetchInterval: expanded ? 5000 : false,
  });
  const ownedReminder = useQuery({
    queryKey: [...key, "reminder", n.reminder_id],
    queryFn: ({ signal }) => api.getReminder(n.reminder_id!, signal),
    enabled: !!n.reminder_id && n.active && !reminder,
  });
  const currentReminder = reminder ?? ownedReminder.data;
  return (
    <Card className="notification-card">
      <article aria-label={n.title}>
        <h2>{n.title}</h2>
        <p>{n.message}</p>
        <p>
          <time dateTime={n.created_at}>{new Date(n.created_at).toLocaleString("ru-RU")}</time> ·{" "}
          {n.read_at ? "Прочитано" : "Не прочитано"}
          {n.muted ? " · Сигнал выключен" : ""}
          {!n.active ? " · Историческая запись" : ""}
        </p>
        <Button
          variant="secondary"
          disabled={action.pending}
          onClick={() =>
            void action.run(async () => {
              await api.mark(n, !n.read_at);
              await changed();
            })
          }
        >
          {n.read_at ? "Отметить непрочитанным" : "Прочитано"}
        </Button>
        <Button
          disabled={action.pending}
          onClick={() =>
            void action.run(async () => {
              const r = await createApiClient().GET("/api/v1/notifications/{id}/source", {
                params: { path: { id: n.id } },
              });
              if (r.data) setSource(r.data.data);
            })
          }
        >
          Проверить источник
        </Button>
        {source &&
          (source.available && source.url ? (
            <SafeLink href={source.url}>Перейти к источнику</SafeLink>
          ) : (
            <p role="status">{source.message}</p>
          ))}
        {action.error && <p role="alert">{action.error}</p>}
        {currentReminder && n.active && <Snooze reminder={currentReminder} changed={changed} />}
        <details onToggle={(e) => setExpanded(e.currentTarget.open)}>
          <summary>История внешних отправок</summary>
          <p>Принятие сервисом не подтверждает получение письма или показ push на устройстве.</p>
          {deliveries.isPending && <p role="status">Загрузка…</p>}
          {deliveries.isError && <p role="alert">Не удалось загрузить отправки.</p>}
          {deliveries.data?.length === 0 && <p>Для этого события внешние отправки не назначены.</p>}
          {deliveries.data?.map((d) => (
            <DeliveryRow key={d.id} delivery={d} changed={changed} />
          ))}
        </details>
      </article>
    </Card>
  );
}
function Snooze({
  reminder: r,
  changed,
}: {
  reminder: api.Reminder;
  changed: () => Promise<unknown>;
}) {
  const action = useChallengeAction();
  const [minutes, setMinutes] = useState(30),
    [ack, setAck] = useState(false),
    [urgent, setUrgent] = useState(false);
  return (
    <fieldset>
      <legend>Напомнить позже</legend>
      <label>
        Через{" "}
        <select value={minutes} onChange={(e) => setMinutes(Number(e.target.value))}>
          {[5, 10, 30, 60, 120].map((m) => (
            <option key={m} value={m}>
              {m} минут
            </option>
          ))}
        </select>
      </label>
      <p>Срок задачи и календарный план сохраняются.</p>
      {r.source_kind === "task" && (
        <label>
          <input type="checkbox" checked={urgent} onChange={(e) => setUrgent(e.target.checked)} />
          Разрешаю этому напоминанию о задаче пройти в тихие часы
        </label>
      )}
      <label>
        <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
        Подтверждаю перенос напоминания
      </label>
      <Button
        disabled={!ack || action.pending}
        onClick={() =>
          void action.run(async () => {
            await api.snooze(r, minutes, urgent);
            setAck(false);
            await changed();
          })
        }
      >
        Отложить напоминание
      </Button>
      {action.error && <p role="alert">{action.error}</p>}
    </fieldset>
  );
}
function DeliveryRow({
  delivery: d,
  changed,
}: {
  delivery: api.Delivery;
  changed: () => Promise<unknown>;
}) {
  const action = useChallengeAction(),
    [ack, setAck] = useState(false);
  return (
    <section>
      <h3>
        {d.channel === "email" ? "Письмо" : "Push"}: {states[d.state] ?? "Обновите список"}
      </h3>
      <p>Попыток: {d.attempts}.</p>
      {d.state === "delivery_unknown" && (
        <p>Сервис мог принять сообщение. Повтор может создать дубль.</p>
      )}
      {(d.state === "failed" || d.state === "delivery_unknown") && (
        <>
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            {d.state === "delivery_unknown"
              ? "Подтверждаю повтор с возможным дублем"
              : "Подтверждаю повтор отправки"}
          </label>
          <Button
            disabled={!ack || action.pending}
            onClick={() =>
              void action.run(async () => {
                await api.retry(d, ack);
                setAck(false);
                await changed();
              })
            }
          >
            Повторить отправку
          </Button>
        </>
      )}
      {action.error && <p role="alert">{action.error}</p>}
    </section>
  );
}
export function NotificationSettings() {
  const { id, key } = useNotificationsScope();
  const q = useQuery({
    queryKey: [...key, "preferences"],
    queryFn: ({ signal }) => api.preferences(signal),
    enabled: !!id,
  });
  return (
    <>
      <h1 className="page-title">Каналы уведомлений</h1>
      <p className="page-intro">Письма и push включаются только по вашему выбору.</p>
      <SafeLink href="/app/notifications">Центр уведомлений</SafeLink>
      {q.isPending && <p role="status">Загружаем настройки…</p>}
      {q.isError && (
        <>
          <p role="alert">Не удалось загрузить настройки.</p>
          <Button onClick={() => void q.refetch()}>Повторить</Button>
        </>
      )}
      {q.data && id && <PreferencesForm key={id} initial={q.data} />}
      <PushDevices />
    </>
  );
}
function PreferencesForm({ initial }: { initial: api.Preferences }) {
  const { changed } = useNotificationsScope(),
    action = useChallengeAction(),
    discard = useConfirmDiscard();
  const [value, setValue] = useState(initial),
    [saved, setSaved] = useState(initial),
    [notice, setNotice] = useState("");
  const dirty = JSON.stringify(value) !== JSON.stringify(saved);
  useUnsavedChanges(dirty);
  const set = (v: Partial<api.PreferencesInput>) => setValue({ ...value, ...v });
  return (
    <Card className="notification-card">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const {
              timezone,
              use_profile_timezone,
              quiet,
              daily_cap,
              deadline_lead_minutes,
              habit_lead_minutes,
              channels,
            } = value;
            const updated = await api.savePreferences(
              {
                timezone,
                use_profile_timezone,
                quiet,
                daily_cap,
                deadline_lead_minutes,
                habit_lead_minutes,
                channels,
              },
              saved.version,
            );
            setValue(updated);
            setSaved(updated);
            setNotice("Настройки сохранены.");
            await changed();
          });
        }}
      >
        <h2>Типы и каналы</h2>
        <p>
          In-app управляет сигналом и счётчиком. Тихие часы и лимит применяются к письмам и push;
          история событий остаётся в центре.
        </p>
        {value.channels.map((c, i) => (
          <fieldset key={c.type}>
            <legend>{names[c.type] ?? c.type}</legend>
            {(["in_app", "email", "push"] as const).map((k) => (
              <label key={k}>
                <input
                  type="checkbox"
                  checked={c[k]}
                  onChange={(e) =>
                    set({
                      channels: value.channels.map((p, j) =>
                        j === i ? { ...p, [k]: e.target.checked } : p,
                      ),
                    })
                  }
                />
                {k === "in_app" ? "Сигнал в приложении" : k === "email" ? "Email" : "Push"}
              </label>
            ))}
          </fieldset>
        ))}
        <h2>Время и частота</h2>
        <label>
          <input
            type="checkbox"
            checked={value.use_profile_timezone}
            onChange={(e) => set({ use_profile_timezone: e.target.checked })}
          />
          Использовать часовой пояс профиля
        </label>
        <p>Сейчас действует: {saved.effective_timezone}</p>
        <label>
          Отдельный часовой пояс
          <input
            value={value.timezone}
            disabled={value.use_profile_timezone}
            onChange={(e) => set({ timezone: e.target.value })}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={value.quiet.enabled}
            onChange={(e) => set({ quiet: { ...value.quiet, enabled: e.target.checked } })}
          />
          Тихие часы
        </label>
        <label>
          Начало тихих часов
          <input
            type="time"
            required
            value={value.quiet.start}
            onChange={(e) => set({ quiet: { ...value.quiet, start: e.target.value } })}
          />
        </label>
        <label>
          Конец тихих часов
          <input
            type="time"
            required
            value={value.quiet.end}
            onChange={(e) => set({ quiet: { ...value.quiet, end: e.target.value } })}
          />
        </label>
        <label>
          Лимит внешних отправок за 24 часа на канал
          <input
            type="number"
            min={0}
            max={50}
            required
            value={value.daily_cap}
            onChange={(e) => set({ daily_cap: Number(e.target.value) })}
          />
        </label>
        <p>Ноль отключает внешние отправки. Все устройства push делят один лимит.</p>
        <label>
          Задачи: за сколько минут до срока
          <input
            type="number"
            min={0}
            max={10080}
            required
            value={value.deadline_lead_minutes}
            onChange={(e) => set({ deadline_lead_minutes: Number(e.target.value) })}
          />
        </label>
        <label>
          Привычки: за сколько минут до конца окна
          <input
            type="number"
            min={0}
            max={1440}
            required
            value={value.habit_lead_minutes}
            onChange={(e) => set({ habit_lead_minutes: Number(e.target.value) })}
          />
        </label>
        <p>
          Для привычек ноль отключает напоминания. Письма отправляются только на подтверждённый
          адрес.
        </p>
        <Button type="submit" disabled={action.pending || !dirty}>
          Сохранить каналы
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={action.pending}
          onClick={() => {
            if (discard())
              void action.run(async () => {
                const latest = await api.preferences();
                setValue(latest);
                setSaved(latest);
                setNotice("");
              });
          }}
        >
          Загрузить сохранённые настройки
        </Button>
        {notice && <p role="status">{notice}</p>}
        {action.error && <p role="alert">{action.error}</p>}
      </form>
    </Card>
  );
}
function PushDevices() {
  const { id, key, changed } = useNotificationsScope(),
    action = useChallengeAction(),
    [notice, setNotice] = useState("");
  const q = useQuery({
    queryKey: [...key, "push"],
    queryFn: ({ signal }) => api.pushDevices(signal),
    enabled: !!id,
  });
  return (
    <Card className="notification-card">
      <h2>Push на устройствах</h2>
      <p>
        Разрешение браузера запрашивается только кнопкой ниже. После подключения включите нужные
        типы push выше.
      </p>
      {q.data && !q.data.configured && (
        <p>Push пока не настроен на сервере. Центр и email доступны.</p>
      )}
      <Button
        disabled={!id || !q.data?.configured || action.pending}
        onClick={() =>
          void action.run(async () => {
            setNotice(await connectPush(id!, q.data!.public_key));
            await changed();
          })
        }
      >
        Подключить push в этом браузере
      </Button>
      {notice && <p role="status">{notice}</p>}
      {action.error && <p role="alert">{action.error}</p>}
      {q.isError && <p role="alert">Не удалось загрузить устройства.</p>}
      {q.data?.items.length === 0 && <p>Подключённых устройств нет.</p>}
      {q.data?.items.map((d) => (
        <Device key={d.id} device={d} changed={changed} />
      ))}
    </Card>
  );
}
function Device({
  device: d,
  changed,
}: {
  device: {
    id: string;
    provider: string;
    version: number;
    revoked_at: string | null;
    expires_at: string;
  };
  changed: () => Promise<unknown>;
}) {
  const [ack, setAck] = useState(false),
    action = useChallengeAction();
  return (
    <section>
      <h3>{d.provider}</h3>
      <p>
        {d.revoked_at
          ? "Отключено"
          : `Подписка до ${new Date(d.expires_at).toLocaleDateString("ru-RU")}`}
      </p>
      {!d.revoked_at && (
        <>
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
            Подтверждаю отключение этого устройства
          </label>
          <Button
            disabled={!ack || action.pending}
            onClick={() =>
              void action.run(async () => {
                await api.revokeDevice(d.id, d.version);
                await changed();
              })
            }
          >
            Отключить устройство
          </Button>
        </>
      )}
      {action.error && <p role="alert">{action.error}</p>}
    </section>
  );
}
