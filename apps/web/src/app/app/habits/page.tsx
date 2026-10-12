import { Habits } from "@/components/habits";
export default function HabitsPage() {
  return (
    <>
      <p className="eyebrow">МАЛЕНЬКИЕ ШАГИ</p>
      <h1 className="page-title">Привычки</h1>
      <p className="page-intro">Посильная норма, честная история и место для отдыха.</p>
      <Habits />
    </>
  );
}
