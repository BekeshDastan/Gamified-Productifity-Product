"use client";
import { useState } from "react";
import { useAuth } from "@/lib/auth/provider";
import { useAchievements } from "@/lib/achievements";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Select } from "./ui/field";
import { Award, LockKeyhole } from "lucide-react";
import { ChoiceChips } from "./ui/experience";
export function Achievements() {
  const auth = useAuth(),
    query = useAchievements(),
    [filter, setFilter] = useState("all");
  if (!auth.session)
    return (
      <Card>
        <p>Достижения видны только владельцу.</p>
        <SafeLink href="/auth/login">Войти</SafeLink>
      </Card>
    );
  if (!query.data)
    return (
      <Card>
        <p role={query.isError ? "alert" : "status"}>
          {query.isError ? "Не удалось загрузить достижения." : "Загружаем ваши достижения…"}
        </p>
        <Button onClick={() => void query.refetch()}>Повторить</Button>
      </Card>
    );
  const data = query.data.filter(
    (a) =>
      filter === "all" ||
      (filter === "earned" && a.active) ||
      (filter === "corrected" && a.revoked) ||
      (filter === "locked" && !a.active),
  );
  return (
    <>
      <p>
        Получено: {query.data.filter((a) => a.active).length} из {query.data.length}
      </p>
      <div className="auth-actions">
        <ChoiceChips
          label="Категория достижений"
          value={filter}
          options={[
            { value: "all", label: "Все" },
            { value: "earned", label: "Полученные" },
            { value: "locked", label: "Впереди" },
            { value: "corrected", label: "Исправленные" },
          ]}
          onChange={setFilter}
        />
        <details className="quiet-details">
          <summary>Выбор списка</summary>
          <Select
            label="Показать достижения"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          >
            <option value="all">Все</option>
            <option value="earned">Полученные</option>
            <option value="locked">Ещё не полученные</option>
            <option value="corrected">После исправления</option>
          </Select>
        </details>
        <Button variant="secondary" onClick={() => void query.refetch()}>
          Обновить достижения
        </Button>
      </div>
      <p className="ui-hint">
        Пропуск не стирает прошлые результаты. Исправление ошибочной исходной записи может изменить
        условие награды; первая дата выдачи сохраняется.
      </p>
      <div className="achievement-grid">
        {data.map((a) => (
          <Card key={a.id} className="achievement-card" data-earned={a.active}>
            <div className="achievement-medal" aria-hidden="true">
              {a.active ? <Award size={38} /> : <LockKeyhole size={31} />}
            </div>
            <h2>{a.name}</h2>
            <p>{a.description}</p>
            <p>
              <strong>
                {a.active
                  ? "Получено"
                  : a.revoked
                    ? "Условие больше не выполнено после исправления"
                    : a.available
                      ? "Впереди"
                      : "Раздел пока недоступен"}
              </strong>
            </p>
            {a.available ? (
              <>
                <progress
                  max={a.threshold}
                  value={Math.min(a.current, a.threshold)}
                  aria-label={"Прогресс достижения " + a.name}
                />
                <p>
                  {a.current} / {a.threshold}
                </p>
                <SafeLink href={a.source_url}>Посмотреть источник условия</SafeLink>
              </>
            ) : (
              <p className="ui-hint">
                Награда откроется только после подтверждённых результатов этого раздела.
              </p>
            )}
            {a.awarded_at && (
              <p className="ui-hint">
                Впервые получено:{" "}
                <time dateTime={a.awarded_at}>
                  {new Date(a.awarded_at).toLocaleDateString("ru-RU")}
                </time>
              </p>
            )}
          </Card>
        ))}
      </div>
      {data.length === 0 && <p>В этом списке пока нет достижений.</p>}
    </>
  );
}
