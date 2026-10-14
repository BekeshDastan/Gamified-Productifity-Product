"use client";
import { useState, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import { AlertTriangle, FileQuestion, Inbox, LockKeyhole, RefreshCw } from "lucide-react";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { ApiError } from "@/lib/api/client";
import { Button } from "./button";
import { Dialog } from "./dialog";

export function LoadingState({ label = "Загружаем данные…" }: { label?: string }) {
  return (
    <div role="status" className="page-state">
      <p>{label}</p>
      <div aria-hidden="true" className="skeleton-stack">
        <span className="skeleton" />
        <span className="skeleton" />
        <span className="skeleton" />
      </div>
    </div>
  );
}
export function EmptyState({
  title = "Пока нет записей",
  description,
  action,
}: {
  title?: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-state">
      <Inbox size={32} aria-hidden="true" />
      <h2>{title}</h2>
      <p className="ui-hint">{description}</p>
      {action}
    </div>
  );
}
export function UnauthorizedState() {
  const pathname = usePathname() ?? "/app";
  return (
    <div className="page-state" role="alert">
      <LockKeyhole size={32} aria-hidden="true" />
      <h2>Нужен вход в аккаунт</h2>
      <p className="ui-hint">Войдите, чтобы открыть свои личные данные.</p>
      <Button asChild>
        <Link
          href={`/auth/login?next=${encodeURIComponent(pathname.startsWith("/app") ? pathname : "/app")}`}
        >
          Перейти ко входу
        </Link>
      </Button>
    </div>
  );
}
export function ErrorState({ error, retry }: { error?: unknown; retry: () => void }) {
  if (error instanceof ApiError && error.status === 401) return <UnauthorizedState />;
  return (
    <div className="page-state" role="alert">
      <AlertTriangle size={30} aria-hidden="true" />
      <h2>Не удалось загрузить данные</h2>
      <p className="ui-hint">
        {error instanceof ApiError
          ? error.message
          : "Попробуйте ещё раз. Если ошибка повторится, вернитесь позже."}
      </p>
      {error instanceof ApiError && error.requestId && (
        <p className="ui-hint request-id">Код обращения: {error.requestId}</p>
      )}
      <Button variant="secondary" onClick={retry}>
        <RefreshCw size={16} aria-hidden="true" />
        Повторить
      </Button>
    </div>
  );
}
export function NotFoundState() {
  return (
    <div className="page-state">
      <FileQuestion size={36} aria-hidden="true" />
      <p className="eyebrow">404</p>
      <h1 className="page-title">Страница не найдена</h1>
      <p className="ui-hint">Ссылка могла измениться или раздел ещё не доступен.</p>
      <Button asChild>
        <Link href="/">На главную</Link>
      </Button>
    </div>
  );
}
export function ConflictDialog({
  open,
  onOpenChange,
  currentVersion,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentVersion?: number;
  reload: () => Promise<void>;
}) {
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Запись уже изменилась"
      description="Ваши изменения не сохранены. Можно вернуться к редактированию или загрузить актуальную запись; при загрузке введённые изменения будут заменены."
    >
      <div className="conflict-actions">
        {currentVersion && <p className="ui-hint">Текущая версия: {currentVersion}</p>}
        {failed && (
          <p role="alert" className="ui-error">
            Не удалось обновить запись. Ввод сохранён.
          </p>
        )}
        <Button variant="secondary" disabled={loading} onClick={() => onOpenChange(false)}>
          Продолжить редактирование
        </Button>
        <Button
          disabled={loading}
          onClick={async () => {
            if (loading) return;
            setLoading(true);
            setFailed(false);
            try {
              await reload();
              onOpenChange(false);
            } catch {
              setFailed(true);
            } finally {
              setLoading(false);
            }
          }}
        >
          {loading ? "Загружаем…" : "Загрузить актуальную запись"}
        </Button>
      </div>
    </Dialog>
  );
}
