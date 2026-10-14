"use client";
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import {
  ArrowRight,
  Check,
  ChevronLeft,
  ChevronRight,
  Sparkles,
  SlidersHorizontal,
} from "lucide-react";
import { DateTime } from "luxon";

function subscribeMotion(change: () => void) {
  const media = window.matchMedia("(prefers-reduced-motion: reduce)");
  const observer = new MutationObserver(change);
  media.addEventListener("change", change);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-motion"],
  });
  return () => {
    media.removeEventListener("change", change);
    observer.disconnect();
  };
}
export function useMotionAllowed() {
  return useSyncExternalStore(
    subscribeMotion,
    () =>
      !window.matchMedia("(prefers-reduced-motion: reduce)").matches &&
      document.documentElement.dataset.motion !== "reduce",
    () => false,
  );
}

export function CountUp({ value }: { value: number }) {
  const [shown, setShown] = useState(value);
  const previous = useRef(value);
  useEffect(() => {
    const start = previous.current;
    previous.current = value;
    const reduced =
      window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
      document.documentElement.dataset.motion === "reduce";
    let frame = 0;
    const began = performance.now();
    const tick = (now: number) => {
      const progress = reduced ? 1 : Math.min(1, (now - began) / 550);
      setShown(Math.round(start + (value - start) * (1 - Math.pow(1 - progress, 3))));
      if (progress < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [value]);
  return <span aria-label={String(value)}>{shown.toLocaleString("ru-RU")}</span>;
}

export function StatTile({
  label,
  value,
  detail,
  icon,
  tone = "violet",
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  icon?: ReactNode;
  tone?: "violet" | "mint" | "coral" | "blue" | "gold";
}) {
  return (
    <div className="stat-tile" data-tone={tone} role="group" aria-label={label}>
      <span className="stat-icon" aria-hidden="true">
        {icon ?? <Sparkles size={20} />}
      </span>
      <span className="stat-label">{label}</span>
      <strong>{typeof value === "number" ? <CountUp value={value} /> : value}</strong>
      {detail && <small>{detail}</small>}
    </div>
  );
}

export function ChoiceChips<T extends string | number>({
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  label: string;
  value: T;
  options: { value: T; label: string; icon?: ReactNode }[];
  onChange: (value: T) => void;
  disabled?: boolean;
}) {
  return (
    <div className="choice-chips" role="group" aria-label={label}>
      {options.map((option) => (
        <button
          type="button"
          key={option.value}
          disabled={disabled}
          aria-pressed={option.value === value}
          onClick={() => onChange(option.value)}
        >
          {option.icon && <span aria-hidden="true">{option.icon}</span>}
          {option.label}
          {option.value === value && <Check size={14} aria-hidden="true" />}
        </button>
      ))}
    </div>
  );
}

export function FilterPanel({
  title = "Фильтры и период",
  children,
  count,
}: {
  title?: string;
  children: ReactNode;
  count?: number;
}) {
  return (
    <details className="filter-panel">
      <summary>
        <SlidersHorizontal size={16} aria-hidden="true" />
        {title}
        {!!count && <span className="count-badge">{count}</span>}
      </summary>
      <div className="filter-panel-fields">{children}</div>
    </details>
  );
}

export function EmptyScene({
  title,
  description,
  children,
  tone = "violet",
}: {
  title: string;
  description?: string;
  children?: ReactNode;
  tone?: string;
}) {
  return (
    <div className="empty-scene" data-tone={tone}>
      <div className="empty-orbit" aria-hidden="true">
        <i />
        <i />
        <span>
          <Sparkles size={30} />
        </span>
      </div>
      <div>
        <h3>{title}</h3>
        {description && <p>{description}</p>}
        {children && (
          <div className="empty-action">
            {children}
            <ArrowRight size={16} aria-hidden="true" />
          </div>
        )}
      </div>
    </div>
  );
}

export function WeekStrip({
  date,
  onChange,
  weekStart = 1,
}: {
  date: string;
  onChange: (date: string) => void;
  weekStart?: number;
}) {
  const current = DateTime.fromISO(date);
  const start = current.minus({
    days: weekStart === 7 ? current.weekday % 7 : current.weekday - 1,
  });
  return (
    <div className="week-strip" aria-label="Выбор дня недели">
      <button
        type="button"
        aria-label="Предыдущая неделя"
        onClick={() => onChange(current.minus({ weeks: 1 }).toISODate()!)}
      >
        <ChevronLeft size={18} />
      </button>
      {Array.from({ length: 7 }, (_, i) => start.plus({ days: i })).map((day) => (
        <button
          type="button"
          key={day.toISODate()}
          aria-label={day.setLocale("ru").toFormat("d MMMM, cccc")}
          aria-pressed={day.toISODate() === date}
          onClick={() => onChange(day.toISODate()!)}
        >
          <small>{day.setLocale("ru").toFormat("ccc")}</small>
          <strong>{day.day}</strong>
          <i />
        </button>
      ))}
      <button
        type="button"
        aria-label="Следующая неделя"
        onClick={() => onChange(current.plus({ weeks: 1 }).toISODate()!)}
      >
        <ChevronRight size={18} />
      </button>
    </div>
  );
}
