import { Achievements } from "@/components/achievements";
export default function AchievementsPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ПУТЬ</p>
      <h1 className="page-title">Достижения</h1>
      <p className="page-intro">Понятные условия, ваши результаты и история их исправлений.</p>
      <Achievements />
    </>
  );
}
