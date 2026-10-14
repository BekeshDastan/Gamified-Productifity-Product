import { Suspense } from "react";
import { Focus } from "@/components/focus";
export default function FocusPage() {
  return (
    <>
      <p className="eyebrow">ОДИН ШАГ</p>
      <h1 className="page-title">Фокус</h1>
      <p className="page-intro">
        Выберите задачу, включите таймер и подтвердите результат после работы.
      </p>
      <Suspense fallback={<p>Загружаем концентрацию…</p>}>
        <Focus />
      </Suspense>
    </>
  );
}
