import { EveningReview } from "@/components/evening-review";
import { MorningRoutine } from "@/components/morning-routine";
import { Journal } from "@/components/journal";
export default function JournalPage() {
  return (
    <>
      <p className="eyebrow">СВОИ МЫСЛИ</p>
      <h1 className="page-title">Дневник и ритуалы</h1>
      <p className="page-intro">Замечать важное, разбирать день и выбирать следующий шаг.</p>
      <Journal />
      <MorningRoutine />
      <EveningReview />
    </>
  );
}
