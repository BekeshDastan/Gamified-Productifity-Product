"use client";
import Link from "next/link";
import {
  createContext,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  type ComponentProps,
  type ReactNode,
} from "react";

type Guard = { setDirty: (id: string, dirty: boolean) => void; isBlocked: () => boolean };
const Context = createContext<Guard | null>(null);
const message = "Изменения ещё не сохранены. Покинуть страницу и потерять их?";

export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const dirtyForms = useRef(new Set<string>());
  const guard = useMemo<Guard>(
    () => ({
      setDirty(id, dirty) {
        if (dirty) dirtyForms.current.add(id);
        else dirtyForms.current.delete(id);
      },
      isBlocked: () => dirtyForms.current.size > 0,
    }),
    [],
  );
  useEffect(() => {
    function beforeUnload(event: BeforeUnloadEvent) {
      if (guard.isBlocked()) {
        event.preventDefault();
        event.returnValue = "";
      }
    }
    // The Navigation API can cancel browser back/forward before React unmounts a form.
    const navigation = (window as Window & { navigation?: EventTarget }).navigation;
    function navigate(event: Event) {
      if (
        (event as Event & { navigationType?: string }).navigationType === "traverse" &&
        event.cancelable &&
        guard.isBlocked() &&
        !window.confirm(message)
      )
        event.preventDefault();
    }
    window.addEventListener("beforeunload", beforeUnload);
    navigation?.addEventListener("navigate", navigate);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      navigation?.removeEventListener("navigate", navigate);
    };
  }, [guard]);
  return <Context.Provider value={guard}>{children}</Context.Provider>;
}
export function useUnsavedChanges(dirty: boolean) {
  const guard = useContext(Context);
  const id = useId();
  if (!guard) throw new Error("UnsavedChangesProvider is required for forms");
  useEffect(() => {
    guard.setDirty(id, dirty);
    return () => guard.setDirty(id, false);
  }, [guard, id, dirty]);
}
export function SafeLink({ onNavigate, ...props }: ComponentProps<typeof Link>) {
  const guard = useContext(Context);
  return (
    <Link
      {...props}
      onNavigate={(event) => {
        if (guard?.isBlocked() && !window.confirm(message)) {
          event.preventDefault();
          return;
        }
        onNavigate?.(event);
      }}
    />
  );
}
