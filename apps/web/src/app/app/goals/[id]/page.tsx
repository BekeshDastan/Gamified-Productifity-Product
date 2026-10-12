import { Goals } from "@/components/goals";
export default async function GoalPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <>
      <p className="eyebrow">ЦЕЛЬ</p>
      <h1 className="page-title">Результат и план</h1>
      <Goals key={id} id={id} />
    </>
  );
}
