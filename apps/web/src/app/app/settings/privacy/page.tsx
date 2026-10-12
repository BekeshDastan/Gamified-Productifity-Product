import { PrivacySettings } from "@/components/privacy-settings";
export default function PrivacyPage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Приватность</h1>
      <p className="page-intro">Новый аккаунт закрыт по умолчанию.</p>
      <PrivacySettings />
    </>
  );
}
