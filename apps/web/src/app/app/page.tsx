import { AccountPanel } from "@/components/account-panel";
import { Dashboard } from "@/components/dashboard";
export default function TodayPage() {
  return (
    <>
      <p className="eyebrow">НОВЫЙ ДЕНЬ · НОВЫЙ ШАГ</p>
      <h1 className="page-title">
        Ваш день.
        <br />
        <span>В вашем ритме.</span>
      </h1>
      <p className="page-intro">Один небольшой шаг — уже движение вперёд.</p>
      <AccountPanel />
      <Dashboard />
    </>
  );
}
