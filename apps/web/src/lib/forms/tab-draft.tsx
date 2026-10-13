"use client";
import { useEffect, useState } from "react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
export function useTabDraft<T>({
  storageKey,
  schema,
  value,
  dirty,
  restore,
}: {
  storageKey?: string;
  schema: z.ZodType<T>;
  value: T;
  dirty: boolean;
  restore: (value: T) => void;
}) {
  const [draft, setDraft] = useState<T | null>(null),
    [enabled, setEnabled] = useState(true);
  const encoded = JSON.stringify(value);
  useEffect(() => {
    if (!storageKey) return;
    let active = true;
    queueMicrotask(() => {
      try {
        const raw = sessionStorage.getItem(storageKey);
        if (raw && raw.length < 500000) {
          const parsed = schema.safeParse(JSON.parse(raw));
          if (parsed.success && active) setDraft(parsed.data);
        }
      } catch {
        /* Storage may be disabled. */
      }
    });
    return () => {
      active = false;
    };
  }, [storageKey, schema]);
  useEffect(() => {
    if (!storageKey) return;
    try {
      if (!enabled) sessionStorage.removeItem(storageKey);
      else if (dirty) sessionStorage.setItem(storageKey, encoded);
    } catch {
      /* Unsaved-change protection still applies. */
    }
  }, [storageKey, encoded, dirty, enabled]);
  function clear() {
    if (storageKey)
      try {
        sessionStorage.removeItem(storageKey);
      } catch {}
    setDraft(null);
  }
  return {
    available: draft !== null,
    enabled,
    setEnabled,
    discard: clear,
    clear,
    restore() {
      if (draft !== null) restore(draft);
      setDraft(null);
    },
  };
}
export function TabDraftRecovery({
  draft,
}: {
  draft: {
    available: boolean;
    enabled: boolean;
    setEnabled: (v: boolean) => void;
    restore: () => void;
    discard: () => void;
  };
}) {
  return (
    <div>
      <label>
        <input
          type="checkbox"
          checked={draft.enabled}
          onChange={(e) => draft.setEnabled(e.target.checked)}
        />{" "}
        Сохранять черновик ритуала в этой вкладке
      </label>
      {draft.available && (
        <div role="status">
          <p>
            Есть несохранённый черновик. Проверьте восстановленный текст и выбранные задачи перед
            подтверждением.
          </p>
          <Button type="button" variant="secondary" onClick={draft.restore}>
            Восстановить черновик ритуала
          </Button>
          <Button type="button" variant="secondary" onClick={draft.discard}>
            Удалить черновик ритуала
          </Button>
        </div>
      )}
    </div>
  );
}
