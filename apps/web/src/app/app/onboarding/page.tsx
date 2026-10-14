import { Onboarding } from "@/components/onboarding";
export default function OnboardingPage() {
  return (
    <>
      <p className="eyebrow">ПЕРВЫЕ ШАГИ</p>
      <h1 className="page-title">Настроим ваш ASCEND</h1>
      <p className="page-intro">
        Добавьте первое дело на сегодня. Настройки можно оставить на потом.
      </p>
      <Onboarding />
    </>
  );
}
