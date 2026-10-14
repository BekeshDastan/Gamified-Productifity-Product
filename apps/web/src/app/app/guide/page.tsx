import { Card } from "@/components/ui/card";
import { ServiceStatus } from "@/components/service-status";
export default function GuidePage() {
  return (
    <>
      <p className="eyebrow">О ПРОДУКТЕ</p>
      <h1 className="page-title">
        Маленькие действия.
        <br />
        <span>Осмысленный прогресс.</span>
      </h1>
      <p className="page-intro">
        ASCEND объединяет планирование и мягкую игровую мотивацию. Личные задачи, привычки,
        календарь, фокус, отчёты и уведомления уже доступны.
      </p>
      <div className="principle-grid">
        <Card>
          <h2>Действия прежде цифр</h2>
          <p className="ui-hint">
            Задачи, привычки и время фокуса служат источниками сводки. У каждой награды есть
            проверяемая история.
          </p>
        </Card>
        <Card>
          <h2>Цели в вашем ритме</h2>
          <p className="ui-hint">
            План можно пересмотреть, а после перерыва — вернуться с небольшого шага. Без потери
            накопленного опыта за отдых.
          </p>
        </Card>
        <Card>
          <h2>Понятные правила</h2>
          <p className="ui-hint">
            Подсказки объясняются конкретным правилом и исходными данными. Скрытых оценок личности
            нет.
          </p>
        </Card>
        <Card>
          <h2>Вы управляете доступом</h2>
          <p className="ui-hint">
            Email и push включаются только по вашему выбору. Личные тексты приватны; совместные
            функции и интеграции находятся в разработке.
          </p>
        </Card>
      </div>
      <Card className="connection-card">
        <h2>Доступность сервиса</h2>
        <ServiceStatus />
      </Card>
    </>
  );
}
