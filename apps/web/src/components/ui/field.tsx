"use client";
import { useId, type ComponentProps, type ReactNode } from "react";
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
