"use client";
import Image from "next/image";
import { useRef, useState } from "react";
import { useWatch } from "react-hook-form";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import {
  useRewardCatalog,
  customRewardInput,
  saveCustomReward,
  purchaseReward,
  changeRedemption,
  getRedemptions,
  type RewardDefinition,
  type Redemption,
  type PurchaseInput,
} from "@/lib/rewards";
import { useValidatedForm } from "@/lib/forms/use-validated-form";
import { useConfirmDiscard, SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { Input } from "./ui/field";
import { Form } from "./ui/form";
export function Rewards() {
  const auth = useAuth(),
    query = useRewardCatalog(),
    cache = useQueryClient(),
    discard = useConfirmDiscard();
  const [editing, setEditing] = useState<RewardDefinition | null>(),
    [selection, setSelection] = useState<{ title: string; input: PurchaseInput }>(),
    [archive, setArchive] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  const lockedRef = useRef(false);
  const changed = () =>
    void cache.invalidateQueries({ queryKey: ["account", auth.session?.account.id] });
  if (!auth.session)
    return (
      <Card>
        <p>Войдите, чтобы открыть личные награды.</p>
        <SafeLink href="/auth/login">Войти</SafeLink>
      </Card>
    );
  if (!query.data)
    return (
      <Card>
        <p role={query.isError ? "alert" : "status"}>
          {query.isError ? "Не удалось загрузить магазин." : "Загружаем награды…"}
        </p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  const data = query.data;
  function choose(title: string, input: Omit<PurchaseInput, "operation_id">) {
    setError("");
    setSelection({ title, input: { ...input, operation_id: crypto.randomUUID() } });
  }
  async function buy() {
    if (!selection || lockedRef.current) return;
    lockedRef.current = true;
    setBusy(true);
    setError("");
    try {
      const saved = await purchaseReward(selection.input);
      setMessage(
        saved.status === "pending"
          ? "Покупка сохранена. Подтвердите получение после использования награды."
          : "Предмет добавлен в инвентарь.",
      );
      setSelection(undefined);
      changed();
    } catch (e) {
      setError(
        e instanceof ApiError ? e.message : "Не удалось подтвердить покупку. Повтор безопасен.",
      );
    } finally {
      lockedRef.current = false;
      setBusy(false);
    }
  }
  return (
    <>
      <Card>
        <h2>Ваш баланс: {data.balance.available_coins} монет</h2>
        {data.balance.coin_correction > 0 && (
          <p>
            Корректировка: {data.balance.coin_correction} монет. Новые поступления сначала покроют
            её.
          </p>
        )}
        <p className="ui-hint">
          Это виртуальные монеты за допустимые результаты. Реальную награду вы организуете
          самостоятельно.
        </p>
        <SafeLink href="/app/progress">История поступлений, расходов и возвратов</SafeLink>
        <Button variant="secondary" onClick={() => void query.refetch()}>
          Обновить каталог
        </Button>
      </Card>
      {message && <p role="status">{message}</p>}
      <Card>
        <h2>Личные награды</h2>
        <Button onClick={() => setEditing(null)}>Создать личную награду</Button>
        <Button variant="secondary" onClick={() => setArchive(!archive)}>
          {archive ? "Показать активные награды" : "Показать архив наград"}
        </Button>
        <div className="reward-grid">
          {data.custom
            .filter((d) => d.archived === archive)
            .map((d) => (
              <section key={d.id}>
                <h3>{d.title}</h3>
                <p>{d.description}</p>
                <p>
                  {d.cost} монет ·{" "}
                  {d.archived ? "В архиве" : d.available ? "Доступна" : "Недоступна"}
                </p>
                <Button variant="secondary" onClick={() => setEditing(d)}>
                  Изменить награду {d.title}
                </Button>
                {!d.archived && d.available && (
                  <Button
                    onClick={() =>
                      choose(d.title, {
                        kind: "custom",
                        target_id: d.id,
                        expected_cost: d.cost,
                        expected_version: d.version,
                      })
                    }
                  >
                    Купить {d.title}
                  </Button>
                )}
              </section>
            ))}
        </div>
        {data.custom.filter((d) => d.archived === archive).length === 0 && (
          <p>Здесь пока нет наград.</p>
        )}
      </Card>
      <Card>
        <h2>Косметика</h2>
        <p>
          Один предмет можно открыть покупкой один раз. Уже открытые предметы повторно не продаются.
        </p>
        <div className="reward-grid">
          {data.cosmetics.map((i) => (
            <section key={i.id}>
              <Image src={i.url} width={80} height={80} alt="" unoptimized />
              <h3>{i.name}</h3>
              <p>
                {i.cost} монет · {i.owned ? "Уже открыто" : "Можно открыть покупкой"}
              </p>
              {!i.owned && (
                <Button
                  variant="secondary"
                  onClick={() =>
                    choose(i.name, {
                      kind: "cosmetic",
                      target_id: i.id,
                      expected_cost: i.cost,
                      expected_version: 1,
                    })
                  }
                >
                  Купить {i.name}
                </Button>
              )}
            </section>
          ))}
        </div>
      </Card>
      <RewardHistory changed={changed} />
      <Dialog
        title="Личная награда"
        description="Настройте цену и доступность. Существующие покупки сохраняют исходную цену."
        open={editing !== undefined}
        onOpenChange={(open) => {
          if (!open && discard()) setEditing(undefined);
        }}
      >
        {editing !== undefined && (
          <RewardEditor
            key={editing?.id ?? "new"}
            existing={editing}
            saved={() => {
              setEditing(undefined);
              changed();
            }}
          />
        )}
      </Dialog>
      <Dialog
        title="Подтверждение покупки"
        description="Проверьте выбор и списание до подтверждения."
        open={!!selection}
        onOpenChange={(open) => {
          if (!open && !busy) setSelection(undefined);
        }}
      >
        {selection && (
          <>
            <h3>{selection.title}</h3>
            <p>Спишется: {selection.input.expected_cost} монет.</p>
            <p>Доступно сейчас: {data.balance.available_coins} монет.</p>
            <p>
              {selection.input.kind === "custom"
                ? "Можно вернуть монеты, пока награда не отмечена полученной."
                : "Предмет выдаётся сразу, возврат косметики недоступен."}
            </p>
            {selection.input.expected_cost > data.balance.available_coins && (
              <p role="alert">Недостаточно монет для этой покупки.</p>
            )}
            {error && <p role="alert">{error}</p>}
            <Button
              disabled={busy || selection.input.expected_cost > data.balance.available_coins}
              onClick={() => void buy()}
            >
              Подтвердить покупку
            </Button>
          </>
        )}
      </Dialog>
    </>
  );
}
function RewardEditor({
  existing,
  saved,
}: {
  existing: RewardDefinition | null;
  saved: () => void;
}) {
  const keyRef = useRef({ body: "", key: "" });
  const flow = useValidatedForm({
    schema: customRewardInput,
    defaultValues: {
      title: existing?.title ?? "",
      description: existing?.description ?? "",
      cost: existing?.cost ?? 5,
      available: existing?.available ?? true,
      archived: existing?.archived ?? false,
      price_confirmed: false,
    },
    fields: ["title", "description", "cost", "price_confirmed"],
    save: async (input) => {
      const body = JSON.stringify(input);
      if (keyRef.current.body !== body) keyRef.current = { body, key: crypto.randomUUID() };
      await saveCustomReward(existing, input, keyRef.current.key);
      saved();
    },
  });
  const cost = useWatch({ control: flow.form.control, name: "cost" });
  return (
    <Form saving={flow.saving} error={flow.error} onSubmit={flow.onSubmit}>
      <Input
        label="Название награды"
        {...flow.form.register("title")}
        error={flow.form.formState.errors.title?.message}
      />
      <Input label="Описание награды" {...flow.form.register("description")} />
      <Input
        label="Стоимость в монетах"
        type="number"
        min="0"
        max="1000000"
        {...flow.form.register("cost", { valueAsNumber: true })}
        error={flow.form.formState.errors.cost?.message}
      />
      <label>
        <input type="checkbox" {...flow.form.register("available")} /> Доступна для покупки
      </label>
      <label>
        <input type="checkbox" {...flow.form.register("archived")} /> В архиве
      </label>
      {existing && cost !== existing.cost && (
        <label>
          <input type="checkbox" {...flow.form.register("price_confirmed")} /> Подтверждаю изменение
          цены; прежние покупки сохранят свою стоимость
        </label>
      )}
      <Button type="submit">Сохранить личную награду</Button>
    </Form>
  );
}
function RewardHistory({ changed }: { changed: () => void }) {
  const auth = useAuth(),
    [selected, setSelected] = useState<{
      purchase: Redemption;
      action: "claim" | "cancel";
      key: string;
    }>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const lockedRef = useRef(false);
  const query = useInfiniteQuery({
    queryKey: ["account", auth.session?.account.id, "redemptions"],
    enabled: !!auth.session?.account.id,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => getRedemptions(pageParam, signal),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  async function apply() {
    if (!selected || lockedRef.current) return;
    lockedRef.current = true;
    setBusy(true);
    setError("");
    try {
      await changeRedemption(selected.purchase, selected.action, selected.key);
      setSelected(undefined);
      changed();
    } catch (e) {
      setError(
        e instanceof ApiError ? e.message : "Не удалось сохранить решение. Повтор безопасен.",
      );
    } finally {
      lockedRef.current = false;
      setBusy(false);
    }
  }
  return (
    <Card>
      <h2>История покупок</h2>
      {query.isError && (
        <p role="alert">
          Не удалось загрузить покупки.{" "}
          <Button onClick={() => void query.refetch()}>Повторить</Button>
        </p>
      )}
      {query.data?.pages.flatMap((p) => p.items).length === 0 && <p>Покупок пока нет.</p>}
      <ol className="reward-history">
        {query.data?.pages
          .flatMap((p) => p.items)
          .map((p) => (
            <li key={p.id}>
              <h3>{p.title}</h3>
              <p>
                Цена при покупке: {p.cost} монет ·{" "}
                {p.status === "pending"
                  ? "Ожидает получения"
                  : p.status === "claimed"
                    ? "Получено"
                    : "Отменено, монеты возвращены"}
              </p>
              {p.kind === "custom" && p.status === "pending" && (
                <>
                  {(["claim", "cancel"] as const).map((action) => (
                    <Button
                      key={action}
                      variant="secondary"
                      onClick={() => {
                        setError("");
                        setSelected({ purchase: p, action, key: crypto.randomUUID() });
                      }}
                    >
                      {action === "claim" ? "Подтвердить получение" : "Отменить покупку"}
                    </Button>
                  ))}
                </>
              )}
            </li>
          ))}
      </ol>
      {query.hasNextPage && (
        <Button disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>
          Более ранние покупки
        </Button>
      )}
      <Dialog
        title={selected?.action === "claim" ? "Получение награды" : "Возврат монет"}
        description={
          selected?.action === "claim"
            ? "После получения возврат монет недоступен."
            : "Отменяется только ещё не полученная личная награда."
        }
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !busy) setSelected(undefined);
        }}
      >
        {selected && (
          <>
            <p>
              {selected.purchase.title} · {selected.purchase.cost} монет
            </p>
            {error && <p role="alert">{error}</p>}
            <Button disabled={busy} onClick={() => void apply()}>
              Подтвердить решение
            </Button>
          </>
        )}
      </Dialog>
    </Card>
  );
}
