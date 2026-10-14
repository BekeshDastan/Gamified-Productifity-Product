"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import * as api from "@/lib/challenges";
import { questFiles } from "@/lib/quest-files";
import { useUnsavedChanges } from "@/lib/forms/unsaved-changes";
import { useChallengeAction, useChallengeKey, units } from "./challenges";
import { QuestPicker } from "./quest-structure";
import { Input, Select } from "./ui/field";
import { Button } from "./ui/button";
export function CheckinList({
  account,
  run,
  member,
  changed,
}: {
  account: string;
  run: api.ChallengeRun;
  member: api.ChallengeMember;
  changed: () => void;
}) {
  const [before, setBefore] = useState(""),
    [amount, setAmount] = useState(1),
    [note, setNote] = useState(""),
    [generation, setGeneration] = useState(0),
    [dirty, setDirty] = useState(false),
    key = useChallengeKey(),
    action = useChallengeAction(),
    params = useSearchParams();
  useUnsavedChanges(dirty);
  const q = useQuery({
    queryKey: ["account", account, "challenge-checkins", run.id, before],
    queryFn: ({ signal }) => api.challengeCheckins(run.id, before, signal),
  });
  return (
    <div>
      <h3>Мои самоотчёты</h3>
      <p className="ui-hint">
        Количество заявлено вами. Файл поддерживает запись, но не является проверкой результата.
        Новые отметки сохраняются сегодняшним днём в поясе испытания. Исправление сохраняет исходную
        запись.
      </p>
      {member.state === "joined" && run.state === "open" && (
        <form
          aria-busy={action.pending}
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              const input = { amount, note, unit: run.rules.unit };
              const operation = key({ input, generation });
              await api.saveCheckin(
                run.id,
                {
                  ...input,
                  operation_id: operation,
                  unit: run.rules.unit as "count" | "pages" | "minutes",
                },
                operation,
              );
              setNote("");
              setDirty(false);
              setGeneration((v) => v + 1);
              changed();
            });
          }}
        >
          <Input
            label="Количество самоотчёта"
            type="number"
            min={1}
            max={run.rules.daily_cap}
            step={1}
            required
            value={amount}
            onChange={(e) => {
              setAmount(e.target.valueAsNumber);
              setDirty(true);
            }}
          />
          <Input
            label="Заметка к самоотчёту"
            value={note}
            maxLength={2000}
            onChange={(e) => {
              setNote(e.target.value);
              setDirty(true);
            }}
          />
          <Button type="submit" disabled={action.pending}>
            Сохранить самоотчёт
          </Button>
        </form>
      )}
      {action.error && <p role="alert">{action.error}</p>}
      {q.isError && (
        <p role="alert">
          Не удалось загрузить самоотчёты.{" "}
          <Button onClick={() => void q.refetch()}>Повторить</Button>
        </p>
      )}
      {q.data?.items.map((c) => (
        <CheckinRow
          key={c.id + ":" + c.version}
          account={account}
          run={run}
          current={c}
          expanded={params.get("checkin_id") === c.id}
          changed={changed}
        />
      ))}
      {before && (
        <Button variant="secondary" onClick={() => setBefore("")}>
          К новым самоотчётам
        </Button>
      )}
      {q.data?.next && (
        <Button variant="secondary" onClick={() => setBefore(q.data!.next)}>
          Более ранние самоотчёты
        </Button>
      )}
    </div>
  );
}
function CheckinRow({
  account,
  run,
  current: c,
  expanded,
  changed,
}: {
  account: string;
  run: api.ChallengeRun;
  current: api.Checkin;
  expanded: boolean;
  changed: () => void;
}) {
  const [amount, setAmount] = useState(c.amount),
    [note, setNote] = useState(c.note),
    [quest, setQuest] = useState(""),
    [file, setFile] = useState(""),
    [share, setShare] = useState(false),
    action = useChallengeAction(),
    key = useChallengeKey();
  useUnsavedChanges(amount !== c.amount || note !== c.note);
  const evidence = useQuery({
      queryKey: ["account", account, "challenge-evidence", run.id, c.id],
      queryFn: ({ signal }) => api.challengeEvidence(run.id, c.id, signal),
    }),
    files = useQuery({
      queryKey: ["account", account, "quest-files", quest],
      queryFn: ({ signal }) => questFiles(quest, signal),
      enabled: !!quest,
    });
  return (
    <details open={expanded || undefined} id={"checkin-" + c.id}>
      <summary>
        {c.date} · {c.amount} {units[c.unit]} · самоотчёт {c.amount === 0 && "(отменён)"}
      </summary>
      <p>{c.note}</p>
      <form
        aria-busy={action.pending}
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const input = { amount, note };
            await api.correctCheckin(run.id, c, input, key({ ...input, version: c.version }));
            changed();
          });
        }}
      >
        <Input
          label={"Исправленное количество: " + c.date}
          type="number"
          min={0}
          max={c.initial_amount}
          step={1}
          required
          value={amount}
          onChange={(e) => setAmount(e.target.valueAsNumber)}
        />
        <Input
          label={"Уточнение записи: " + c.date}
          maxLength={2000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <p className="ui-hint">
          От 0 до исходных {c.initial_amount} {units[c.unit]}. Для новых действий создайте отдельную
          отметку.
        </p>
        <Button type="submit" disabled={action.pending}>
          Сохранить исправление
        </Button>
      </form>
      <h4>Личные доказательства</h4>
      {evidence.isError && (
        <p role="alert">
          Не удалось загрузить файлы.{" "}
          <Button onClick={() => void evidence.refetch()}>Повторить</Button>
        </p>
      )}
      <ul>
        {evidence.data?.map((v) => (
          <li key={v.attachment_id}>
            {v.available ? <a href={v.url}>{v.name}</a> : v.name} ·{" "}
            {v.visibility === "private" ? "Только вам" : "Есть согласие для участников"}{" "}
            {v.visibility === "participants" && (
              <Button
                variant="ghost"
                disabled={action.pending}
                onClick={() =>
                  void action.run(async () => {
                    await api.setChallengeEvidence(
                      run.id,
                      c.id,
                      v.attachment_id,
                      v.version,
                      { visibility: "private", consent: false },
                      crypto.randomUUID(),
                    );
                    changed();
                  })
                }
              >
                Отозвать согласие
              </Button>
            )}
          </li>
        ))}
      </ul>
      <p className="ui-hint">
        Выберите проверенный файл из своей задачи. Здесь можно записать отдельное согласие для
        участников; скачивание остаётся доступно только вам до настройки совместного доступа.
      </p>
      <QuestPicker
        label="Задача с файлом"
        value={quest}
        onChange={(id) => {
          setQuest(id);
          setFile("");
          setShare(false);
        }}
      />
      {files.isError && (
        <p role="alert">
          Не удалось загрузить файлы задачи.{" "}
          <Button onClick={() => void files.refetch()}>Повторить</Button>
        </p>
      )}
      <Select
        label="Файл для доказательства"
        value={file}
        onChange={(e) => setFile(e.target.value)}
      >
        <option value="">Выберите файл</option>
        {files.data
          ?.filter((v) => v.verdict === "clean")
          .map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
      </Select>
      <label>
        <input type="checkbox" checked={share} onChange={(e) => setShare(e.target.checked)} />{" "}
        Отдельно разрешаю показывать этот файл участникам этого испытания, когда совместный доступ
        станет доступен
      </label>
      <Button
        variant="secondary"
        disabled={!file || action.pending || evidence.isPending}
        onClick={() =>
          void action.run(async () => {
            const existing = evidence.data?.find((v) => v.attachment_id === file);
            await api.setChallengeEvidence(
              run.id,
              c.id,
              file,
              existing?.version ?? 0,
              { visibility: share ? "participants" : "private", consent: share },
              crypto.randomUUID(),
            );
            setFile("");
            setShare(false);
            changed();
          })
        }
      >
        Прикрепить выбранный файл
      </Button>
      {action.error && <p role="alert">{action.error}</p>}
    </details>
  );
}
