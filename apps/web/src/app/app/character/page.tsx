import { Character } from "@/components/character";
export default function CharacterPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ПУТЬ</p>
      <h1 className="page-title">Персонаж и навыки</h1>
      <p className="page-intro">
        Выберите образ персонажа. Навыки растут благодаря выполненным делам в ваших направлениях.
      </p>
      <Character />
    </>
  );
}
