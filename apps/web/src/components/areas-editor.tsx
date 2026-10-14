"use client";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import {
  BookOpen,
  Briefcase,
  Code,
  Languages,
  Activity,
  Rocket,
  Heart,
  Sprout,
  Tag,
} from "lucide-react";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { SafeLink, useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Input } from "./ui/field";
import { Form } from "./ui/form";
const icons = {
  book: BookOpen,
  briefcase: Briefcase,
  code: Code,
  languages: Languages,
  activity: Activity,
  rocket: Rocket,
  heart: Heart,
  sprout: Sprout,
  tag: Tag,
};
const iconLabels = {
  book: "Учёба",
  briefcase: "Работа",
  code: "Код",
  languages: "Языки",
  activity: "Активность",
  rocket: "Проект",
  heart: "Отношения",
  sprout: "Росток",
  tag: "Тег",
};
const inputSchema = z.object({
  name: z.string().min(1, "Введите название").max(60, "До 60 символов"),
  color: z.string().regex(/^#[0-9a-f]{6}$/),
  icon: z.enum([
    "book",
    "briefcase",
    "code",
    "languages",
    "activity",
    "rocket",
    "heart",
    "sprout",
    "tag",
  ]),
  archived: z.boolean(),
});
const labelSchema = inputSchema.extend({
  id: z.uuid(),
  version: z.number().int().positive(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
type Label = z.infer<typeof labelSchema>;
export function AreasEditor() {
  const auth = useAuth();
  if (auth.session === undefined) return <p role="status">Проверяем вход…</p>;
  if (!auth.session)
    return (
      <SafeLink className="text-link" href="/auth/login?next=/app/settings/areas">
        Войти для настройки направлений
      </SafeLink>
    );
  return (
    <>
      <Labels kind="areas" ownerId={auth.session.account.id} />
      <Labels kind="tags" ownerId={auth.session.account.id} />
    </>
  );
}
function Labels({ kind, ownerId }: { kind: "areas" | "tags"; ownerId: string }) {
  const confirmDiscard = useConfirmDiscard();
  const cache = useQueryClient();
  const key = ["account", ownerId, kind];
  const [editing, setEditing] = useState<Label | null>(null);
  const query = useQuery({
    queryKey: key,
    queryFn: async ({ signal }) => {
      await requireAccount();
      const result = await createApiClient().GET(
        kind === "areas" ? "/api/v1/areas" : "/api/v1/tags",
        { signal },
      );
      return z.array(labelSchema).parse(result.data?.data);
    },
  });
  const catalog = useQuery({
    queryKey: ["account", ownerId, "area-catalog"],
    enabled: kind === "areas",
    queryFn: async () => {
      await requireAccount();
      const result = await createApiClient().GET("/api/v1/catalog/areas");
      return z.array(inputSchema).parse(result.data?.data);
    },
  });
  if (query.isPending)
    return <p role="status">Загружаем {kind === "areas" ? "направления" : "теги"}…</p>;
  if (!query.data)
    return (
      <Card>
        <p role="alert">Не удалось загрузить список.</p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  const count = query.data.filter((item) => !item.archived).length;
  return (
    <Card className="settings-card">
      <h2>{kind === "areas" ? "Направления жизни" : "Ваши теги"}</h2>
      <p className="ui-hint">
        {kind === "areas" ? `${count} из 12 активных направлений. ` : ""}Архив сохраняет записи и их
        связи.
      </p>
      <ul className="label-list">
        {query.data.map((item) => {
          const Icon = icons[item.icon];
          return (
            <li key={item.id}>
              <span
                className="label-color"
                style={{ backgroundColor: item.color }}
                aria-hidden="true"
              />
              <Icon size={18} aria-hidden="true" />
              <span>
                {item.name}
                {item.archived ? " · В архиве" : ""}
              </span>
              <Button
                variant="ghost"
                onClick={() => {
                  if (confirmDiscard()) setEditing(item);
                }}
              >
                Изменить<span className="sr-only"> {item.name}</span>
              </Button>
            </li>
          );
        })}
      </ul>
      <LabelForm
        key={editing?.id ?? "new"}
        kind={kind}
        existing={editing}
        catalog={catalog.data ?? []}
        onSaved={() => {
          setEditing(null);
          void cache.invalidateQueries({ queryKey: key });
        }}
        onCancel={() => setEditing(null)}
      />
    </Card>
  );
}
function LabelForm({
  kind,
  existing,
  catalog,
  onSaved,
  onCancel,
}: {
  kind: "areas" | "tags";
  existing: Label | null;
  catalog: z.infer<typeof inputSchema>[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [saved, setSaved] = useState(false);
  const flow = useValidatedForm<z.infer<typeof inputSchema>>({
    schema: inputSchema,
    defaultValues: existing
      ? inputSchema.parse(existing)
      : { name: "", color: "#588f85", icon: kind === "areas" ? "sprout" : "tag", archived: false },
    fields: ["name", "color", "icon", "archived"],
    save: async (values) => {
      await requireAccount();
      await withAuthLock(async () => {
        const api = await authClient();
        if (existing)
          await api.PATCH(kind === "areas" ? "/api/v1/areas/{id}" : "/api/v1/tags/{id}", {
            params: {
              path: { id: existing.id },
              header: { ...authParams().header, "If-Match": `"${existing.version}"` },
            },
            body: values,
          });
        else
          await api.POST(kind === "areas" ? "/api/v1/areas" : "/api/v1/tags", {
            params: authParams(),
            body: values,
          });
      });
      flow.form.reset();
      setSaved(true);
      onSaved();
      return existing
        ? values
        : {
            name: "",
            color: "#588f85",
            icon: kind === "areas" ? ("sprout" as const) : ("tag" as const),
            archived: false,
          };
    },
  });
  return (
    <>
      <h3>{existing ? "Изменить запись" : kind === "areas" ? "Новое направление" : "Новый тег"}</h3>
      {!existing && kind === "areas" && (
        <div className="area-catalog">
          {catalog.map((item) => (
            <Button
              key={item.name}
              variant="secondary"
              onClick={() => {
                flow.form.reset(item);
                flow.form.setValue("name", item.name, { shouldDirty: true });
                setSaved(false);
              }}
            >
              {item.name}
            </Button>
          ))}
        </div>
      )}
      <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
        <Input
          label={kind === "areas" ? "Название направления" : "Название тега"}
          maxLength={60}
          error={flow.form.formState.errors.name?.message}
          {...flow.form.register("name")}
        />
        <Input
          label="Цвет"
          type="color"
          {...flow.form.register("color")}
          value={flow.form.watch("color")}
        />
        <div className="color-swatches" role="group" aria-label="Готовые цвета">
          {["#6554d9", "#2b8c78", "#d76b4c", "#3688c2", "#b17f20", "#ae5c9e"].map((color) => (
            <button
              type="button"
              key={color}
              style={{ backgroundColor: color }}
              aria-label={`Цвет ${color}`}
              aria-pressed={flow.form.watch("color") === color}
              onClick={() => flow.form.setValue("color", color, { shouldDirty: true })}
            />
          ))}
        </div>
        <fieldset className="icon-picker">
          <legend>Значок</legend>
          <div>
            {(Object.keys(icons) as (keyof typeof icons)[]).map((value) => {
              const Icon = icons[value];
              return (
                <button
                  type="button"
                  key={value}
                  aria-pressed={flow.form.watch("icon") === value}
                  onClick={() => flow.form.setValue("icon", value, { shouldDirty: true })}
                >
                  <Icon size={22} aria-hidden="true" />
                  <span>{iconLabels[value]}</span>
                </button>
              );
            })}
          </div>
        </fieldset>
        {existing && (
          <label className="auth-checkbox">
            <input type="checkbox" {...flow.form.register("archived")} />В архиве
          </label>
        )}
        <Button type="submit">
          {existing
            ? "Сохранить изменения"
            : kind === "areas"
              ? "Создать направление"
              : "Создать тег"}
        </Button>
        {existing && (
          <Button
            variant="ghost"
            onClick={() => {
              flow.form.reset();
              onCancel();
            }}
          >
            Отмена
          </Button>
        )}
      </Form>
      {saved && <p role="status">Запись сохранена.</p>}
      {flow.error && (
        <p className="ui-hint">
          При конфликте версии отмените редактирование и откройте запись снова.
        </p>
      )}
    </>
  );
}
