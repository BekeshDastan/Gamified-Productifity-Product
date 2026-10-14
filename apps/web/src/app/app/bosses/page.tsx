import { Bosses } from "@/components/bosses";
export default function BossesPage() {
  return (
    <>
      <p className="eyebrow">БОЛЬШАЯ ЦЕЛЬ</p>
      <h1 className="page-title">Боссы по целям</h1>
      <p className="page-intro">
        Большая цель в игровом виде. Выполняйте связанные обязательные задачи, чтобы продвинуться.
      </p>
      <Bosses />
    </>
  );
}
