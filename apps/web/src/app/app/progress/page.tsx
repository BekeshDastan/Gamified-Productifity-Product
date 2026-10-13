import { Progression } from "@/components/progression";
export default function ProgressPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ПУТЬ</p>
      <h1 className="page-title">Опыт и награды</h1>
      <p className="page-intro">
        Каждое начисление связано с конкретным выполнением. Исправления сохраняются в истории.
      </p>
      <Progression />
    </>
  );
}
