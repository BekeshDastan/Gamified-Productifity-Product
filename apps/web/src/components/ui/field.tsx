"use client";
import { useId, useState, type ComponentProps, type ReactNode } from "react";
import { Eye, EyeOff } from "lucide-react";
import { cn } from "@/lib/utils";

type FieldProps = { label: string; hint?: string; error?: string };
function Field({
  id,
  label,
  hint,
  error,
  children,
}: FieldProps & { id: string; children: ReactNode }) {
  return (
    <div className="ui-field">
      <label htmlFor={id}>{label}</label>
      {children}
      {hint && (
        <p id={`${id}-hint`} className="ui-hint">
          {hint}
        </p>
      )}
      {error && (
        <p id={`${id}-error`} className="ui-error" role="alert">
          ! {error}
        </p>
      )}
    </div>
  );
}
export function Input({
  id: providedId,
  label,
  hint,
  error,
  className,
  ...props
}: ComponentProps<"input"> & FieldProps) {
  const generatedId = useId();
  const id = providedId ?? generatedId;
  const description =
    [props["aria-describedby"], hint && `${id}-hint`, error && `${id}-error`]
      .filter(Boolean)
      .join(" ") || undefined;
  return (
    <Field {...{ id, label, hint, error }}>
      <input
        {...props}
        id={id}
        aria-invalid={!!error || undefined}
        aria-describedby={description}
        className={cn("ui-input", className)}
      />
    </Field>
  );
}
export function Select({
  id: providedId,
  label,
  hint,
  error,
  className,
  children,
  ...props
}: ComponentProps<"select"> & FieldProps) {
  const generatedId = useId();
  const id = providedId ?? generatedId;
  return (
    <Field {...{ id, label, hint, error }}>
      <select
        {...props}
        id={id}
        aria-invalid={!!error || undefined}
        aria-describedby={
          [props["aria-describedby"], hint && `${id}-hint`, error && `${id}-error`]
            .filter(Boolean)
            .join(" ") || undefined
        }
        className={cn("ui-input", className)}
      >
        {children}
      </select>
    </Field>
  );
}
// Native date input keeps keyboard/calendar support without timezone conversion of date-only values.
export function DatePicker(props: Omit<ComponentProps<typeof Input>, "type">) {
  return <Input {...props} type="date" />;
}

export function PasswordInput({
  id: providedId,
  label,
  disabled,
  ...props
}: Omit<ComponentProps<typeof Input>, "type">) {
  const generatedId = useId();
  const id = providedId ?? generatedId;
  const [visible, setVisible] = useState(false);
  return (
    <div className="password-field">
      <Input
        {...props}
        id={id}
        label={label}
        disabled={disabled}
        type={visible ? "text" : "password"}
      />
      <button
        className="password-reveal"
        type="button"
        disabled={disabled}
        aria-label={`${visible ? "Скрыть" : "Показать"}: ${label}`}
        aria-controls={id}
        aria-pressed={visible}
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff size={17} aria-hidden="true" /> : <Eye size={17} aria-hidden="true" />}
        {visible ? "Скрыть" : "Показать"}
      </button>
    </div>
  );
}
