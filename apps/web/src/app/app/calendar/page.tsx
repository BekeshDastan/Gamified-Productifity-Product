import { Calendar } from "@/components/calendar";
export default function CalendarPage() {
  return (
    <>
      <p className="eyebrow">ВРЕМЯ ДЛЯ ВАЖНОГО</p>
      <h1 className="page-title">Календарь</h1>
      <p className="page-intro">
        Выделите время для дел: выберите день и создайте событие или рабочий блок.
      </p>
      <Calendar />
    </>
  );
}
