import { Quests } from "@/components/quests";
export default function QuestsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ СЛЕДУЮЩИЙ ШАГ</p>
      <h1 className="page-title">Задачи</h1>
      <p className="page-intro">
        Запишите, что нужно сделать. Выберите задачу, чтобы выполнить её или выделить время на
        фокус.
      </p>
      <Quests />
    </>
  );
}
