import { Goals } from "@/components/goals";
export default function GoalsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ОРИЕНТИР</p>
      <h1 className="page-title">Цели</h1>
      <p className="page-intro">
        Опишите желаемый результат, затем добавьте ближайший шаг. Задачи покажут продвижение к цели.
      </p>
      <Goals />
    </>
  );
}
