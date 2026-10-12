import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
export function Card({ className, ...props }: ComponentProps<"section">) {
  return <section className={cn("ui-card", className)} {...props} />;
}
export function Progress({
  value,
  max = 100,
  label,
}: {
  value: number | null;
  max?: number;
  label: string;
}) {
  if (value === null || !Number.isFinite(value) || !Number.isFinite(max) || max <= 0)
    return <p className="ui-hint">{label}: пока нет данных</p>;
  const current = Math.min(max, Math.max(0, value));
  return (
    <label className="ui-progress-label">
      {label}
      <span>{Math.round((current / max) * 100)}%</span>
      <progress aria-label={label} className="ui-progress" value={current} max={max} />
    </label>
  );
}
