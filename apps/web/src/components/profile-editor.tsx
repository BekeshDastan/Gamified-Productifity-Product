"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Mountain, Compass, Sprout, Sun, Moon, Star } from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { accountQueryKey } from "@/lib/api/query-provider";
import {
  getProfile,
  profileInput,
  profileSchema,
  requireAccount,
  type Profile,
} from "@/lib/profile";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { ErrorState, LoadingState } from "./ui/states";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { Form } from "./ui/form";

const catalog = [
  { id: "mountain", label: "Гора", Icon: Mountain },
  { id: "compass", label: "Компас", Icon: Compass },
  { id: "sprout", label: "Росток", Icon: Sprout },
  { id: "sun", label: "Солнце", Icon: Sun },
  { id: "moon", label: "Луна", Icon: Moon },
  { id: "star", label: "Звезда", Icon: Star },
] as const;

export function ProfileEditor() {
  const auth = useAuth();
  const id = auth.session?.account.id;
  const query = useQuery({
    queryKey: ["account", id, "profile"],
    enabled: !!id,
    queryFn: ({ signal }) => getProfile(signal),
  });
  if (auth.session === undefined || (!!id && query.isPending))
    return <LoadingState label="Загружаем профиль…" />;
  if (!id)
    return (
      <p>
        Для настройки профиля{" "}
        <SafeLink className="text-link" href="/auth/login?next=/app/settings/profile">
          войдите в аккаунт
        </SafeLink>
        .
      </p>
    );
  if (!query.data) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return <ProfileForm key={id} profile={query.data} />;
}
function ProfileForm({ profile }: { profile: Profile }) {
  const cache = useQueryClient();
  const [version, setVersion] = useState(profile.version);
  const [saved, setSaved] = useState(false);
  const flow = useValidatedForm({
    schema: profileInput,
    defaultValues: { name: profile.name, nickname: profile.nickname, avatar: profile.avatar },
    fields: ["name", "nickname", "avatar"],
    save: async (values) => {
      setSaved(false);
      await requireAccount();
      const updated = await withAuthLock(async () => {
        const api = await authClient();
        const response = await api.PATCH("/api/v1/me", {
          params: { header: { ...authParams().header, "If-Match": `"${version}"` } },
          body: values,
        });
        return profileSchema.parse(response.data?.data);
      });
      setVersion(updated.version);
      cache.setQueryData(accountQueryKey(profile.owner_id, "profile"), updated);
      setSaved(true);
      return { name: updated.name, nickname: updated.nickname, avatar: updated.avatar };
    },
  });
  const Avatar = catalog.find((item) => item.id === flow.form.watch("avatar"))?.Icon ?? Mountain;
  return (
    <Card className="settings-card profile-composer">
      <h2>Как к вам обращаться</h2>
      <div className="profile-preview" aria-label="Предпросмотр профиля">
        <span>
          <Avatar size={38} aria-hidden="true" />
        </span>
        <div>
          <strong>{flow.form.watch("name") || "Ваше имя"}</strong>
          <small>
            {flow.form.watch("nickname") ? `@${flow.form.watch("nickname")}` : "Ваш личный путь"}
          </small>
        </div>
      </div>
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label="Имя"
          autoComplete="name"
          maxLength={80}
          error={flow.form.formState.errors.name?.message}
          {...flow.form.register("name")}
        />
        <Input
          label="Ник"
          autoComplete="nickname"
          maxLength={24}
          hint="3–24 символа: латинские буквы в нижнем регистре, цифры и _. Можно оставить пустым."
          error={flow.form.formState.errors.nickname?.message}
          {...flow.form.register("nickname")}
        />
        <fieldset className="avatar-picker">
          <legend>Стартовый аватар</legend>
          {catalog.map(({ id, label, Icon }) => (
            <label key={id}>
              <input type="radio" value={id} {...flow.form.register("avatar")} />
              <Icon size={26} aria-hidden="true" />
              <span>{label}</span>
            </label>
          ))}
        </fieldset>
        <Button type="submit">Сохранить профиль</Button>
      </Form>
      {saved && !flow.form.formState.isDirty && <p role="status">Профиль сохранён.</p>}
      {flow.error && (
        <Button
          variant="ghost"
          onClick={() => {
            void getProfile()
              .then((fresh) => {
                flow.form.reset({
                  name: fresh.name,
                  nickname: fresh.nickname,
                  avatar: fresh.avatar,
                });
                setVersion(fresh.version);
                setSaved(false);
                cache.setQueryData(accountQueryKey(profile.owner_id, "profile"), fresh);
              })
              .catch(() =>
                flow.form.setError("root.server", {
                  message: "Не удалось обновить данные. Попробуйте снова.",
                }),
              );
          }}
        >
          Обновить данные
        </Button>
      )}
    </Card>
  );
}
