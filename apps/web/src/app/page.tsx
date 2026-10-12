import Image from "next/image";
import { ArrowRight, ArrowUpRight, Compass, Footprints, LockKeyhole, Sprout } from "lucide-react";
import { SafeLink as Link } from "@/lib/forms/unsaved-changes";
import { ThemeSelect } from "@/components/theme-select";
import { Button } from "@/components/ui/button";

export default function Home() {
  return (
    <div className="landing">
      <a href="#main-content" className="skip-link">
        К содержимому
      </a>
      <header className="landing-header">
        <Link className="wordmark" href="/" aria-label="ASCEND, главная">
          <ArrowUpRight aria-hidden="true" />
          ASCEND
        </Link>
        <nav className="landing-links" aria-label="О продукте">
          <Link href="#how-it-works">Подход</Link>
          <Link href="#privacy">Приватность</Link>
        </nav>
        <div className="landing-actions">
          <ThemeSelect />
          <Button asChild variant="ghost">
            <Link href="/auth/login">Войти</Link>
          </Button>
        </div>
      </header>
      <main id="main-content">
        <section className="hero" aria-labelledby="intro-title">
          <div className="hero-copy">
            <p className="release-label">
              <span aria-hidden="true">↗</span> ПУТЬ К ВАШЕЙ ЛУЧШЕЙ ВЕРСИИ
            </p>
            <h1 id="intro-title">
              Большие перемены.
              <br />
              <span>
                Маленькие
                <br className="desktop-break" /> действия.
              </span>
            </h1>
            <p className="description">
              Место для ваших целей, привычек и ежедневного прогресса. Без штрафов за отдых. В вашем
              ритме.
            </p>
            <div className="hero-actions">
              <Button asChild>
                <Link href="/app">
                  Открыть пространство <ArrowRight size={17} aria-hidden="true" />
                </Link>
              </Button>
              <Link href="#how-it-works" className="text-link">
                Познакомиться с ASCEND <ArrowUpRight size={16} aria-hidden="true" />
              </Link>
            </div>
            <p className="availability" role="status">
              <span aria-hidden="true">◷</span> Доступны аккаунт, обзор и оформление. Задачи и
              персональные сводки появятся на следующих этапах.
            </p>
          </div>
          <div className="hero-art">
            <Image src="/ascend-path.svg" width={560} height={560} alt="" priority />
            <div className="art-caption">
              <span>01 — НАЧНИТЕ С МАЛОГО</span>
              <p>
                Ваш следующий шаг
                <br />
                важнее идеального плана.
              </p>
            </div>
          </div>
        </section>
        <section id="how-it-works" className="approach" aria-labelledby="approach-title">
          <div className="section-heading">
            <p className="eyebrow">ПРОГРЕСС, КОТОРЫЙ ИМЕЕТ ЗНАЧЕНИЕ</p>
            <h2 id="approach-title">Не гонка. Ваш путь.</h2>
            <p className="ui-hint">
              Мы строим пространство, в котором планирование помогает жить,
              <br className="desktop-break" /> а игровые элементы поддерживают реальные действия.
            </p>
          </div>
          <div className="approach-grid">
            <article>
              <span className="feature-icon">
                <Compass size={24} aria-hidden="true" />
              </span>
              <p className="feature-number">01 / НАПРАВЛЕНИЕ</p>
              <h3>Знайте, куда идёте</h3>
              <p className="ui-hint">
                Цели станут понятными шагами, а задачи и привычки — частью одного плана.
              </p>
            </article>
            <article>
              <span className="feature-icon">
                <Footprints size={24} aria-hidden="true" />
              </span>
              <p className="feature-number">02 / ДЕЙСТВИЕ</p>
              <h3>Замечайте небольшие победы</h3>
              <p className="ui-hint">
                Прогресс будет опираться на выполненные действия и прозрачную историю.
              </p>
            </article>
            <article>
              <span className="feature-icon">
                <Sprout size={24} aria-hidden="true" />
              </span>
              <p className="feature-number">03 / БАЛАНС</p>
              <h3>Оставляйте место отдыху</h3>
              <p className="ui-hint">
                Пропуски не отнимут заработанный опыт. Вернуться можно будет с небольшого шага.
              </p>
            </article>
          </div>
        </section>
        <section id="privacy" className="privacy-strip" aria-labelledby="privacy-title">
          <div className="privacy-heading">
            <LockKeyhole size={26} aria-hidden="true" />
            <h2 id="privacy-title">Личное остаётся личным.</h2>
          </div>
          <p>
            Личные записи будут закрыты по умолчанию. Подключение интеграций и совместные функции —
            только по вашему выбору.
          </p>
          <Link href="/app/guide" className="text-link">
            Подробнее о принципах <ArrowUpRight size={16} aria-hidden="true" />
          </Link>
        </section>
      </main>
      <footer className="landing-footer">
        <div>
          <span className="footer-brand">ASCEND</span>
          <p>Реальные действия. Честный прогресс.</p>
        </div>
        <nav aria-label="Дополнительные ссылки">
          <Link href="/auth/register">Регистрация</Link>
          <Link href="/app/guide">О приложении</Link>
          <a href="/licenses/README.txt">Лицензии ресурсов</a>
        </nav>
        <span className="copyright">© 2026 ASCEND</span>
      </footer>
    </div>
  );
}
