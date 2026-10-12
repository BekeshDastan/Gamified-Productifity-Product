"use client";
import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { questFiles, uploadQuestFile, changeQuestFile, type Attachment } from "@/lib/quest-files";
import type { Quest } from "@/lib/quests";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Input } from "./ui/field";
export function QuestFiles({ quest, onChanged }: { quest: Quest; onChanged: () => void }) {
  const auth = useAuth();
  const discard = useConfirmDiscard();
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const files = useQuery({
    queryKey: ["account", auth.session?.account.id, "quest-files", quest.id, quest.version],
    queryFn: ({ signal }) => questFiles(quest.id, signal),
  });
  async function change(a: Attachment, remove: boolean) {
    if (busy || !discard()) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const result = await changeQuestFile(a, remove);
      setMessage(
        remove
          ? "Файл удалён."
          : result?.verdict === "pending"
            ? "Проверка пока недоступна. Файл остаётся закрытым."
            : "Проверка завершена.",
      );
      await files.refetch();
      if (remove) onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Не удалось выполнить действие");
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>Вложения</h2>
      <p className="ui-hint">
        PNG, JPEG, WebP, PDF и TXT. До 10 МБ на файл, пяти файлов на задачу и 250 МБ на аккаунт.
        Скачивание доступно после проверки.
      </p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (busy || !file || !discard()) return;
          if (file.size > 10 * 1024 * 1024) {
            setError("Файл больше 10 МБ");
            return;
          }
          setBusy(true);
          setError("");
          setMessage("");
          try {
            const result = await uploadQuestFile(quest, file);
            setFile(undefined);
            if (input.current) input.current.value = "";
            setMessage(
              result.verdict === "clean"
                ? "Файл добавлен."
                : "Файл загружен и ожидает проверки. Пока скачать его нельзя.",
            );
            onChanged();
          } catch (err) {
            setError(
              err instanceof ApiError
                ? (err.fields.file ?? err.message)
                : "Не удалось загрузить файл",
            );
          } finally {
            setBusy(false);
          }
        }}
      >
        <Input
          ref={input}
          label="Файл для задачи"
          type="file"
          accept=".png,.jpg,.jpeg,.webp,.pdf,.txt"
          disabled={busy || !!quest.archived_at}
          onChange={(e) => setFile(e.target.files?.[0])}
        />
        <Button type="submit" disabled={busy || !file || !!quest.archived_at}>
          {busy ? "Подождите…" : "Загрузить файл"}
        </Button>
      </form>
      {error && (
        <p role="alert" className="ui-error">
          {error}
        </p>
      )}
      {message && <p role="status">{message}</p>}
      {files.isError && (
        <p role="alert">
          Файлы не загрузились. <Button onClick={() => void files.refetch()}>Повторить</Button>
        </p>
      )}
      <ul className="quest-files">
        {files.data?.map((a) => (
          <li key={a.id}>
            <strong>{a.name}</strong> · {Math.ceil(a.size / 1024)} КБ
            <p>
              {a.verdict === "clean"
                ? "Проверен"
                : a.verdict === "rejected"
                  ? "Отклонён проверкой"
                  : "Ожидает проверки"}
            </p>
            <div className="button-row">
              {a.verdict === "clean" && (
                <a
                  className="text-link"
                  href={"/api/v1/attachments/" + a.id + "/download"}
                  download
                >
                  Скачать {a.name}
                </a>
              )}
              {a.verdict === "pending" && (
                <Button variant="secondary" disabled={busy} onClick={() => void change(a, false)}>
                  Повторить проверку
                </Button>
              )}
              <Button variant="ghost" disabled={busy} onClick={() => void change(a, true)}>
                Удалить {a.name}
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
}
