"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Users,
  HeartHandshake,
  Trophy,
  Sparkles,
  ShieldCheck,
  Copy,
  Plus,
  Check,
  ArrowRight,
  BookOpen,
  Timer,
  ListTodo,
  Moon,
  Flag,
  LockKeyhole,
  RefreshCw,
  Medal,
} from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import { loadSocial, mutateSocial } from "@/lib/social";
import { type SocialCommand, type SocialDashboard, type SocialRoom } from "@/lib/social-schema";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { ChoiceCards } from "@/components/ui/choice-cards";
import { ChoiceChips, EmptyScene, StatTile } from "@/components/ui/experience";
import { MeterRing } from "@/components/ui/meter-ring";
import { SafeLink, useUnsavedChanges } from "@/lib/forms/unsaved-changes";

type RoomDraft = { amount?: number; invite?: string; consent?: boolean; board?: boolean };
type Act = (command: SocialCommand, message?: string) => Promise<boolean>;
const units: Record<string, string> = {
  tasks: "задач",
  minutes: "мин",
  pages: "стр.",
  count: "раз",
  reviews: "разборов",
};
const templates = {
  small_step: "Один небольшой шаг — уже движение вперёд",
  rest: "Пауза тоже часть пути. Береги силы",
  celebrate: "Замечаю твои усилия. Продолжай в своём темпе",
};
const reviewStates = {
  pending: "Ждёт партнёра",
  accepted: "Подтверждено партнёром",
  rejected: "Не подтверждено",
  disputed: "Есть спор",
  withdrawn: "Доступ отозван",
};
const date = (value: string) =>
  new Intl.DateTimeFormat("ru-RU", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "UTC",
  }).format(new Date(value));
function Avatar({ name, tone = "violet" }: { name: string; tone?: string }) {
  return (
    <span className="social-avatar" data-tone={tone} aria-hidden="true">
      {name.slice(0, 2).toUpperCase()}
    </span>
  );
}

