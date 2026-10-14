"use client";
import { useEffect, useRef, useState } from "react";
import { createApiClient } from "@/lib/api/client";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { AuthFrame } from "./auth-forms";
import { Button } from "./ui/button";
export function NotificationUnsubscribe() {
  const captured = useRef(false),
    [token, setToken] = useState(""),
    [title, setTitle] = useState(""),
    [error, setError] = useState(""),
    [ack, setAck] = useState(false),
    [busy, setBusy] = useState(false),
    [done, setDone] = useState(false);
  useEffect(() => {
    if (captured.current) return;
    captured.current = true;
    const value = new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "";
    history.replaceState(history.state, "", window.location.pathname + window.location.search);
    if (!value) {
      queueMicrotask(() => setError("Откройте ссылку из письма."));
      return;
    }
    void createApiClient()
      .POST("/api/v1/notifications/unsubscribe/preview", {
        params: { header: { Origin: window.location.origin } },
        body: { token: value },
      })
      .then((r) => {
        setToken(value);
        const data = r.data?.data;
        if (data) setTitle(data.title);
        else setError("Ссылка недействительна.");
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Не удалось проверить ссылку."));
  }, []);
  return (
    <AuthFrame title="Настройки писем">
      {!title && !error && <p role="status">Проверяем ссылку…</p>}
      {error && <p role="alert">{error}</p>}
      {title && !done && (
        <>
          <p>Отключить письма типа «{title}»?</p>
          <label>
            <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />{" "}
            Подтверждаю отключение этих писем
          </label>
          <Button
            disabled={!ack || busy}
            onClick={() => {
              setBusy(true);
              setError("");
              void createApiClient()
                .POST("/api/v1/notifications/unsubscribe", {
                  params: { header: { Origin: window.location.origin } },
                  body: { token, confirm: true },
                })
                .then(() => setDone(true))
                .catch((e) =>
                  setError(e instanceof Error ? e.message : "Не удалось сохранить настройку."),
                )
                .finally(() => setBusy(false));
            }}
          >
            Отключить письма этого типа
          </Button>
        </>
      )}
      {done && <p role="status">Письма этого типа отключены.</p>}
      <SafeLink href="/app/notifications">Открыть центр уведомлений</SafeLink>
    </AuthFrame>
  );
}
