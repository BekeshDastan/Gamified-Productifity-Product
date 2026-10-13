import { Suspense } from "react";
import { Focus } from "@/components/focus";
export default function FocusPage() {
  return (
    <>
      <p className="eyebrow">ОДИН ШАГ</p>
      <h1 className="page-title">Концентрация</h1>
      <p className="page-intro">Свой ритм, честное время и право на паузу.</p>
      <Suspense fallback={<p>Загружаем концентрацию…</p>}>
        <Focus />
      </Suspense>
    </>
  );
}
