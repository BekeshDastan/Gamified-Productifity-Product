"use client";
import { useSyncExternalStore, type ComponentProps, type ReactNode } from "react";
const subscribeReady = () => () => {};
export function Form({
  saving,
  error,
  children,
  ...props
}: ComponentProps<"form"> & { saving: boolean; error?: string; children: ReactNode }) {
  // Keep server-rendered forms inert until their submit and validation handlers exist.
  const ready = useSyncExternalStore(
    subscribeReady,
    () => true,
    () => false,
  );
  return (
    <form {...props} noValidate aria-busy={saving || !ready}>
      <fieldset disabled={saving || !ready} className="ui-form-fields">
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
