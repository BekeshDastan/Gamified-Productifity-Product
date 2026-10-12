import { Calendar } from "@/components/calendar";
export default function CalendarPage() {
  return (
    <>
      <p className="eyebrow">ВРЕМЯ ДЛЯ ВАЖНОГО</p>
      <h1 className="page-title">Календарь</h1>
      <p className="page-intro">Планы задач и блоки работы в одном месте.</p>
      <Calendar />
    </>
  );
}
