import { NavigatorCards } from "@/components/navigator";
export default function NavigatorPage() {
  return (
    <>
      <h1 className="page-title">Навигатор</h1>
      <p className="page-intro">Понятные предложения по вашим записям. Решение остаётся за вами.</p>
      <NavigatorCards />
    </>
  );
}
