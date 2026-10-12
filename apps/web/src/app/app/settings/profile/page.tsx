import { ProfileEditor } from "@/components/profile-editor";
export default function ProfilePage() {
  return (
    <>
      <p className="eyebrow">НАСТРОЙКИ</p>
      <h1 className="page-title">Ваш профиль</h1>
      <p className="page-intro">Имя и аватар для вашего пространства. Профиль виден только вам.</p>
      <ProfileEditor />
    </>
  );
}
