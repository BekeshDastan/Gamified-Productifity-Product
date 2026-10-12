"use client";
import type { ComponentProps, ReactNode } from "react";
export function Form({
  saving,
  error,
  children,
  ...props
}: ComponentProps<"form"> & { saving: boolean; error?: string; children: ReactNode }) {
  return (
    <form {...props} noValidate aria-busy={saving}>
      <fieldset disabled={saving} className="ui-form-fields">
        {children}
      </fieldset>
      {saving && (
        <p role="status" className="ui-hint">
          Сохраняем…
        </p>
      )}
      {error && (
        <p role="alert" className="ui-error">
          ! {error}
        </p>
      )}
    </form>
  );
}
