import Link from "next/link";
import { ArrowUpRight, LockKeyhole, Sprout } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
export default function TodayPage() {
  return (
    <>
      <p className="eyebrow">НОВЫЙ ДЕНЬ · НОВЫЙ ШАГ</p>
      <h1 className="page-title">
        Место для вашего
        <br />
        <span>следующего шага.</span>
      </h1>
      <p className="page-intro">
        Начните с малого. Двигайтесь в своём ритме.
        <br />
        Отдых — тоже часть пути.
      </p>
      <Card className="welcome-card">
        <div className="welcome-symbol">
          <Sprout size={32} aria-hidden="true" />
        </div>
        <div>
          <h2>Ваше пространство готовится</h2>
          <p className="ui-hint">
            Сейчас доступны оформление и описание продукта. Регистрация, задачи и персональная
            сводка появятся на следующих этапах.
          </p>
          <Button asChild variant="secondary">
            <Link href="/app/guide">
              Как устроен ASCEND <ArrowUpRight size={16} aria-hidden="true" />
            </Link>
          </Button>
        </div>
      </Card>
      <div className="principle-grid">
        <Card>
          <LockKeyhole size={22} aria-hidden="true" />
          <h3>Только ваши данные</h3>
          <p className="ui-hint">
            Личные записи будут закрыты по умолчанию. Делиться ими можно будет по вашему решению.
          </p>
        </Card>
        <Card>
          <Sprout size={22} aria-hidden="true" />
          <h3>Без штрафов за отдых</h3>
          <p className="ui-hint">
            Прогресс складывается из реальных действий. Пропуски не создают долг, который нужно
            отрабатывать.
          </p>
        </Card>
      </div>
    </>
  );
}
