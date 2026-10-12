import { SecuritySettings } from "@/components/security-settings";
export default function SecurityPage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Устройства и безопасность</h1>
      <p className="page-intro">Управляйте входами и защищайте аккаунт.</p>
      <SecuritySettings />
    </>
  );
}
