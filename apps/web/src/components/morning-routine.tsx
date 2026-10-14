"use client";
import { useState } from "react";
import { DateTime } from "luxon";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { useSettings } from "@/lib/settings";
import { accountQueryKey } from "@/lib/api/query-provider";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { getMorning, saveMorning, getRoutineTemplate, type RoutineTemplate } from "@/lib/journal";
import { PlanEditor } from "./plan-editor";
import { Card } from "./ui/card";
import { draftKey } from "@/lib/private-drafts";
import { z } from "zod";
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
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const template = useQuery({
    queryKey: accountQueryKey(account, "routine-template", "morning"),
    queryFn: ({ signal }) => getRoutineTemplate("morning", signal),
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
      {q.data && template.data && (
        <MorningEditor
          key={date + ":" + q.data.plan.version + ":" + q.data.run?.version}
          account={account}
          date={date}
          current={q.data}
          template={template.data}
          saved={async () => {
            await cache.invalidateQueries({ queryKey: accountQueryKey(account) });
            setNotice("Утренний план подтверждён.");
          }}
        />
      )}
    </div>
  );
}

function MorningEditor({
  account,
  date,
  current,
  template,
  saved,
}: {
  account: string;
  date: string;
  current: Awaited<ReturnType<typeof getMorning>>;
  template: RoutineTemplate;
  saved: () => Promise<void>;
}) {
  const [frozen] = useState(template);
  const input = z
    .object({ checked: z.array(z.string()).nullable().optional() })
    .safeParse(current.run?.payload.input);
  const checked = (input.success ? (input.data.checked ?? []) : []).filter((id) =>
    frozen.steps.some((step) => step.id === id),
  );
  return (
    <PlanEditor
      plan={current.plan}
      title="Выбрать утренний план"
      submitLabel="Подтвердить утренний план"
      checklist={frozen}
      initialChecked={checked}
      draftStorageKey={draftKey(account, "morning:" + date)}
      saveOverride={async (input, reason, key, checked) => {
        await saveMorning(date, current, input, reason, key, frozen.version, checked);
      }}
      onChanged={saved}
    />
  );
}
