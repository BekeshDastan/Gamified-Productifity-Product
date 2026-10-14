import { Seasons } from "@/components/seasons";
export default function SeasonsPage() {
  return (
    <>
      <p className="eyebrow">СВОЙ ТЕМП</p>
      <h1 className="page-title">Сезоны и возвращение</h1>
      <p className="page-intro">
        Выберите результат на ближайший период. Продолжайте с посильного шага, сохраняя историю
        своего пути.
      </p>
      <Seasons />
    </>
  );
}
