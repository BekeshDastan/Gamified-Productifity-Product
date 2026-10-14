"use client";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import type { Goal } from "@/lib/goals";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { GoalTemplates } from "./goal-templates";
export function GoalWizard() {
  const account = useAuth().session?.account.id;
  if (!account) return null;
  return <WizardWorkspace key={account} account={account} />;
}
function WizardWorkspace({ account }: { account: string }) {
  const [saved, setSaved] = useState<Goal | null>(null),
    cache = useQueryClient();
  return (
    <>
      <GoalTemplates
        guided
        onSaved={(g) => {
          setSaved(g);
          void cache.invalidateQueries({ queryKey: ["account", account] });
        }}
      />
      {saved && (
        <p role="status">
          Цель и показанные шаги созданы.{" "}
          <SafeLink href={"/app/goals/" + saved.id}>Открыть «{saved.title}»</SafeLink>
        </p>
      )}
    </>
  );
}
