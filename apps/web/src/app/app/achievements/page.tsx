import { Achievements } from "@/components/achievements";
export default function AchievementsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ПУТЬ</p>
      <h1 className="page-title">Достижения</h1>
      <p className="page-intro">
        Посмотрите условия достижений. Они открываются по вашим реальным действиям.
      </p>
      <Achievements />
    </>
  );
}
