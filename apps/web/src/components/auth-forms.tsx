"use client";
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { ArrowUpRight, LockKeyhole } from "lucide-react";
import { z } from "zod";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import {
  authClient,
  authParams,
  authError,
  parseSession,
  safeRedirect,
  withAuthLock,
} from "@/lib/auth/session";
import { ThemeSelect } from "./theme-select";
import { Button } from "./ui/button";
import { Card } from "./ui/card";
import { Form } from "./ui/form";
import { Input, PasswordInput } from "./ui/field";

const email = z.string().trim().email("Введите корректный email.").max(254);
export const passwordSchema = z.string().refine((value) => {
  const length = [...value].length;
  return length >= 12 && length <= 128 && new TextEncoder().encode(value).length <= 512;
}, "Пароль должен содержать 12–128 символов.");
const loginSchema = z.object({
  email,
  password: z.string().min(1, "Введите пароль.").max(512),
  remember_me: z.boolean(),
});
const registerSchema = z
  .object({ email, password: passwordSchema, confirmation: z.string() })
  .refine((value) => value.password === value.confirmation, {
    path: ["confirmation"],
    message: "Пароли не совпадают.",
  });
const forgotSchema = z.object({ email });
const resetSchema = z
  .object({ password: passwordSchema, confirmation: z.string() })
  .refine((value) => value.password === value.confirmation, {
    path: ["confirmation"],
    message: "Пароли не совпадают.",
  });

function subscribeURL(callback: () => void) {
  window.addEventListener("popstate", callback);
  return () => window.removeEventListener("popstate", callback);
}

export function AuthFrame({ title, children }: { title: string; children: ReactNode }) {
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
        <LockKeyhole size={28} aria-hidden="true" />
        <p className="eyebrow">ВАШЕ ЛИЧНОЕ ПРОСТРАНСТВО</p>
        <h1>{title}</h1>
        {children}
      </Card>
    </main>
  );
}

export function LoginForm() {
  const auth = useAuth();
  const router = useRouter();
  const search = useSyncExternalStore(
    subscribeURL,
    () => window.location.search,
    () => "",
  );
  const notice =
    new URLSearchParams(search).get("reset") === "1"
      ? "Пароль обновлён. Войдите с новым паролем."
      : "";
  const flow = useValidatedForm({
    schema: loginSchema,
    defaultValues: { email: "", password: "", remember_me: false },
    fields: ["email", "password", "remember_me"],
    save: async (values) => {
      try {
        await withAuthLock(async () => {
          const api = await authClient();
          const response = await api.POST("/api/v1/auth/login", {
            params: authParams(),
            body: values,
          });
          const session = parseSession(response.data?.data);
          flow.form.reset();
          auth.adopt(session);
          router.replace(safeRedirect(new URLSearchParams(window.location.search).get("next")));
        });
      } catch (error) {
        authError(error, true);
      }
    },
  });
  return (
    <AuthFrame title="С возвращением">
      <p className="ui-hint">Войдите, чтобы продолжить свой путь.</p>
      {notice && <p role="status">{notice}</p>}
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Email"
          type="email"
          autoComplete="email"
          inputMode="email"
          error={flow.form.formState.errors.email?.message}
          {...flow.form.register("email")}
        />
        <PasswordInput
          label="Пароль"
          autoComplete="current-password"
          error={flow.form.formState.errors.password?.message}
          {...flow.form.register("password")}
        />
        <label className="auth-checkbox">
          <input type="checkbox" {...flow.form.register("remember_me")} />
          Запомнить меня на 30 дней
        </label>
        <Button type="submit">Войти</Button>
      </Form>
      <Link href="/auth/forgot-password" className="text-link">
        Забыли пароль?
      </Link>
      <p className="ui-hint">
        Первый шаг здесь?{" "}
        <Link href="/auth/register" className="text-link">
          Создать аккаунт
        </Link>
      </p>
    </AuthFrame>
  );
}

export function RegisterForm() {
  const [created, setCreated] = useState(false);
  const flow = useValidatedForm({
    schema: registerSchema,
    defaultValues: { email: "", password: "", confirmation: "" },
    fields: ["email", "password", "confirmation"],
    save: async (values) => {
      try {
        await withAuthLock(async () => {
          const api = await authClient();
          await api.POST("/api/v1/auth/register", {
            params: authParams(),
            body: { email: values.email, password: values.password },
          });
        });
        setCreated(true);
      } catch (error) {
        authError(error);
      }
    },
  });
  return (
    <AuthFrame title={created ? "Аккаунт создан" : "Начните свой путь"}>
      {created ? (
        <>
          <p role="status">
            Проверьте почту: мы отправили ссылку подтверждения. Она действует 24 часа.
          </p>
          <Button asChild>
            <Link href="/auth/login?next=/app/onboarding">Добавить первую задачу</Link>
          </Button>
          <p className="ui-hint">Если письмо не пришло, после входа можно отправить его ещё раз.</p>
        </>
      ) : (
        <>
          <p className="ui-hint">Личные данные закрыты по умолчанию.</p>
          <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
            <Input
              label="Email"
              type="email"
              autoComplete="email"
              error={flow.form.formState.errors.email?.message}
              {...flow.form.register("email")}
            />
            <PasswordInput
              label="Пароль"
              autoComplete="new-password"
              hint="12–128 символов. Пробелы и менеджеры паролей поддерживаются."
              error={flow.form.formState.errors.password?.message}
              {...flow.form.register("password")}
            />
            <PasswordInput
              label="Повторите пароль"
              autoComplete="new-password"
              error={flow.form.formState.errors.confirmation?.message}
              {...flow.form.register("confirmation")}
            />
            <Button type="submit">Создать аккаунт</Button>
          </Form>
          <p className="ui-hint">
            Уже с нами?{" "}
            <Link href="/auth/login" className="text-link">
              Войти
            </Link>
          </p>
        </>
      )}
    </AuthFrame>
  );
}

