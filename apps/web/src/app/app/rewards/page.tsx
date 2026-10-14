import { Rewards } from "@/components/rewards";
export default function RewardsPage() {
  return (
    <>
      <p className="eyebrow">ВАШИ НАГРАДЫ</p>
      <h1 className="page-title">Магазин и личные награды</h1>
      <p className="page-intro">
        Обменивайте заработанные монеты на оформление и личные награды. Перед покупкой вы увидите её
        стоимость.
      </p>
      <Rewards />
    </>
  );
}
