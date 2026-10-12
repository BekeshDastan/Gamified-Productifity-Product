"use client";
import { useRef, useState } from "react";
import { z } from "zod";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { ApiError, createApiClient } from "@/lib/api/client";
import { useAuth } from "@/lib/auth/provider";
import { authClient, authParams, authError, loadSession, withAuthLock } from "@/lib/auth/session";
import { Button } from "./ui/button";
import { Card } from "./ui/card";

const sessionsSchema = z.array(
  z.object({
    id: z.uuid(),
    created_at: z.iso.datetime({ offset: true }),
    expires_at: z.iso.datetime({ offset: true }),
    remember_me: z.boolean(),
    current: z.boolean(),
  }),
);
type SessionInfo = z.infer<typeof sessionsSchema>[number];
export function AccountPanel() {
  const auth = useAuth();
  const [busy, setBusy] = useState(false);
  const locked = useRef(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [sessions, setSessions] = useState<SessionInfo[] | null>(null);
  async function action(work: () => Promise<void>) {
    if (locked.current) return;
    locked.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await work();
    } catch (error) {
      try {
        authError(error);
      } catch (safe) {
        setError(
          safe instanceof ApiError
            ? safe.message
            : "Не удалось выполнить действие. Попробуйте снова.",
        );
      }
    } finally {
      locked.current = false;
      setBusy(false);
    }
  }
  async function list() {
    await loadSession();
    const response = await createApiClient().GET("/api/v1/auth/sessions");
    const parsed = sessionsSchema.safeParse(response.data?.data);
    if (!parsed.success)
      throw new ApiError(502, "INVALID_RESPONSE", "Не удалось загрузить устройства.");
    setSessions(parsed.data);
  }
  if (auth.session === undefined)
    return (
      <p role="status" className="ui-hint">
        Проверяем вход…
      </p>
    );
  if (!auth.session)
    return (
      <Card className="account-panel">
        <h2>Ваш личный аккаунт</h2>
        <p className="ui-hint">Создайте аккаунт или войдите, чтобы сохранить своё пространство.</p>
        <div className="auth-actions">
          <Button asChild>
            <Link href="/auth/register">Создать аккаунт</Link>
          </Button>
          <Button asChild variant="secondary">
            <Link href="/auth/login">Войти</Link>
          </Button>
        </div>
        {auth.error && (
          <p className="ui-hint">
            {auth.error}{" "}
            <Button variant="ghost" onClick={() => void auth.reload()}>
              Повторить
            </Button>
          </p>
        )}
      </Card>
    );
  return (
    <Card className="account-panel">
      <p className="eyebrow">ЛИЧНЫЙ АККАУНТ</p>
      <h2 className="account-email">{auth.session.account.email}</h2>
      <p className="ui-hint">
        {auth.session.account.email_verified
          ? "Почта подтверждена"
          : "Почта ещё не подтверждена. Проверьте письмо: ссылка действует 24 часа."}
      </p>
      <div className="auth-actions">
        {!auth.session.account.email_verified && (
          <Button
            disabled={busy}
            variant="secondary"
            onClick={() =>
              void action(async () => {
                await loadSession();
                await withAuthLock(async () => {
                  const api = await authClient();
                  await api.POST("/api/v1/auth/resend-verification", {
                    params: authParams(),
                    body: {},
                  });
                });
                setMessage("Письмо отправлено. Проверьте почту и папку «Спам».");
              })
            }
          >
            Отправить письмо ещё раз
          </Button>
        )}
        <Button disabled={busy} variant="secondary" onClick={() => void action(list)}>
          Устройства и входы
        </Button>
        <Button disabled={busy} variant="ghost" onClick={() => void action(() => auth.logout())}>
          Выйти
        </Button>
      </div>
      {sessions && (
        <div className="session-list">
          <h3>Активные входы</h3>
          <ul>
            {sessions.map((item) => (
              <li key={item.id}>
                <div>
                  <strong>{item.current ? "Это устройство" : "Другое устройство"}</strong>
                  <p className="ui-hint">
                    Вход: {new Date(item.created_at).toLocaleString("ru-RU")}
                    {item.remember_me ? " · Запомнено на 30 дней" : " · До закрытия браузера"}
                  </p>
                </div>
                <Button
                  disabled={busy}
                  variant="ghost"
                  onClick={() =>
                    void action(async () => {
                      await loadSession();
                      await withAuthLock(async () => {
                        const api = await authClient();
                        await api.DELETE("/api/v1/auth/sessions/{id}", {
                          params: { ...authParams(), path: { id: item.id } },
                          body: {},
                        });
                      });
                      if (item.current) auth.adopt(null);
                      else await list();
                    })
                  }
                >
                  {item.current ? "Выйти" : "Завершить вход"}
                </Button>
              </li>
            ))}
          </ul>
          <Button
            disabled={busy}
            variant="danger"
            onClick={() =>
              void action(async () => {
                await loadSession();
                await auth.logout(true);
              })
            }
          >
            Выйти на всех устройствах
          </Button>
        </div>
      )}
      {busy && (
        <p role="status" className="ui-hint">
          Выполняем…
        </p>
      )}
      {message && <p role="status">{message}</p>}
      {error && (
        <p role="alert" className="ui-error">
          {error}
        </p>
      )}
    </Card>
  );
}
