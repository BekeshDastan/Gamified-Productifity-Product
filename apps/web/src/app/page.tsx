export default function Home() {
  return (
    <main className="foundation">
      <a className="wordmark" href="/" aria-label="ASCEND, главная">
        <span aria-hidden="true">↗</span> ASCEND
      </a>
      <section aria-labelledby="intro-title">
        <p className="eyebrow">LEVEL UP YOUR LIFE</p>
        <h1 id="intro-title">Большие перемены.<br /><span>Маленькие действия.</span></h1>
        <p className="description">
          Место для ваших целей, привычек и ежедневного прогресса.
          Без штрафов за отдых. В вашем ритме.
        </p>
        <div className="notice" role="status">
          <span className="status-dot" aria-hidden="true" />
          Основа приложения запущена. Регистрация и личный кабинет пока в разработке.
        </div>
      </section>
      <footer>Реальные действия. Честный прогресс. Ваши данные под вашим контролем.</footer>
    </main>
  );
}
