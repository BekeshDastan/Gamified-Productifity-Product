"use client";
import { useRouter } from "next/navigation";
import { z } from "zod";
import { KeyRound } from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { requireAccount } from "@/lib/profile";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { passwordSchema } from "./auth-forms";
import { AccountPanel } from "./account-panel";
import { Card } from "./ui/card";
import { PasswordInput } from "./ui/field";
import { Form } from "./ui/form";
import { Button } from "./ui/button";
const schema = z
  .object({
    current_password: z.string().min(1, "Введите текущий пароль").max(512),
    password: passwordSchema,
    confirmation: z.string(),
  })
  .refine((v) => v.password === v.confirmation, {
    path: ["confirmation"],
    message: "Пароли не совпадают",
  });
export function SecuritySettings() {
  const auth = useAuth();
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!auth.session)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/settings/security">
        Войти для настройки безопасности
      </SafeLink>
    );
  return (
    <>
      <AccountPanel />
      <PasswordChange />
    </>
  );
}
function PasswordChange() {
  const auth = useAuth();
  const router = useRouter();
  const flow = useValidatedForm({
    schema,
    defaultValues: { current_password: "", password: "", confirmation: "" },
    fields: ["current_password", "password", "confirmation"],
    save: async (values) => {
      await requireAccount();
      await withAuthLock(async () => {
        const api = await authClient();
        await api.PATCH("/api/v1/me/password", {
          params: authParams(),
          body: { current_password: values.current_password, password: values.password },
        });
        flow.form.reset();
        auth.adopt(null);
        router.replace("/auth/login?reset=1");
      });
    },
  });
  return (
    <Card className="settings-card password-settings">
      <div className="support-card-heading">
        <span className="support-icon" data-tone="gold">
          <KeyRound aria-hidden="true" />
        </span>
        <div>
          <p className="eyebrow">ДОСТУП ПОД КОНТРОЛЕМ</p>
          <h2>Сменить пароль</h2>
        </div>
      </div>
      <p className="ui-hint">
        После смены пароля все устройства, включая это, выйдут из аккаунта. Войдите с новым паролем.
      </p>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <PasswordInput
          label="Текущий пароль"
          autoComplete="current-password"
          error={flow.form.formState.errors.current_password?.message}
          {...flow.form.register("current_password")}
        />
        <PasswordInput
          label="Новый пароль"
          autoComplete="new-password"
          hint="12–128 символов, включая пробелы."
          error={flow.form.formState.errors.password?.message}
          {...flow.form.register("password")}
        />
        <PasswordInput
          label="Повторите новый пароль"
          autoComplete="new-password"
          error={flow.form.formState.errors.confirmation?.message}
          {...flow.form.register("confirmation")}
        />
        <Button type="submit">Изменить пароль и выйти</Button>
      </Form>
    </Card>
  );
}
