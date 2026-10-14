import { NavigatorCards } from "@/components/navigator";
import { SmartSchedule } from "@/components/smart-schedule";
import { GoalWizard } from "@/components/goal-wizard";
import { FutureSelf } from "@/components/future-self";
export default function NavigatorPage() {
  return (
    <>
      <h1 className="page-title">Навигатор</h1>
      <p className="page-intro">Понятные предложения по вашим записям. Решение остаётся за вами.</p>
      <NavigatorCards />
      <SmartSchedule />
      <GoalWizard />
      <FutureSelf />
    </>
  );
}
