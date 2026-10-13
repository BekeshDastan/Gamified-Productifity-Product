import { Character } from "@/components/character";
export default function CharacterPage() {
  return (
    <>
      <p className="eyebrow">ВАШ ПУТЬ</p>
      <h1 className="page-title">Персонаж и навыки</h1>
      <p className="page-intro">Ваш образ и вклад в выбранные направления.</p>
      <Character />
    </>
  );
}