export function FriendsWorkspace() {
  const { session } = useAuth();
  const owner = session?.account.id;
  const [data, setData] = useState<SocialDashboard>();
  const [serverTime, setServerTime] = useState("");
  const [error, setError] = useState("");
  const [loadError, setLoadError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState("friends");
  const [code, setCode] = useState("");
  const [create, setCreate] = useState(false);
  const [roomDrafts, setRoomDrafts] = useState<Record<string, RoomDraft>>({});
  const [communityDrafts, setCommunityDrafts] = useState<
    Record<string, { recipient?: string; template?: keyof typeof templates; achievement?: string }>
  >({});
  const communityDraft = communityDrafts[owner ?? ""] ?? {};
  function updateCommunityDraft(patch: typeof communityDraft) {
    if (owner)
      setCommunityDrafts((previous) => ({
        ...previous,
        [owner]: { ...previous[owner], ...patch },
      }));
  }
  useUnsavedChanges(
    Object.entries(roomDrafts).some(
      ([key, draft]) => key.startsWith(`${owner}:`) && draft.amount !== undefined,
    ),
  );
  const [confirmation, setConfirmation] = useState<{
    title: string;
    description: string;
    command: SocialCommand;
  }>();
  const [reviewRoom, setReviewRoom] = useState<string>();
  const [reviewer, setReviewer] = useState("");
  const [file, setFile] = useState("");
  const [reviewConsent, setReviewConsent] = useState(false);
  const [grantFriend, setGrantFriend] = useState<string>();
  const [challengeID, setChallengeID] = useState("");
  const [personalRuns, setPersonalRuns] = useState<{ id: string; name: string }[]>([]);
  const [report, setReport] = useState<{ target?: string; id?: string }>();
  const [reason, setReason] = useState<"spam" | "abuse" | "privacy" | "other">("spam");
  const request = useRef<AbortController | null>(null);
  const sequence = useRef(0);
  const saving = useRef(false);
  const retry = useRef<{ body: string; key: string } | null>(null);
  const refresh = useCallback(async (clear = true) => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const ticket = ++sequence.current;
    if (clear) setData(undefined);
    try {
      const next = await loadSocial(controller.signal);
      if (ticket !== sequence.current || controller.signal.aborted) return;
      setData(next.data);
      setServerTime(next.meta.server_time);
      setLoadError("");
    } catch (e) {
      if (controller.signal.aborted || ticket !== sequence.current || controller.signal.aborted)
        return;
      setData(undefined);
      setLoadError(
        e instanceof ApiError ? e.message : "Не удалось обновить раздел. Проверьте связь.",
      );
    }
  }, []);
  useEffect(() => {
    if (!owner) return;
    let stopped = false,
      stream: EventSource | undefined,
      timer: ReturnType<typeof setTimeout> | undefined,
      last = "";
    const initial = setTimeout(() => void refresh(), 0);
    const connect = () => {
      if (stopped) return;
      stream = new EventSource("/api/v1/social/changes");
      stream.onmessage = (event) => {
        if (event.data !== last) {
          last = event.data;
          void refresh();
        }
      };
      stream.addEventListener("reconnect", () => {
        stream?.close();
        timer = setTimeout(connect, 150);
      });
      stream.onerror = () => {
        stream?.close();
        setData(undefined);
        setLoadError("Связь прервалась. Общие данные скрыты до обновления.");
        timer = setTimeout(() => {
          void refresh();
          connect();
        }, 2000);
      };
    };
    connect();
    // Recorded progress can change in another section without a social command.
    const progressTimer = setInterval(() => {
      if (!document.hidden && !saving.current) void refresh(false);
    }, 30000);
    const visible = () => {
      if (!document.hidden) void refresh();
      else {
        sequence.current++;
        request.current?.abort();
        setData(undefined);
      }
    };
    document.addEventListener("visibilitychange", visible);
    return () => {
      stopped = true;
      request.current?.abort();
      stream?.close();
      clearTimeout(timer);
      clearTimeout(initial);
      clearInterval(progressTimer);
      document.removeEventListener("visibilitychange", visible);
    };
  }, [owner, refresh]);
  const act: Act = useCallback(
    async (command, message = "Готово. Изменения сохранены.") => {
      if (saving.current) return false;
      saving.current = true;
      setBusy(true);
      setNotice("");
      setError("");
      // Show the user's own switch immediately; a failed write reloads authority.
      setData((previous) => {
        if (!previous) return previous;
        if (command.section === "grants") {
          const input = command.input;
          const grants = previous.grants.filter(
            (g) =>
              !(
                g.owner === previous.owner &&
                g.recipient === input.recipient &&
                g.metric === input.metric &&
                g.scope === input.scope
              ),
          );
          if (input.allow)
            grants.push({
              owner: previous.owner,
              recipient: input.recipient,
              metric: input.metric,
              scope: input.scope,
              value: 0,
              unit: "",
            });
          return { ...previous, grants };
        }
        if (command.section === "community" && command.input.action === "preferences")
          return {
            ...previous,
            community: {
              ...previous.community,
              partner: command.input.recipient || null,
              nudges: !!command.input.enabled,
            },
          };
        if (command.section === "rooms" && command.input.action === "sharing")
          return {
            ...previous,
            rooms: previous.rooms.map((room) =>
              room.id === command.input.id
                ? {
                    ...room,
                    share: !!command.input.consent,
                    leaderboard: !!command.input.leaderboard && !!command.input.consent,
                  }
                : room,
            ),
          };
        return previous;
      });
      const body = JSON.stringify(command);
      if (retry.current?.body !== body) retry.current = { body, key: crypto.randomUUID() };
      try {
        await mutateSocial(command, retry.current.key);
        retry.current = null;
        setNotice(message);
        await refresh();
        return true;
      } catch (e) {
        await refresh();
        setError(
          e instanceof ApiError
            ? ({
                EMAIL_VERIFICATION_REQUIRED: "Сначала подтвердите почту в настройках безопасности.",
                NUDGES_DISABLED: "Друг ещё не разрешил поддержку от вас.",
                SOCIAL_CONFLICT: "Данные изменились. Обновите раздел и повторите действие.",
                NOT_FOUND: "Доступ к этой записи уже закрыт.",
              }[e.code] ?? e.message)
            : "Не удалось сохранить. Ваш ввод сохранён — повторите попытку.",
        );
        return false;
      } finally {
        saving.current = false;
        setBusy(false);
      }
    },
    [refresh],
  );
  const current = data?.owner === owner ? data : undefined;
  const friends = current?.friends.items.filter((f) => f.state === "accepted") ?? [];
  const confirm = (title: string, description: string, command: SocialCommand) =>
    setConfirmation({ title, description, command });
  const chosenRoom = current?.rooms.find((r) => r.id === reviewRoom);
  useEffect(() => {
    if (!grantFriend) return;
    const controller = new AbortController();
    void fetch("/api/v1/challenges/runs", { cache: "no-store", signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error();
        const value: unknown = await response.json();
        const { z } = await import("zod");
        const parsed = z
          .object({
            data: z.object({
              items: z.array(z.object({ id: z.uuid(), rules: z.object({ name: z.string() }) })),
            }),
          })
          .parse(value);
        setPersonalRuns(parsed.data.items.map((r) => ({ id: r.id, name: r.rules.name })));
      })
      .catch(() => {
        if (!controller.signal.aborted) setPersonalRuns([]);
      });
    return () => controller.abort();
  }, [grantFriend]);
  return (
    <div className="social-workspace">
      <header className="social-hero">
        <div>
          <p className="eyebrow">ВАША КОМАНДА</p>
          <h1 className="page-title">Вместе — дальше</h1>
          <p>Свои люди. Общий ритм. Маленькие победы, которые хочется разделить.</p>
          <div className="social-hero-tags">
            <span>
              <ShieldCheck size={15} /> Только по согласию
            </span>
            <span>
              <HeartHandshake size={15} /> Поддержка без давления
            </span>
          </div>
        </div>
        <div className="social-constellation" aria-hidden="true">
          <i />
          <i />
          <span>
            <Users size={42} />
          </span>
          <b>
            <Sparkles size={22} />
          </b>
          <b>
            <HeartHandshake size={24} />
          </b>
          <b>
            <Trophy size={23} />
          </b>
        </div>
      </header>
      <div className="social-toolbar">
        <ChoiceChips
          label="Раздел сообщества"
          value={tab}
          onChange={setTab}
          options={[
            { value: "friends", label: "Мои люди" },
            { value: "rooms", label: "Общие испытания" },
            { value: "feed", label: "Поддержка и победы" },
          ]}
        />
        <Button variant="ghost" size="small" onClick={() => void refresh()} disabled={busy}>
          <RefreshCw size={16} /> Обновить
        </Button>
      </div>
      {(error || loadError) && (
        <p className="ui-alert" role="alert">
          {error || loadError}
        </p>
      )}
      {notice && (
        <p className="social-notice" role="status">
          <Check size={18} />
          {notice}
        </p>
      )}
      {!current && (
        <Card>
          <p role="status">
            {error
              ? "Общие данные появятся после восстановления связи."
              : "Собираем ваше пространство…"}
          </p>
        </Card>
      )}
      {current && (
        <>
          {!current.friends.verified && (
            <Card className="social-verification">
              <LockKeyhole />
              <div>
                <strong>Подтвердите почту, чтобы приглашать друзей</strong>
                <p>Ваши личные разделы уже доступны.</p>
              </div>
              <SafeLink href="/app/settings/security">
                Подтвердить почту <ArrowRight size={16} />
              </SafeLink>
            </Card>
          )}
          <fieldset disabled={busy} className="social-controls">
            {tab === "friends" && (
              <>
                <div className="social-stats">
                  <StatTile
                    label="Ваша команда"
                    value={friends.length}
                    detail="подтверждённых друзей"
                    icon={<Users />}
                    tone="violet"
                  />
                  <StatTile
                    label="Общий путь"
                    value={current.rooms.filter((r) => r.state === "joined").length}
                    detail="совместных испытаний"
                    icon={<Flag />}
                    tone="mint"
                  />
                  <StatTile
                    label="Открытые показатели"
                    value={current.grants.filter((g) => g.owner === owner).length}
                    detail="разрешения под вашим контролем"
                    icon={<ShieldCheck />}
                    tone="blue"
                  />
                </div>
                <Card className="social-invite">
                  <div>
                    <p className="eyebrow">ПЕРВЫЙ ШАГ — ПРИВЕТ</p>
                    <h2>Пригласите своего человека</h2>
                    <p className="ui-hint">
                      Передайте этот код другу. Дружба появится после его подтверждения.
                    </p>
                    <div className="social-code">
                      <output aria-label="Мой код приглашения">{current.friends.code}</output>
                      <Button
                        variant="secondary"
                        size="small"
                        onClick={async () => {
                          try {
                            await navigator.clipboard.writeText(current.friends.code);
                            setNotice("Код скопирован.");
                          } catch {
                            setError(
                              "Не удалось скопировать. Выделите код и скопируйте его вручную.",
                            );
                          }
                        }}
                      >
                        <Copy size={16} /> Копировать
                      </Button>
                    </div>
                    <Button
                      variant="ghost"
                      size="small"
                      onClick={() =>
                        confirm(
                          "Сменить код?",
                          "Старый код перестанет принимать новые приглашения. Друзья останутся.",
                          { section: "friends", input: { action: "rotate" } },
                        )
                      }
                    >
                      Сменить код
                    </Button>
                  </div>
                  <form
                    onSubmit={async (event) => {
                      event.preventDefault();
                      if (
                        await act(
                          { section: "friends", input: { action: "request", code: code.trim() } },
                          "Приглашение отправлено. Ждём ответа друга.",
                        )
                      )
                        setCode("");
                    }}
                  >
                    <label htmlFor="friend-code">У вас уже есть код друга?</label>
                    <input
                      className="ui-input"
                      id="friend-code"
                      required
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      placeholder="Вставьте код приглашения"
                      autoComplete="off"
                      maxLength={36}
                    />
                    <Button type="submit" disabled={!current.friends.verified || !code.trim()}>
                      <Plus size={17} /> Отправить приглашение
                    </Button>
                  </form>
                </Card>
                {current.friends.items.some((f) => f.state === "pending") && (
                  <section aria-labelledby="social-invites-title">
                    <h2 id="social-invites-title">Ожидают ответа</h2>
                    <div className="social-people">
                      {current.friends.items
                        .filter((f) => f.state === "pending")
                        .map((f) => (
                          <Card key={f.id} className="social-person">
                            <Avatar name={f.nickname} tone="blue" />
                            <div>
                              <h3>{f.nickname}</h3>
                              <p className="ui-hint">
                                {f.incoming
                                  ? "Хочет добавить вас в друзья"
                                  : "Приглашение отправлено"}
                              </p>
                            </div>
                            <div className="social-actions">
                              {f.incoming && (
                                <Button
                                  size="small"
                                  onClick={() =>
                                    void act(
                                      {
                                        section: "friends",
                                        input: { action: "accept", id: f.id, version: f.version },
                                      },
                                      "Теперь вы друзья. Показатели пока закрыты.",
                                    )
                                  }
                                >
                                  Принять дружбу
                                </Button>
                              )}
                              <Button
                                size="small"
                                variant="ghost"
                                onClick={() =>
                                  void act({
                                    section: "friends",
                                    input: {
                                      action: f.incoming ? "reject" : "remove",
                                      id: f.id,
                                      version: f.version,
                                    },
                                  })
                                }
                              >
                                {f.incoming ? "Отклонить" : "Отменить приглашение"}
                              </Button>
                              {f.incoming && (
                                <>
                                  <Button
                                    size="small"
                                    variant="ghost"
                                    onClick={() => {
                                      setReport({ target: f.user_id });
                                      setReason("spam");
                                    }}
                                  >
                                    Пожаловаться
                                  </Button>
                                  <Button
                                    size="small"
                                    variant="danger"
                                    onClick={() =>
                                      confirm(
                                        "Заблокировать участника?",
                                        "Приглашение будет отменено. Новые приглашения от этого человека будут закрыты.",
                                        {
                                          section: "moderation",
                                          input: { action: "block", target: f.user_id },
                                        },
                                      )
                                    }
                                  >
                                    Заблокировать
                                  </Button>
                                </>
                              )}
                            </div>
                          </Card>
                        ))}
                    </div>
                  </section>
                )}
                <section aria-labelledby="social-friends-title">
                  <div className="social-section-heading">
                    <h2 id="social-friends-title">
                      Мои люди <span>{friends.length}</span>
                    </h2>
                    <SafeLink href="/app/settings/profile">
                      Мой псевдоним: {current.nickname}
                    </SafeLink>
                  </div>
                  {friends.length === 0 ? (
                    <EmptyScene
                      title="Здесь появится ваша команда"
                      description="Достаточно одного человека, с которым приятно двигаться вперёд."
                      tone="mint"
                    />
                  ) : (
                    <div className="social-people">
                      {friends.map((f, index) => (
                        <Card key={f.id} className="social-person">
                          <Avatar name={f.nickname} tone={index % 2 ? "mint" : "violet"} />
                          <div>
                            <h3>{f.nickname}</h3>
                            <span className="social-badge">
                              <Check size={13} /> Подтверждённый друг
                            </span>
                          </div>
                          <div className="social-shared">
                            {current.grants
                              .filter((g) => g.owner === f.user_id)
                              .map((g) => (
                                <div key={g.metric + g.scope}>
                                  <strong>
                                    {g.metric === "evening"
                                      ? g.value
                                        ? "Готово"
                                        : "Пока нет"
                                      : `${Math.round(g.value)}${g.metric === "challenge" ? "%" : " / 7"}`}
                                  </strong>
                                  <span>
                                    {g.metric === "regularity"
                                      ? "активных дней · UTC"
                                      : g.metric === "evening"
                                        ? "вечерний разбор · UTC"
                                        : "личное испытание"}
                                  </span>
                                </div>
                              ))}
                            {!current.grants.some((g) => g.owner === f.user_id) && (
                              <p className="ui-hint">
                                <LockKeyhole size={14} /> Показатели пока личные
                              </p>
                            )}
                          </div>
                          <Button
                            variant="secondary"
                            onClick={() => {
                              setGrantFriend(f.user_id);
                              setChallengeID("");
                            }}
                          >
                            Настроить мой доступ
                          </Button>
                          <details className="social-details">
                            <summary>Действия с другом</summary>
                            <Button
                              variant="ghost"
                              size="small"
                              onClick={() => {
                                setReport({ target: f.user_id });
                                setReason("spam");
                              }}
                            >
                              Пожаловаться
                            </Button>
                            <Button
                              variant="ghost"
                              size="small"
                              onClick={() =>
                                confirm(
                                  "Удалить из друзей?",
                                  "Общие разрешения и приглашения будут отозваны. Новая дружба потребует согласия.",
                                  {
                                    section: "friends",
                                    input: { action: "remove", id: f.id, version: f.version },
                                  },
                                )
                              }
                            >
                              Удалить друга
                            </Button>
                            <Button
                              variant="danger"
                              size="small"
                              onClick={() =>
                                confirm(
                                  "Заблокировать участника?",
                                  "Дружба и доступ будут отозваны. Вы выйдете из общих испытаний с этим человеком.",
                                  {
                                    section: "moderation",
                                    input: { action: "block", target: f.user_id },
                                  },
                                )
                              }
                            >
                              Заблокировать
                            </Button>
                          </details>
                        </Card>
                      ))}
                    </div>
                  )}
                </section>
                {current.moderation.blocks.length > 0 && (
                  <Card>
                    <h2>Заблокированные</h2>
                    {current.moderation.blocks.map((b) => (
                      <div className="social-list-row" key={b.id}>
                        <span>{b.nickname}</span>
                        <Button
                          variant="ghost"
                          size="small"
                          onClick={() =>
                            void act(
                              { section: "moderation", input: { action: "unblock", target: b.id } },
                              "Блокировка снята. Дружба и доступ не восстановлены.",
                            )
                          }
                        >
                          Разблокировать
                        </Button>
                      </div>
                    ))}
                  </Card>
                )}
              </>
            )}
            {tab === "rooms" && (
              <>
                <div className="social-section-heading">
                  <div>
                    <h2>Маленькие шаги. Общая вершина.</h2>
                    <p className="ui-hint">
                      Вместе к цели или дружеское соревнование на равных условиях.
                    </p>
                  </div>
                  <Button onClick={() => setCreate(true)} disabled={!current.friends.verified}>
                    <Plus size={17} /> Новое общее испытание
                  </Button>
                </div>
                {current.rooms.length === 0 ? (
                  <EmptyScene
                    title="Ваш первый общий вызов"
                    description="Читайте, фокусируйтесь или закрывайте задачи. Выберите посильный темп и пригласите друга."
                    tone="gold"
                  >
                    <Button onClick={() => setCreate(true)} disabled={!current.friends.verified}>
                      Создать общее испытание
                    </Button>
                  </EmptyScene>
                ) : (
                  <div className="social-rooms">
                    {current.rooms.map((room) => (
                      <RoomCard
                        key={room.id}
                        room={room}
                        draft={roomDrafts[`${owner}:${room.id}`] ?? {}}
                        updateDraft={(patch) =>
                          setRoomDrafts((previous) => ({
                            ...previous,
                            [`${owner}:${room.id}`]: {
                              ...previous[`${owner}:${room.id}`],
                              ...patch,
                            },
                          }))
                        }
                        owner={owner!}
                        friends={friends}
                        act={act}
                        now={serverTime}
                        requestReview={() => {
                          setReviewRoom(room.id);
                          setReviewer("");
                          setFile("");
                          setReviewConsent(false);
                        }}
                        confirm={confirm}
                      />
                    ))}
                  </div>
                )}
                {current.reviews.length > 0 && (
                  <section aria-labelledby="reviews-title">
                    <h2 id="reviews-title">Подтверждения партнёра</h2>
                    <div className="social-review-grid">
                      {current.reviews.map((review) => (
                        <ReviewCard key={review.id} review={review} owner={owner!} act={act} />
                      ))}
                    </div>
                  </section>
                )}
              </>
            )}
            {tab === "feed" && (
              <>
                <div className="social-feed-layout">
                  <Card>
                    <p className="eyebrow">НА ОДНОЙ СТОРОНЕ</p>
                    <h2>Партнёр по ритму</h2>
                    <p className="ui-hint">
                      Выберите, от кого хотите получать поддержку. Не чаще одного обращения за 24
                      часа.
                    </p>
                    <label htmlFor="social-partner">Мой партнёр</label>
                    <select
                      className="ui-input"
                      id="social-partner"
                      value={current.community.partner ?? ""}
                      onChange={(e) =>
                        void act({
                          section: "community",
                          input: {
                            action: "preferences",
                            recipient: e.target.value,
                            enabled: false,
                            version: current.community.version,
                          },
                        })
                      }
                    >
                      <option value="">Пока без партнёра</option>
                      {friends.map((f) => (
                        <option key={f.id} value={f.user_id}>
                          {f.nickname}
                        </option>
                      ))}
                    </select>
                    <label className="social-toggle">
                      <input
                        type="checkbox"
                        checked={current.community.nudges}
                        disabled={!current.community.partner}
                        onChange={(e) =>
                          void act({
                            section: "community",
                            input: {
                              action: "preferences",
                              recipient: current.community.partner ?? "",
                              enabled: e.target.checked,
                              version: current.community.version,
                            },
                          })
                        }
                      />
                      <span>
                        <strong>Принимать поддержку</strong>
                        <small>Три добрых шаблона. Без свободного чата.</small>
                      </span>
                    </label>
                    {current.community.inbox.map((n) => (
                      <div className="social-nudge" key={n.id}>
                        <HeartHandshake size={20} />
                        <div>
                          <strong>{n.nickname}</strong>
                          <p>{templates[n.template]}</p>
                        </div>
                      </div>
                    ))}
                  </Card>
                  <Card>
                    <p className="eyebrow">ПОДДЕРЖАТЬ ДРУГА</p>
                    <h2>Иногда достаточно пары слов</h2>
                    <p className="ui-hint">
                      Сработает, если друг выбрал вас партнёром и включил поддержку.
                    </p>
                    <NudgeComposer
                      friends={friends}
                      act={act}
                      recipient={communityDraft.recipient ?? ""}
                      template={communityDraft.template ?? "small_step"}
                      setRecipient={(recipient) => updateCommunityDraft({ recipient })}
                      setTemplate={(template) => updateCommunityDraft({ template })}
                    />
                  </Card>
                </div>
                <Card>
                  <div className="social-section-heading">
                    <div>
                      <p className="eyebrow">ДЕЛИТЕСЬ ТЕМ, ЧТО ВАЖНО</p>
                      <h2>Победы вашей команды</h2>
                    </div>
                    <Medal className="social-gold" size={36} />
                  </div>
                  <PublishComposer
                    data={current}
                    act={act}
                    selected={communityDraft.achievement ?? ""}
                    setSelected={(achievement) => updateCommunityDraft({ achievement })}
                  />
                  {current.community.feed.length === 0 ? (
                    <EmptyScene
                      title="Победы любят хорошую компанию"
                      description="Заработанное достижение появится здесь только после отдельной публикации."
                      tone="gold"
                    />
                  ) : (
                    <div className="social-feed">
                      {current.community.feed.map((post) => (
                        <article key={post.id}>
                          <Avatar name={post.nickname} tone="gold" />
                          <div>
                            <small>
                              {post.nickname} · {date(post.created_at)} UTC
                            </small>
                            <h3>{post.title}</h3>
                            <span className="social-badge" data-tone="gold">
                              <Trophy size={14} /> Достижение
                            </span>
                          </div>
                          {post.owner === owner ? (
                            <Button
                              variant="ghost"
                              size="small"
                              onClick={() =>
                                void act({
                                  section: "moderation",
                                  input: { action: "hide", id: post.id },
                                })
                              }
                            >
                              Скрыть публикацию
                            </Button>
                          ) : (
                            <Button
                              variant="ghost"
                              size="small"
                              onClick={() => {
                                setReport({ id: post.id });
                                setReason("spam");
                              }}
                            >
                              Пожаловаться
                            </Button>
                          )}
                        </article>
                      ))}
                    </div>
                  )}
                </Card>
                {current.moderation.reports.length > 0 && (
                  <Card>
                    <h2>{current.moderation.admin ? "Очередь модерации" : "Ваши обращения"}</h2>
                    {current.moderation.reports.map((r) => (
                      <div className="social-list-row" key={r.id}>
                        <span>
                          {
                            (
                              {
                                spam: "Спам",
                                abuse: "Оскорбления",
                                privacy: "Нарушение приватности",
                                other: "Другая причина",
                              } as Record<string, string>
                            )[r.reason]
                          }{" "}
                          · {r.resolved ? "Рассмотрено" : "На рассмотрении"}
                        </span>
                        {current.moderation.admin && !r.resolved && (
                          <Button
                            variant="secondary"
                            size="small"
                            onClick={() =>
                              void act({
                                section: "moderation",
                                input: { action: "resolve", id: r.id },
                              })
                            }
                          >
                            Завершить рассмотрение
                          </Button>
                        )}
                        {current.moderation.admin && r.post_id && !r.resolved && (
                          <Button
                            variant="danger"
                            size="small"
                            onClick={() =>
                              void act({
                                section: "moderation",
                                input: { action: "hide", id: r.post_id! },
                              })
                            }
                          >
                            Скрыть материал
                          </Button>
                        )}
                      </div>
                    ))}
                  </Card>
                )}
              </>
            )}
          </fieldset>
        </>
      )}
      <RoomCreator open={create} onOpenChange={setCreate} act={act} busy={busy} error={error} />
      <Dialog
        open={!!confirmation}
        onOpenChange={(open) => {
          if (!open && !busy) setConfirmation(undefined);
        }}
        title={confirmation?.title ?? "Подтвердите действие"}
        description={confirmation?.description ?? ""}
      >
        <SocialError message={error} />
        <div className="social-actions">
          <Button variant="secondary" disabled={busy} onClick={() => setConfirmation(undefined)}>
            Отмена
          </Button>
          <Button
            disabled={busy}
            onClick={async () => {
              if (confirmation && (await act(confirmation.command))) setConfirmation(undefined);
            }}
          >
            Подтвердить действие
          </Button>
        </div>
      </Dialog>
      <Dialog
        open={!!grantFriend}
        onOpenChange={(open) => {
          if (!open) setGrantFriend(undefined);
        }}
        title="Что видит этот друг"
        description="Каждый показатель включается отдельно. Названия задач и текст дневника остаются личными."
      >
        <SocialError message={error} />
        {current && grantFriend && (
          <fieldset disabled={busy} className="social-controls">
            {(["regularity", "evening"] as const).map((metric) => (
              <label className="social-toggle" key={metric}>
                <input
                  type="checkbox"
                  checked={current.grants.some(
                    (g) => g.owner === owner && g.recipient === grantFriend && g.metric === metric,
                  )}
                  onChange={(e) =>
                    void act({
                      section: "grants",
                      input: { recipient: grantFriend, metric, scope: "", allow: e.target.checked },
                    })
                  }
                />
                <span>
                  <strong>
                    {metric === "regularity" ? "Регулярность за 7 дней" : "Факт вечернего разбора"}
                  </strong>
                  <small>
                    {metric === "regularity"
                      ? "Количество активных дней по заработанным наградам · UTC"
                      : "Только «готово / пока нет» за сегодня UTC"}
                  </small>
                </span>
              </label>
            ))}
            <label htmlFor="social-personal-run">Прогресс конкретного личного испытания</label>
            <select
              className="ui-input"
              id="social-personal-run"
              value={challengeID}
              onChange={(e) => setChallengeID(e.target.value)}
            >
              <option value="">Выберите испытание</option>
              {personalRuns.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
            <Button
              disabled={!challengeID}
              onClick={() =>
                void act({
                  section: "grants",
                  input: {
                    recipient: grantFriend,
                    metric: "challenge",
                    scope: challengeID,
                    allow: true,
                  },
                })
              }
            >
              Разрешить только процент прогресса
            </Button>
            {current.grants
              .filter(
                (g) => g.owner === owner && g.recipient === grantFriend && g.metric === "challenge",
              )
              .map((g) => (
                <div className="social-list-row" key={g.scope}>
                  <span>
                    {personalRuns.find((r) => r.id === g.scope)?.name ?? "Личное испытание"}
                  </span>
                  <Button
                    variant="ghost"
                    size="small"
                    onClick={() =>
                      void act({
                        section: "grants",
                        input: {
                          recipient: grantFriend,
                          metric: "challenge",
                          scope: g.scope,
                          allow: false,
                        },
                      })
                    }
                  >
                    Отозвать доступ
                  </Button>
                </div>
              ))}
          </fieldset>
        )}
      </Dialog>
      <Dialog
        open={!!reviewRoom}
        onOpenChange={(open) => {
          if (!open) setReviewRoom(undefined);
        }}
        title="Попросить подтверждение"
        description="Партнёр увидит дневную сумму и выбранный файл. Самоотчёт станет подтверждённым только после его решения."
      >
        <SocialError message={error} />
        {current && chosenRoom?.checkin && (
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              const previous = current.reviews.find(
                (r) => r.owner === owner && r.checkin_id === chosenRoom.checkin!.id,
              );
              if (
                await act({
                  section: "reviews",
                  input: {
                    action: "request",
                    id: chosenRoom.checkin!.id,
                    recipient: reviewer,
                    attachment_id: file,
                    consent: reviewConsent,
                    version: previous?.version ?? 0,
                  },
                })
              )
                setReviewRoom(undefined);
            }}
          >
            <fieldset className="social-controls social-form" disabled={busy}>
              <label htmlFor="social-reviewer">Кто подтвердит?</label>
              <select
                className="ui-input"
                id="social-reviewer"
                required
                value={reviewer}
                onChange={(e) => setReviewer(e.target.value)}
              >
                <option value="">Выберите друга из этого испытания</option>
                {friends
                  .filter((f) => chosenRoom.members.some((m) => m.user_id === f.user_id))
                  .map((f) => (
                    <option key={f.id} value={f.user_id}>
                      {f.nickname}
                    </option>
                  ))}
              </select>
              <label htmlFor="social-evidence">Приватное доказательство</label>
              <select
                className="ui-input"
                id="social-evidence"
                value={file}
                onChange={(e) => setFile(e.target.value)}
              >
                <option value="">Без файла</option>
                {current.files.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.name}
                  </option>
                ))}
              </select>
              <p className="ui-hint">
                Здесь доступны ваши проверенные вложения из задач. Передавайте только файл, которым
                готовы поделиться.
              </p>
              <label className="social-toggle">
                <input
                  type="checkbox"
                  required
                  checked={reviewConsent}
                  onChange={(e) => setReviewConsent(e.target.checked)}
                />
                <span>Разрешаю выбранному партнёру видеть отметку и этот файл</span>
              </label>
              <Button type="submit" disabled={!reviewer || !reviewConsent}>
                Отправить на подтверждение
              </Button>
            </fieldset>
          </form>
        )}
      </Dialog>
      <Dialog
        open={!!report}
        onOpenChange={(open) => {
          if (!open) setReport(undefined);
        }}
        title="Сообщить о проблеме"
        description="Обращение увидит модератор. Оно не публикуется в ленте."
      >
        <SocialError message={error} />
        <ChoiceChips
          label="Причина обращения"
          value={reason}
          onChange={setReason}
          options={[
            { value: "spam", label: "Спам" },
            { value: "abuse", label: "Оскорбления" },
            { value: "privacy", label: "Приватность" },
            { value: "other", label: "Другое" },
          ]}
        />
        <Button
          disabled={busy}
          onClick={async () => {
            if (
              report &&
              (await act(
                { section: "moderation", input: { action: "report", ...report, reason } },
                "Обращение отправлено модератору.",
              ))
            )
              setReport(undefined);
          }}
        >
          Отправить обращение
        </Button>
      </Dialog>
    </div>
  );
}

