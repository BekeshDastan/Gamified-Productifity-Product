"use client";
import { useState } from "react";
import { useConfirmDiscard } from "@/lib/forms/unsaved-changes";
import { Journal } from "./journal";
import { MorningRoutine } from "./morning-routine";
import { EveningReview } from "./evening-review";
import { WeeklyReset } from "./weekly-reset";
import { RoutineTemplates } from "./routine-templates";
import { Button } from "./ui/button";
import { BookOpen, Sunrise, Moon, CalendarRange, ListChecks } from "lucide-react";
const sectionIcons = {
  entries: BookOpen,
  morning: Sunrise,
  evening: Moon,
  week: CalendarRange,
  templates: ListChecks,
};
const sections = {
  entries: "Мои записи",
  morning: "Утро",
  evening: "Вечер",
  week: "Обзор недели",
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
      <nav aria-label="Разделы дневника" className="form-actions journal-tabs">
        {Object.entries(sections).map(([value, label]) => (
          <Button
            key={value}
            variant={value === section ? "primary" : "secondary"}
            aria-pressed={value === section}
            onClick={() => {
              if (value !== section && discard()) setSection(value as keyof typeof sections);
            }}
          >
            {(() => {
              const Icon = sectionIcons[value as keyof typeof sectionIcons];
              return <Icon size={19} aria-hidden="true" />;
            })()}
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
