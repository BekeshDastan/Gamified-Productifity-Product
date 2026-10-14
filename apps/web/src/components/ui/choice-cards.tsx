"use client";
import { Check, type LucideIcon } from "lucide-react";

export function ChoiceCards<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string; description: string; icon: LucideIcon; tone?: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <fieldset className="choice-card-field">
      <legend>{label}</legend>
      <div className="choice-card-grid">
        {options.map(({ value: id, label: title, description, icon: Icon, tone = "violet" }) => (
          <button
            type="button"
            key={id}
            aria-label={title}
            aria-pressed={id === value}
            data-tone={tone}
            onClick={() => {
              if (id !== value) onChange(id);
            }}
          >
            <span className="choice-card-icon" aria-hidden="true">
              <Icon size={23} />
            </span>
            <strong>{title}</strong>
            <small>{description}</small>
            {id === value && <Check className="choice-card-check" size={17} aria-hidden="true" />}
          </button>
        ))}
      </div>
    </fieldset>
  );
}
