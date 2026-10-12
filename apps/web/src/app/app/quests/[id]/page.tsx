import { Quests } from "@/components/quests";
export default async function QuestPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <>
      <p className="eyebrow">ЗАДАЧА</p>
      <h1 className="page-title">Детали и план</h1>
      <Quests id={id} />
    </>
  );
}
