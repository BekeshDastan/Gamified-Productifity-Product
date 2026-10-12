import { Quests } from "@/components/quests";
export default function QuestsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ СЛЕДУЮЩИЙ ШАГ</p>
      <h1 className="page-title">Задачи</h1>
      <p className="page-intro">Соберите идеи, выберите важное и двигайтесь в своём ритме.</p>
      <Quests />
    </>
  );
}
