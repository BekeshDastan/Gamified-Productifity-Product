import { Suspense } from "react";
import { Challenges } from "@/components/challenges";
export default function ChallengesPage() {
  return (
    <>
      <p className="eyebrow">СВОЙ ВЫЗОВ</p>
      <h1 className="page-title">Испытания и квесты</h1>
      <p className="page-intro">
        Понятные правила, посильный результат и право изменить направление.
      </p>
      <Suspense fallback={<p>Загружаем испытания…</p>}>
        <Challenges />
      </Suspense>
    </>
  );
}
