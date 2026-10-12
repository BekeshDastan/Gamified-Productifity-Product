"use client";
import dynamic from "next/dynamic";
const CalendarWorkspace = dynamic(() => import("./calendar-workspace"), {
  ssr: false,
  loading: () => <p role="status">Загружаем календарь…</p>,
});
export function Calendar() {
  return <CalendarWorkspace />;
}