function RoomCard({
  room,
  draft,
  updateDraft,
  owner,
  friends,
  act,
  now,
  requestReview,
  confirm,
}: {
  room: SocialRoom;
  draft: RoomDraft;
  updateDraft: (patch: RoomDraft) => void;
  owner: string;
  friends: SocialDashboard["friends"]["items"];
  act: Act;
  now: string;
  requestReview: () => void;
  confirm: (title: string, description: string, command: SocialCommand) => void;
}) {
  const { consent = false, board = false, invite = "", amount } = draft;
  const setConsent = (consent: boolean) => updateDraft({ consent });
  const setBoard = (board: boolean) => updateDraft({ board });
  const setInvite = (invite: string) => updateDraft({ invite });
  const setAmount = (amount: number | undefined) => updateDraft({ amount });
  const ended = Date.parse(now) >= Date.parse(room.ends_at),
    future = Date.parse(now) < Date.parse(room.starts_at);
  const value = amount ?? room.checkin?.amount ?? 0;
  const availableFriends = friends.filter(
    (friend) => !room.members.some((member) => member.user_id === friend.user_id),
  );
  return (
    <Card className="social-room" data-mode={room.mode} aria-label={room.name}>
      <div className="social-room-top">
        <span className="social-badge" data-tone={room.mode === "coop" ? "mint" : "gold"}>
          {room.mode === "coop" ? <HeartHandshake size={15} /> : <Trophy size={15} />}
          {room.mode === "coop" ? "Общая цель" : "Дружеский PvP"}
        </span>
        <span className="ui-hint">
          {room.state === "invited"
            ? "Вас пригласили"
            : room.finalized_at
              ? "Итоги подведены"
              : ended
                ? "Готово к итогам"
                : future
                  ? "Скоро старт"
                  : "В пути"}
        </span>
      </div>
      <div className="social-room-summary">
        <div>
          <h3>{room.name}</h3>
          <p>
            {room.target} {units[room.unit]} · до {room.daily_cap} в день на человека
          </p>
          <small>
            {date(room.starts_at)} — {date(room.ends_at)} UTC
          </small>
        </div>
        {room.state === "joined" && (
          <MeterRing
            value={
              (100 *
                (room.mode === "pvp"
                  ? (room.members.find((m) => m.user_id === owner)?.amount ?? 0)
                  : room.total)) /
              room.target
            }
            label={`Прогресс: ${room.name}`}
          />
        )}
      </div>
      <details className="social-details">
        <summary>
          <ShieldCheck size={15} /> Правила зафиксированы
        </summary>
        <p>
          {room.metric === "manual"
            ? "Дневные самоотметки. Партнёр может отдельно подтвердить их. Отклонённые и спорные суммы не засчитываются."
            : room.metric === "focus"
              ? "Подтверждённые рабочие минуты сессий после вступления. Паузы и перерывы исключены."
              : room.metric === "tasks"
                ? "Первые завершения самостоятельных задач после вступления. Повторное завершение не создаёт новый зачёт."
                : "Первые сохранения вечернего разбора. Не больше одного в день."}
        </p>
        <p>
          До 20 участников. Подсчёт по дням UTC. Выход или отзыв доступа убирает вклад.{" "}
          {room.mode === "pvp"
            ? "Вступление до старта. Одинаковые результаты делят место. Победителям — знак испытания."
            : "Вклад складывается. При общей победе участники с вкладом получают знак испытания."}{" "}
          Знак не добавляет личный XP.
        </p>
        <p className="ui-hint">
          После срока можно подвести итоги. Исправления и споры пересчитывают результат.
        </p>
      </details>
      {room.state === "invited" ? (
        <div className="social-join">
          <label className="social-toggle">
            <input
              type="checkbox"
              checked={consent}
              onChange={(e) => setConsent(e.target.checked)}
            />
            <span>
              Принимаю правила и разрешаю показывать мой вклад всем участникам этого испытания,
              включая новых
            </span>
          </label>
          {room.mode === "pvp" && (
            <label className="social-toggle">
              <input type="checkbox" checked={board} onChange={(e) => setBoard(e.target.checked)} />
              <span>Отдельно согласен участвовать в рейтинге по псевдониму</span>
            </label>
          )}
          <div className="social-actions">
            <Button
              disabled={!consent || ended || (room.mode === "pvp" && future === false)}
              onClick={() =>
                void act(
                  {
                    section: "rooms",
                    input: {
                      action: "join",
                      id: room.id,
                      version: room.version,
                      rules_hash: room.rules_hash,
                      consent,
                      leaderboard: board,
                    },
                  },
                  "Вы в команде. Начинаем с маленького шага.",
                )
              }
            >
              Присоединиться
            </Button>
            <Button
              variant="ghost"
              onClick={() =>
                void act({
                  section: "rooms",
                  input: { action: "reject", id: room.id, version: room.version },
                })
              }
            >
              Отклонить испытание
            </Button>
          </div>
        </div>
      ) : (
        <>
          <div className="social-score">
            <strong>
              {room.mode === "coop"
                ? room.total
                : room.leaderboard && room.share
                  ? (room.members.find((m) => m.user_id === owner)?.amount ?? 0)
                  : "—"}
            </strong>
            <span>
              {room.mode === "pvp" && (!room.leaderboard || !room.share)
                ? "Участие в рейтинге выключено"
                : `${units[room.unit]} ${room.mode === "coop" ? "вместе" : "ваш результат в рейтинге"}`}
            </span>
          </div>
          <ol className="social-standings" aria-label={`Участники: ${room.name}`}>
            {room.members.map((m) => (
              <li key={m.user_id}>
                <span className="social-rank">
                  {room.mode === "pvp" ? m.rank : <Check size={15} />}
                </span>
                <div>
                  <strong>{m.user_id === owner ? "Вы" : m.nickname}</strong>
                  <small>
                    {m.confirmation === "partner_confirmed"
                      ? "Подтверждено партнёром"
                      : m.confirmation === "self_report"
                        ? "Самоотчёт"
                        : "По записям приложения"}
                  </small>
                </div>
                <b>
                  {m.amount} {units[room.unit]}
                </b>
                {m.earned && (
                  <Medal className="social-gold" aria-label="Знак испытания" size={22} />
                )}
              </li>
            ))}
          </ol>
          {!room.share && (
            <p className="ui-hint">
              <LockKeyhole size={14} /> Ваш вклад скрыт и не участвует в подсчёте.
            </p>
          )}
          {room.metric === "manual" && !ended && !future && (
            <form
              className="social-checkin"
              onSubmit={async (e) => {
                e.preventDefault();
                if (
                  await act(
                    {
                      section: "rooms",
                      input: {
                        action: "checkin",
                        id: room.id,
                        amount: value,
                        version: room.checkin?.version ?? 0,
                      },
                    },
                    "Дневная отметка сохранена.",
                  )
                )
                  setAmount(undefined);
              }}
            >
              <label htmlFor={`amount-${room.id}`}>Сегодня, {units[room.unit]}</label>
              <div className="social-stepper">
                <Button
                  variant="secondary"
                  size="icon"
                  aria-label={`Уменьшить: ${room.name}`}
                  disabled={value <= 0}
                  onClick={() => setAmount(Math.max(0, value - 1))}
                >
                  −
                </Button>
                <input
                  className="ui-input"
                  id={`amount-${room.id}`}
                  type="number"
                  min={0}
                  max={room.daily_cap}
                  required
                  value={value}
                  onChange={(e) => setAmount(Number(e.target.value))}
                />
                <Button
                  variant="secondary"
                  size="icon"
                  aria-label={`Увеличить: ${room.name}`}
                  disabled={value >= room.daily_cap}
                  onClick={() => setAmount(Math.min(room.daily_cap, value + 1))}
                >
                  +
                </Button>
                <Button type="submit">Сохранить отметку</Button>
              </div>
            </form>
          )}
          {room.checkin && room.checkin.amount > 0 && room.share && (
            <Button variant="secondary" size="small" onClick={requestReview}>
              <ShieldCheck size={16} /> Попросить подтверждение
            </Button>
          )}
          {ended && (
            <Button
              variant="primary"
              onClick={() =>
                void act(
                  { section: "rooms", input: { action: "finalize", id: room.id } },
                  "Итоги проверены. Повторное действие не дублирует награды.",
                )
              }
            >
              <Trophy size={17} />
              {room.finalized_at ? "Проверить итоги ещё раз" : "Подвести итоги"}
            </Button>
          )}
          {room.owner === owner &&
            !ended &&
            availableFriends.length > 0 &&
            (room.mode !== "pvp" || future) && (
              <form
                className="social-room-invite"
                onSubmit={async (e) => {
                  e.preventDefault();
                  if (
                    await act({
                      section: "rooms",
                      input: { action: "invite", id: room.id, recipient: invite },
                    })
                  )
                    setInvite("");
                }}
              >
                <label htmlFor={`invite-${room.id}`}>Пригласить друга в испытание</label>
                <select
                  className="ui-input"
                  id={`invite-${room.id}`}
                  value={invite}
                  onChange={(e) => setInvite(e.target.value)}
                  required
                >
                  <option value="">Выберите друга</option>
                  {friends
                    .filter((f) => !room.members.some((m) => m.user_id === f.user_id))
                    .map((f) => (
                      <option key={f.id} value={f.user_id}>
                        {f.nickname}
                      </option>
                    ))}
                </select>
                <Button variant="secondary" type="submit" disabled={!invite}>
                  Пригласить в испытание
                </Button>
              </form>
            )}
          <details className="social-details">
            <summary>Мой доступ и участие</summary>
            <label className="social-toggle">
              <input
                type="checkbox"
                checked={room.share}
                onChange={(e) =>
                  void act({
                    section: "rooms",
                    input: {
                      action: "sharing",
                      id: room.id,
                      version: room.version,
                      consent: e.target.checked,
                      leaderboard: room.leaderboard,
                    },
                  })
                }
              />
              <span>Показывать мой вклад участникам</span>
            </label>
            {room.mode === "pvp" && (
              <label className="social-toggle">
                <input
                  type="checkbox"
                  checked={room.leaderboard}
                  disabled={!room.share}
                  onChange={(e) =>
                    void act({
                      section: "rooms",
                      input: {
                        action: "sharing",
                        id: room.id,
                        version: room.version,
                        consent: room.share,
                        leaderboard: e.target.checked,
                      },
                    })
                  }
                />
                <span>Участвовать в рейтинге</span>
              </label>
            )}
            <Button
              variant="danger"
              size="small"
              onClick={() =>
                confirm(
                  "Выйти из испытания?",
                  "Ваш вклад будет исключён. Повторное вступление в это испытание не предусмотрено.",
                  {
                    section: "rooms",
                    input: { action: "leave", id: room.id, version: room.version },
                  },
                )
              }
            >
              Выйти из испытания
            </Button>
          </details>
        </>
      )}
    </Card>
  );
}

