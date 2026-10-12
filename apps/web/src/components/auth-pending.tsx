import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { ArrowUpRight, LockKeyhole } from "lucide-react";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { ThemeSelect } from "./theme-select";
export function AuthPending({ kind }: { kind: "login" | "register" }) {
  return (
    <main className="auth-page">
      <header>
        <Link href="/" className="wordmark" aria-label="ASCEND, главная">
          <ArrowUpRight aria-hidden="true" />
          ASCEND
        </Link>
        <ThemeSelect />
      </header>
      <Card>
        <LockKeyhole size={32} aria-hidden="true" />
        <p className="eyebrow">СКОРО</p>
        <h1>{kind === "login" ? "Вход пока недоступен" : "Регистрация пока недоступна"}</h1>
        <p className="ui-hint">
          Личные аккаунты ещё в разработке. Сейчас можно посмотреть описание приложения и выбрать
          оформление.
        </p>
        <Button asChild>
          <Link href="/app">Открыть пространство</Link>
        </Button>
        <Link href="/" className="text-link">
          Вернуться на главную
        </Link>
      </Card>
    </main>
  );
}
