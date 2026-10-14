"use client";
import { useSyncExternalStore } from "react";
import { useTheme } from "next-themes";
const subscribe = () => () => {};

export function ThemeSelect() {
  const { theme, setTheme } = useTheme();
  const mounted = useSyncExternalStore(
    subscribe,
    () => true,
    () => false,
  );
  return (
    <label className="theme-select">
      <span className="sr-only">Тема оформления</span>
      <select
        aria-label="Тема оформления"
        disabled={!mounted}
        value={mounted ? theme : "system"}
        onChange={(event) => setTheme(event.target.value)}
      >
        <option value="system">Системная тема</option>
        <option value="dark">Тёмная тема</option>
        <option value="light">Светлая тема</option>
      </select>
    </label>
  );
}
