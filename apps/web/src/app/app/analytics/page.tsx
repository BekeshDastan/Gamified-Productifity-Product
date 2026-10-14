import { Analytics } from "@/components/analytics";
export default function AnalyticsPage() {
  return (
    <>
      <p className="eyebrow">ЗАМЕЧАТЬ ДВИЖЕНИЕ</p>
      <h1 className="page-title">Аналитика</h1>
      <p className="page-intro">
        Ваши действия, время и собственные планы. Показатели помогают увидеть ритм и выбрать
        посильное продолжение.
      </p>
      <Analytics />
    </>
  );
}
