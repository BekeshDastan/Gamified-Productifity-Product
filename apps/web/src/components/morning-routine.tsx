"use client";
import { useState } from "react";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { getMorning, saveMorning } from "@/lib/journal";
import { PlanEditor } from "./plan-editor";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
export function MorningRoutine() {
  const auth = useAuth(),
    settings = useSettings();
  if (!auth.session || !settings.data) return <p>Загружаем утренний план…</p>;
  return (
    <Morning
      key={auth.session.account.id}
      account={auth.session.account.id}
      timezone={settings.data.timezone}
    />
  );
}
function Morning({ account, timezone }: { account: string; timezone: string }) {
  const cache = useQueryClient(),
    discard = useConfirmDiscard();
  const [date, setDate] = useState(() => DateTime.now().setZone(timezone).toISODate()!),
    [notice, setNotice] = useState("");
  const q = useQuery({
    queryKey: accountQueryKey(account, "morning", date),
    queryFn: ({ signal }) => getMorning(date, signal),
  });
  return (
    <div className="journal-layout">
      <Card>
        <h2>Утренний ритуал</h2>
        <p>
          Оцените время, выберите задачи и до трёх главных. Подтверждение сохраняет тот же план дня.
          Повторное прохождение добавляет изменение с причиной и сохраняет исходный снимок.
        </p>
        <label className="ui-field">
          Дата утреннего ритуала
          <input
            className="ui-input"
            type="date"
            value={date}
            onChange={(e) => {
              if (e.target.value && discard()) {
                setDate(e.target.value);
                setNotice("");
              }
            }}
          />
        </label>
        <p>{timezone}</p>
        {notice && <p role="status">{notice}</p>}
        {q.isError && (
          <p role="alert">
            Не удалось загрузить план. <Button onClick={() => void q.refetch()}>Повторить</Button>
          </p>
        )}
      </Card>
      {q.data && (
        <PlanEditor
          key={date + ":" + q.data.plan.version + ":" + q.data.run?.version}
          plan={q.data.plan}
          title="Выбрать утренний план"
          submitLabel="Подтвердить утренний план"
          saveOverride={async (input, reason, key) => {
            await saveMorning(date, q.data!, input, reason, key);
          }}
          onChanged={() => {
            setNotice("Утренний план подтверждён.");
            void cache.invalidateQueries({ queryKey: accountQueryKey(account) });
          }}
        />
      )}
    </div>
  );
}
