import { AreasEditor } from "@/components/areas-editor";
export default function AreasPage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Направления и теги</h1>
      <p className="page-intro">Выберите важное для вас. Названия, цвета и значки можно менять.</p>
      <AreasEditor />
    </>
  );
}
