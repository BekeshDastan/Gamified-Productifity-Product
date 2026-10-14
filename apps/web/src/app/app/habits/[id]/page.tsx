import { HabitSource } from "@/components/habit-source";
export default async function HabitSourcePage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ period_id?: string }>;
}) {
  const { id } = await params,
    { period_id } = await searchParams;
  return (
    <>
      <h1 className="page-title">Источник привычки</h1>
      <HabitSource id={id} periodID={period_id} />
    </>
  );
}
