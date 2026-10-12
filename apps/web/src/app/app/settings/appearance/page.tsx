import { AppearanceSettings } from "@/components/personalization";
export default function AppearancePage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Ваше оформление</h1>
      <p className="page-intro">Тема, движение и ваш ритм — с сохранением в аккаунте.</p>
      <AppearanceSettings />
    </>
  );
}
