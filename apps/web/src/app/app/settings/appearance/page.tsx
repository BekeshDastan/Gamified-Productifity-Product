import { ThemeSelect } from "@/components/theme-select";
import { Card } from "@/components/ui/card";
export default function AppearancePage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Ваше оформление</h1>
      <p className="page-intro">
        Выберите комфортную тему. Сейчас выбор сохраняется в этом браузере.
      </p>
      <Card className="settings-card">
        <h2>Цветовая тема</h2>
        <p className="ui-hint">Системная тема следует настройке вашего устройства.</p>
        <ThemeSelect />
        <hr />
        <h2>Уменьшение движения</h2>
        <p className="ui-hint">
          Интерфейс учитывает системную настройку уменьшения движения. Звуков и игровых эффектов на
          этом этапе нет.
        </p>
      </Card>
    </>
  );
}