export function ForgotForm() {
  const [sent, setSent] = useState(false);
  const flow = useValidatedForm({
    schema: forgotSchema,
    defaultValues: { email: "" },
    fields: ["email"],
    save: async (values) => {
      try {
        await withAuthLock(async () => {
          const api = await authClient();
          await api.POST("/api/v1/auth/forgot-password", { params: authParams(), body: values });
        });
        setSent(true);
      } catch (error) {
        authError(error);
      }
    },
  });
  return (
    <AuthFrame title="Восстановление доступа">
      {sent ? (
        <p role="status">
          Если аккаунт с таким адресом существует, мы отправим ссылку для сброса пароля. Проверьте
          почту и папку «Спам».
        </p>
      ) : (
        <>
          <p className="ui-hint">Отправим одноразовую ссылку. Она действует 30 минут.</p>
          <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
            <Input
              label="Email"
              type="email"
              autoComplete="email"
              error={flow.form.formState.errors.email?.message}
              {...flow.form.register("email")}
            />
            <Button type="submit">Отправить ссылку</Button>
          </Form>
        </>
      )}
      <Link href="/auth/login" className="text-link">
        Вернуться ко входу
      </Link>
    </AuthFrame>
  );
}

function useLinkToken() {
  const captured = useRef(false);
  const [value, setValue] = useState<{ ready: boolean; token: string }>({
    ready: false,
    token: "",
  });
  useEffect(() => {
    if (captured.current) return;
    captured.current = true;
    const token = new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "";
    history.replaceState(history.state, "", window.location.pathname + window.location.search);
    setValue({ ready: true, token: /^[A-Za-z0-9_-]{43}$/.test(token) ? token : "" });
  }, []);
  return value;
}

export function VerifyForm() {
  const { ready, token } = useLinkToken();
  const auth = useAuth();
  const locked = useRef(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");
  async function verify() {
    if (locked.current) return;
    locked.current = true;
    setBusy(true);
    setError("");
    try {
      await withAuthLock(async () => {
        const api = await authClient();
        await api.POST("/api/v1/auth/verify-email", { params: authParams(), body: { token } });
      });
      setDone(true);
      void auth.reload();
    } catch (error) {
      try {
        authError(error);
      } catch (safe) {
        setError(safe instanceof ApiError ? safe.message : "Не удалось подтвердить почту.");
      }
    } finally {
      locked.current = false;
      setBusy(false);
    }
  }
  return (
    <AuthFrame title="Подтверждение почты">
      {done ? (
        <p role="status">Почта подтверждена. Можно продолжать!</p>
      ) : !ready ? (
        <p role="status">Проверяем ссылку…</p>
      ) : !token ? (
        <p role="alert">
          В ссылке нет кода подтверждения. Откройте ссылку из письма или запросите новую после
          входа.
        </p>
      ) : (
        <>
          <p className="ui-hint">Подтвердите, что этот адрес принадлежит вам.</p>
          <Button disabled={busy} onClick={() => void verify()}>
            {busy ? "Подтверждаем…" : "Подтвердить почту"}
          </Button>
          {error && (
            <p role="alert" className="ui-error">
              {error}
            </p>
          )}
        </>
      )}
      <Link href="/app" className="text-link">
        В пространство
      </Link>
      <Link href="/auth/login" className="text-link">
        Войти
      </Link>
    </AuthFrame>
  );
}

export function ResetForm() {
  const { ready, token } = useLinkToken();
  const auth = useAuth();
  const router = useRouter();
  const flow = useValidatedForm({
    schema: resetSchema,
    defaultValues: { password: "", confirmation: "" },
    fields: ["password", "confirmation"],
    save: async (values) => {
      try {
        await withAuthLock(async () => {
          const api = await authClient();
          await api.POST("/api/v1/auth/reset-password", {
            params: authParams(),
            body: { token, password: values.password },
          });
          flow.form.reset();
          auth.adopt(null);
          router.replace("/auth/login?reset=1");
        });
      } catch (error) {
        authError(error);
      }
    },
  });
  return (
    <AuthFrame title="Новый пароль">
      {!ready ? (
        <p role="status">Проверяем ссылку…</p>
      ) : !token ? (
        <p role="alert">В ссылке нет кода сброса. Запросите новое письмо.</p>
      ) : (
        <>
          <p className="ui-hint">После смены пароля все предыдущие входы будут завершены.</p>
          <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
            <PasswordInput
              label="Новый пароль"
              autoComplete="new-password"
              hint="12–128 символов, без обрезания пробелов."
              error={flow.form.formState.errors.password?.message}
              {...flow.form.register("password")}
            />
            <PasswordInput
              label="Повторите пароль"
              autoComplete="new-password"
              error={flow.form.formState.errors.confirmation?.message}
              {...flow.form.register("confirmation")}
            />
            <Button type="submit">Сохранить пароль</Button>
          </Form>
        </>
      )}
      <Link href="/auth/forgot-password" className="text-link">
        Запросить новую ссылку
      </Link>
    </AuthFrame>
  );
}
