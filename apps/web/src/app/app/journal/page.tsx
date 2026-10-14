import { JournalWorkspace } from "@/components/journal-workspace";
export default async function JournalPage({
  searchParams,
}: {
  searchParams: Promise<{ section?: string; review_date?: string }>;
}) {
  const params = await searchParams;
  return (
    <>
      <p className="eyebrow">СВОИ МЫСЛИ</p>
      <h1 className="page-title">Дневник и ритуалы</h1>
      <p className="page-intro">
        Запишите наблюдение или выберите утренний, вечерний или недельный обзор.
      </p>
      <JournalWorkspace initialSection={params.section} reviewDate={params.review_date} />
    </>
  );
}