function RoomCreator({
  open,
  onOpenChange,
  act,
  busy,
  error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  act: Act;
  busy: boolean;
  error: string;
}) {
  const [mode, setMode] = useState<"coop" | "pvp">("coop"),
    [metric, setMetric] = useState<"tasks" | "focus" | "reviews" | "manual">("manual"),
    [name, setName] = useState(""),
    [days, setDays] = useState(7),
    [target, setTarget] = useState(50),
    [cap, setCap] = useState(10),
    [consent, setConsent] = useState(false),
    [board, setBoard] = useState(false);
  useUnsavedChanges(open && !!name.trim());
  const unit = metric === "manual" ? "pages" : metric === "focus" ? "minutes" : metric;
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Создадим общий вызов"
      description="Выберите темп, который хочется поддерживать. После создания условия фиксируются."
    >
      <SocialError message={error} />
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (
            await act(
              {
                section: "rooms",
                input: {
                  action: "create",
                  name: name.trim(),
                  mode,
                  metric,
                  unit,
                  days,
                  target,
                  daily_cap: cap,
                  consent,
                  leaderboard: board,
                },
              },
              "Испытание создано. Пригласите друга в его карточке.",
            )
          ) {
            onOpenChange(false);
            setName("");
            setConsent(false);
            setBoard(false);
          }
        }}
      >
        <fieldset className="social-controls social-form" disabled={busy}>
          <ChoiceCards
            label="Как идём к цели"
            value={mode}
            onChange={setMode}
            options={[
              {
                value: "coop",
                label: "Вместе к цели",
                description: "Складываем вклад команды",
                icon: HeartHandshake,
                tone: "mint",
              },
              {
                value: "pvp",
                label: "Дружеский PvP",
                description: "Одинаковые правила, общий старт",
                icon: Trophy,
                tone: "gold",
              },
            ]}
          />
          <ChoiceCards
            label="Что считаем"
            value={metric}
            onChange={(v) => {
              setMetric(v);
              setCap(v === "reviews" ? 1 : v === "focus" ? 60 : 10);
              setTarget(v === "reviews" ? 5 : v === "focus" ? 180 : 50);
            }}
            options={[
              {
                value: "manual",
                label: "Чтение",
                description: "Страницы · самоотчёт",
                icon: BookOpen,
                tone: "blue",
              },
              {
                value: "tasks",
                label: "Задачи",
                description: "По завершениям в приложении",
                icon: ListTodo,
                tone: "violet",
              },
              {
                value: "focus",
                label: "Фокус",
                description: "Подтверждённые минуты",
                icon: Timer,
                tone: "mint",
              },
              {
                value: "reviews",
                label: "Вечерний ритм",
                description: "Только факт разбора",
                icon: Moon,
                tone: "gold",
              },
            ]}
          />
          <label htmlFor="social-room-name">Название общего испытания</label>
          <input
            className="ui-input"
            id="social-room-name"
            maxLength={80}
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Например, семь дней хороших книг"
          />
          <ChoiceChips
            label="Продолжительность"
            value={days}
            onChange={setDays}
            options={[
              { value: 3, label: "3 дня" },
              { value: 7, label: "Неделя" },
              { value: 14, label: "2 недели" },
              { value: 30, label: "30 дней" },
            ]}
          />
          <div className="social-form-grid">
            <label>
              Цель, {units[unit]}
              <input
                className="ui-input"
                aria-label="Общая цель"
                type="number"
                required
                min={1}
                max={cap * days * (mode === "coop" ? 20 : 1)}
                value={target}
                onChange={(e) => setTarget(Number(e.target.value))}
              />
            </label>
            <label>
              Предел на человека в день
              <input
                className="ui-input"
                aria-label="Дневной предел"
                type="number"
                required
                min={1}
                max={
                  metric === "reviews"
                    ? 1
                    : metric === "focus"
                      ? 240
                      : metric === "tasks"
                        ? 100
                        : 1000
                }
                value={cap}
                onChange={(e) => setCap(Number(e.target.value))}
              />
            </label>
          </div>
          <div className="social-rule-preview">
            <Flag size={22} />
            <p>
              {mode === "coop"
                ? "Начинаем сегодня. Вклад команды суммируется."
                : "Старт завтра в 00:00 UTC. Присоединиться можно до старта."}{" "}
              Цель:{" "}
              <strong>
                {target} {units[unit]}
              </strong>
              , до {cap} в день. Срок и дневной подсчёт — UTC. Самоотчёт не выдаётся за
              подтверждённое действие.
            </p>
          </div>
          <label className="social-toggle">
            <input
              type="checkbox"
              required
              checked={consent}
              onChange={(e) => setConsent(e.target.checked)}
            />
            <span>
              Принимаю правила и разрешаю показывать мой вклад всем участникам этого испытания,
              включая новых
            </span>
          </label>
          {mode === "pvp" && (
            <label className="social-toggle">
              <input type="checkbox" checked={board} onChange={(e) => setBoard(e.target.checked)} />
              <span>Отдельно согласен участвовать в рейтинге по псевдониму</span>
            </label>
          )}
          <Button type="submit" disabled={!consent || !name.trim()}>
            <Sparkles size={17} /> Создать с общими правилами
          </Button>
        </fieldset>
      </form>
    </Dialog>
  );
}
function NudgeComposer({
  friends,
  act,
  recipient: draftRecipient,
  template,
  setRecipient,
  setTemplate,
}: {
  friends: SocialDashboard["friends"]["items"];
  act: Act;
  recipient: string;
  template: keyof typeof templates;
  setRecipient: (value: string) => void;
  setTemplate: (value: keyof typeof templates) => void;
}) {
  const recipient = friends.some((f) => f.user_id === draftRecipient) ? draftRecipient : "";
  return (
    <form
      className="social-form"
      onSubmit={async (e) => {
        e.preventDefault();
        await act(
          { section: "community", input: { action: "nudge", recipient, template } },
          "Поддержка отправлена.",
        );
      }}
    >
      <label htmlFor="nudge-recipient">Кого поддержать</label>
      <select
        className="ui-input"
        id="nudge-recipient"
        value={recipient}
        required
        onChange={(e) => setRecipient(e.target.value)}
      >
        <option value="">Выберите друга</option>
        {friends.map((f) => (
          <option key={f.id} value={f.user_id}>
            {f.nickname}
          </option>
        ))}
      </select>
      <div className="social-template-options" role="group" aria-label="Текст поддержки">
        {(Object.entries(templates) as [keyof typeof templates, string][]).map(([key, label]) => (
          <button
            key={key}
            type="button"
            aria-pressed={key === template}
            onClick={() => setTemplate(key)}
          >
            {key === template && <Check size={15} />}
            {label}
          </button>
        ))}
      </div>
      <Button type="submit" disabled={!recipient}>
        <HeartHandshake size={17} /> Поддержать
      </Button>
    </form>
  );
}
function PublishComposer({
  data,
  act,
  selected,
  setSelected,
}: {
  data: SocialDashboard;
  act: Act;
  selected: string;
  setSelected: (value: string) => void;
}) {
  if (!data.community.publishable.length)
    return (
      <p className="ui-hint">
        Первые доступные достижения появятся после ваших действий в приложении.
      </p>
    );
  const item = data.community.publishable.find((p) => p.key === selected);
  return (
    <form
      className="social-publish"
      onSubmit={async (e) => {
        e.preventDefault();
        if (
          item &&
          (await act(
            {
              section: "community",
              input: {
                action: "publish",
                source_key: item.key,
                source_version: item.version,
                consent: true,
              },
            },
            "Достижение опубликовано для друзей.",
          ))
        )
          setSelected("");
      }}
    >
      <label htmlFor="social-publish">Поделиться заработанным достижением</label>
      <select
        className="ui-input"
        id="social-publish"
        value={selected}
        onChange={(e) => setSelected(e.target.value)}
        required
      >
        <option value="">Выберите победу</option>
        {data.community.publishable.map((p) => (
          <option key={p.key} value={p.key}>
            {p.title}
          </option>
        ))}
      </select>
      <Button type="submit" disabled={!item}>
        Опубликовать для друзей
      </Button>
    </form>
  );
}

