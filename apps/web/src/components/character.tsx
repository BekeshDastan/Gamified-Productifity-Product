"use client";
import Image from "next/image";
import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { ApiError } from "@/lib/api/client";
import {
  useCharacter,
  equipCharacter,
  slots,
  slotNames,
  type CharacterProfile,
} from "@/lib/character";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { SafeLink } from "@/lib/forms/unsaved-changes";
export function Character() {
  const auth = useAuth(),
    query = useCharacter();
  if (!auth.session)
    return (
      <Card>
        <p>Войдите, чтобы увидеть своего персонажа.</p>
        <SafeLink href="/auth/login">Войти</SafeLink>
      </Card>
    );
  if (!query.data)
    return (
      <Card>
        <p role={query.isError ? "alert" : "status"}>
          {query.isError ? "Не удалось загрузить персонажа." : "Загружаем ваш образ…"}
        </p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  return (
    <>
      <Button variant="secondary" onClick={() => void query.refetch()}>
        Обновить персонажа
      </Button>
      <CharacterEquipment profile={query.data} />
      <SkillTrees areas={query.data.areas} />
    </>
  );
}
function CharacterEquipment({ profile: p }: { profile: CharacterProfile }) {
  const auth = useAuth(),
    cache = useQueryClient(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const keyRef = useRef({ body: "", key: "" }),
    lockedRef = useRef(false);
  async function equip(slot: (typeof slots)[number], id: string) {
    if (lockedRef.current) return;
    lockedRef.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    const body = JSON.stringify({ slot, id, version: p.version });
    if (body !== keyRef.current.body) keyRef.current = { body, key: crypto.randomUUID() };
    try {
      const fresh = await equipCharacter(p, slot, id, keyRef.current.key);
      cache.setQueryData(["account", auth.session?.account.id, "character"], fresh);
      setMessage("Образ сохранён");
    } catch (e) {
      setError(
        e instanceof ApiError ? e.message : "Не удалось сохранить образ. Повторите действие.",
      );
    } finally {
      lockedRef.current = false;
      setBusy(false);
    }
  }
  return (
    <div className="character-layout">
      <Card>
        <h2>Ваш образ · уровень {p.level.number}</h2>
        <div
          className="character-avatar"
          role="img"
          aria-label={
            "Образ: " +
            slots
              .map((s) => p.inventory.find((i) => i.id === p.equipment[s])?.name ?? slotNames[s])
              .join(", ")
          }
        >
          {slots.map((slot) => {
            const i = p.inventory.find((i) => i.id === p.equipment[slot]);
            return i ? (
              <Image
                key={slot}
                src={i.url}
                width={240}
                height={240}
                alt=""
                unoptimized
                aria-hidden="true"
              />
            ) : null;
          })}
        </div>
        <p className="ui-hint">Все предметы косметические. Стартовый набор бесплатный.</p>
        <p>
          <SafeLink href="/app/achievements">Достижения и открытия</SafeLink>
        </p>
        {message && <p role="status">{message}</p>}
        {error && <p role="alert">{error}</p>}
      </Card>
      <Card>
        <h2>Инвентарь</h2>
        {slots.map((slot) => (
          <details key={slot}>
            <summary>
              {slotNames[slot]} · {p.inventory.filter((i) => i.slot === slot && i.owned).length}{" "}
              открыто
            </summary>
            <ul className="character-items">
              {p.inventory
                .filter((i) => i.slot === slot)
                .map((i) => (
                  <li key={i.id}>
                    <Image src={i.url} width={80} height={80} alt="" unoptimized />
                    <h3>{i.name}</h3>
                    <p>
                      {i.revoked
                        ? "Условие исправлено"
                        : i.owned
                          ? "Открыто"
                          : i.achievement
                            ? "За достижение «Первый шаг»"
                            : "Откроется на уровне " + i.level}
                    </p>
                    <p className="ui-hint">
                      {i.rarity === "common"
                        ? "Обычный"
                        : i.rarity === "uncommon"
                          ? "Необычный"
                          : "Редкий"}
                    </p>
                    {i.owned &&
                      (p.equipment[slot] === i.id ? (
                        <p>Надето</p>
                      ) : (
                        <Button
                          disabled={busy}
                          variant="secondary"
                          onClick={() => void equip(slot, i.id)}
                        >
                          Надеть {i.name}
                        </Button>
                      ))}
                  </li>
                ))}
            </ul>
          </details>
        ))}
      </Card>
    </div>
  );
}
export function SkillTrees({ areas }: { areas: CharacterProfile["areas"] }) {
  return (
    <Card>
      <h2>Дерево активности по направлениям</h2>
      <p className="ui-hint">
        Это ваша активность в приложении. Условия узлов одинаковы для всех направлений. XP
        направления уже входит в общий опыт.
      </p>
      {areas.length === 0 ? (
        <p>
          Добавьте направление в{" "}
          <SafeLink href="/app/settings/areas">настройках направлений</SafeLink> и выберите его в
          задаче или привычке.
        </p>
      ) : (
        areas.map((a) => (
          <section key={a.id} className="skill-area">
            <h3>
              {a.name}
              {a.archived ? " · в архиве" : ""}
            </h3>
            <p>
              {a.xp} XP · уровень направления {a.level.number}
            </p>
            <svg className="skill-map" viewBox="0 0 400 170" aria-hidden="true">
              <path
                d="M50 85L165 35L335 85M50 85L165 135L335 85"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              />
              {a.skills.map((s, i) => {
                const x = [50, 165, 165, 335][i] ?? 200,
                  y = [85, 35, 135, 85][i] ?? 85;
                return (
                  <g key={s.id}>
                    <circle
                      cx={x}
                      cy={y}
                      r="18"
                      fill={s.unlocked ? "var(--primary)" : "var(--card)"}
                      stroke="currentColor"
                      strokeWidth="2"
                    />
                    <text
                      x={x}
                      y={y + 5}
                      textAnchor="middle"
                      fill={s.unlocked ? "var(--primary-foreground)" : "currentColor"}
                    >
                      {i + 1}
                    </text>
                  </g>
                );
              })}
            </svg>
            <ol className="skill-list" aria-label={"Условия навыков: " + a.name}>
              {a.skills.map((s) => (
                <li key={s.id}>
                  <strong>
                    {s.name} · {s.unlocked ? "Открыт" : "Закрыт"}
                  </strong>
                  <p>
                    {s.threshold}{" "}
                    {s.metric === "xp"
                      ? "XP направления"
                      : "зачтённых норм привычек с правом награды"}{" "}
                    · сейчас {s.current}
                  </p>
                  <p className="ui-hint">
                    {s.depends_on.length
                      ? "Сначала: " +
                        s.depends_on
                          .map((id) => a.skills.find((n) => n.id === id)?.name ?? id)
                          .join(", ")
                      : "Без предварительных узлов"}
                  </p>
                </li>
              ))}
            </ol>
          </section>
        ))
      )}
    </Card>
  );
}
