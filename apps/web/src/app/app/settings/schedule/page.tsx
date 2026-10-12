import { ScheduleEditor } from "@/components/schedule-editor";
export default function SchedulePage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Часовой пояс и расписание</h1>
      <p className="page-intro">Выберите привычные даты и время, оставьте место для отдыха.</p>
      <ScheduleEditor />
    </>
  );
}
