"use client";
import { useState } from "react";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Journal } from "./journal";
import { MorningRoutine } from "./morning-routine";
import { EveningReview } from "./evening-review";
import { WeeklyReset } from "./weekly-reset";
import { RoutineTemplates } from "./routine-templates";
import { Button } from "./ui/button";
const sections = {
  entries: "Мои записи",
  morning: "Утро",
  evening: "Вечер",
  week: "Weekly Reset",
  templates: "Чек-листы и история",
};
export function JournalWorkspace({
  initialSection,
  reviewDate,
}: {
  initialSection?: string;
  reviewDate?: string;
}) {
  const [section, setSection] = useState<keyof typeof sections>(
      initialSection && Object.hasOwn(sections, initialSection)
        ? (initialSection as keyof typeof sections)
        : "entries",
    ),
    discard = useConfirmDiscard();
  return (
    <div className="journal-layout">
      <nav aria-label="Разделы дневника" className="form-actions">
        {Object.entries(sections).map(([value, label]) => (
          <Button
            key={value}
            variant={value === section ? "primary" : "secondary"}
            aria-pressed={value === section}
            onClick={() => {
              if (value !== section && discard()) setSection(value as keyof typeof sections);
            }}
          >
            {label}
          </Button>
        ))}
      </nav>
      {section === "entries" ? (
        <Journal />
      ) : section === "morning" ? (
        <MorningRoutine />
      ) : section === "evening" ? (
        <EveningReview initialDate={reviewDate} />
      ) : section === "week" ? (
        <WeeklyReset />
      ) : (
        <RoutineTemplates />
      )}
    </div>
  );
}
