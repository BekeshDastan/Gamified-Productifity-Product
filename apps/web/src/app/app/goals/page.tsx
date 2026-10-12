import { Goals } from "@/components/goals";
export default function GoalsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ОРИЕНТИР</p>
      <h1 className="page-title">Цели</h1>
      <p className="page-intro">Определите результат и двигайтесь к нему в своём темпе.</p>
      <Goals />
    </>
  );
}
