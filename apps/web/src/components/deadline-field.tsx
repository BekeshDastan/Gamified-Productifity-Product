"use client";
import { toWall, fromWall } from "@/lib/calendar-time";
import { Input } from "./ui/field";
export function DeadlineField({
  value,
  zone,
  onChange,
  error,
}: {
  value: string;
  zone: string;
  onChange: (value: string) => void;
  error?: string;
}) {
  const isInstant = /(?:Z|[+-]\d\d:\d\d)$/.test(value);
  return (
    <div>
      <Input
        label="Крайний срок"
        name="deadline"
        type="datetime-local"
        value={value ? (isInstant ? toWall(value, zone) : value) : ""}
        hint={`Необязательно. Время в вашем поясе: ${zone}. При осеннем переводе часов выбирается первое вхождение времени.`}
        error={error}
        onChange={(event) => {
          if (!event.target.value) {
            onChange("");
            return;
          }
          try {
            onChange(fromWall(event.target.value, zone));
          } catch {
            onChange(event.target.value);
          }
        }}
      />
      <details>
        <summary>Точный формат времени</summary>
        <Input
          label="Дедлайн с часовым поясом"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          hint="Для точного смещения при переводе часов: датаTвремя±часы:минуты."
          error={error}
        />
      </details>
    </div>
  );
}