function ReviewCard({
  review,
  owner,
  act,
}: {
  review: SocialDashboard["reviews"][number];
  owner: string;
  act: Act;
}) {
  return (
    <Card key={review.id} className="social-review">
      <span className="social-badge" data-tone={review.state === "accepted" ? "mint" : "gold"}>
        <ShieldCheck size={15} />
        {review.matches_version
          ? reviewStates[review.state]
          : "Отметка изменена — нужна новая проверка"}
      </span>
      <h3>{review.room_name}</h3>
      <strong>
        {review.amount} {units[review.unit]}
      </strong>
      <p className="ui-hint">
        {review.owner === owner ? "Вы запросили подтверждение" : "Партнёр просит проверить отметку"}
      </p>
      {review.file && review.matches_version && review.state !== "withdrawn" && (
        <a className="social-file" href={`/api/v1/social/reviews/${review.id}/file`}>
          Скачать приватное доказательство
        </a>
      )}
      <div className="social-actions">
        {review.reviewer === owner &&
          review.matches_version &&
          review.state !== "withdrawn" &&
          (["accepted", "rejected", "disputed"] as const).map((state) => (
            <Button
              key={state}
              variant={state === "accepted" ? "primary" : "secondary"}
              size="small"
              disabled={review.state === state}
              onClick={() =>
                void act({
                  section: "reviews",
                  input: {
                    action: state,
                    id: review.id,
                    version: review.version,
                  },
                })
              }
            >
              {state === "accepted"
                ? "Подтвердить"
                : state === "rejected"
                  ? "Отклонить"
                  : "Отметить спор"}
            </Button>
          ))}
        {review.owner === owner && review.state !== "withdrawn" && (
          <Button
            variant="ghost"
            size="small"
            onClick={() =>
              void act({
                section: "reviews",
                input: {
                  action: "withdrawn",
                  id: review.id,
                  version: review.version,
                },
              })
            }
          >
            Отозвать доказательство
          </Button>
        )}
      </div>
    </Card>
  );
}

function SocialError({ message }: { message: string }) {
  return message ? (
    <p className="ui-alert" role="alert">
      {message}
    </p>
  ) : null;
}
