# Журнал разработки ASCEND

Требования: ТЗ 1.0 от 12 октября 2026 года. Полный продукт пока не реализован.
Отмечаются только проверенные пункты; эксплуатационные ограничения записываются
отдельно от готовности исходного кода.

## Этап 01

- C001: создана структура монорепозитория, README, MIT LICENSE, правила исключения
  секретов, ADR-001; ТЗ перенесено в `docs` без изменения требований.
- Проверка C001: `.env`, приватные ключи, локальные данные и сборки исключены из Git.

- C002: Next.js 16.4.0 / React 19.3.0 / TypeScript strict; lockfile сохранён.
  Typecheck и production build прошли. Стартовая страница без фиктивных метрик.

- C003: Go 1.27.2, chi 5.3.2; API и worker собираются, go vet проходит.
  Публичных доменных маршрутов пока нет.

- C004: ESLint, Prettier, gofmt, go vet и Staticcheck настроены и проходят.
  Отрицательные пробы any и fmt.Printf отклонены. Go закреплён на 1.26.9
  для совместимости со Staticcheck; глобальные настройки не изменены.

- C005: PostgreSQL 17.11 запущена в Docker; healthcheck зелёный, порт только
  на 127.0.0.1:15432. Контрольная строка сохранилась после restart; проба удалена.

- C006: описаны точные версии и env; конфигурация разделяет dev/test/prod,
  отклоняет пропущенные/неверные значения и dev-пароль в production.
  Все Go-тесты конфигурации и Staticcheck проходят; секреты в ошибках отсутствуют.

- C007: CI web/Go, закреплённые SHA Actions, component/Go/tooling tests,
  проверки lockfiles и секретов. Намеренно падающий тест возвращает ненулевой код.
  Production audit: 0 advisories. У dev-линтера есть advisory, описан в dependencies.
  Результат удалённого CI проверяется после первого push. Защита main на стороне
  GitHub должна требовать успешные Checks; сама конфигурация workflow её не включает.

- C008: единые команды make/npm, check-changed запускает полный существующий
  набор. Lint, typecheck, unit/component/tooling tests и build прошли.
  Общий dev-запуск проверен HTTP 200; Ctrl+C закрыл порты приложений.
  Production standalone отвечает HTTP 200, CSS также 200. Неподключённый E2E
  явно завершился с ошибкой. next-env.d.ts генерируется и не отслеживается.

Этап 01: удалённый CI web/go success, run 38206646556. Проверенный код в main.

## Этап 02

- C009: JSON logging с редактированием секретов, безопасные correlation IDs,
  подменяемый Clock и graceful shutdown с ожиданием запросов. Go tests/vet/staticcheck зелёные.

- C010: health/live и health/ready, pgx pool.Ping, таймауты и лимит тела.
  Тесты подтверждают 503 readiness при отказе БД, живой liveness, 413 и безопасный 500.

- C011: pgx pool, goose forward CLI, служебная миграция audit_events, sqlc 1.31.1.
  Реальная БД: миграции повторяемы, rollback оставляет 0 строк, commit — 1.
  Тестовая схема удалена автоматически; production/основная БД не очищается.

- C012: OpenAPI 3.1, генерация TS types, единые data/meta и error envelope.
  JSON 400 отделён от DTO 422; неизвестные role/owner_id отклоняются, Unicode
  считается символами; пагинация ограничена. Contract/unit tests и typecheck зелёные.

- C013: owner/session context, строгий If-Match, PostgreSQL idempotency scope
  user+method+concrete route+key и SHA-256 тела. 12 параллельных запросов дают
  одно выполнение; чужой пользователь/ресурс не получает чужой ответ; другое тело
  возвращает IDEMPOTENCY_MISMATCH. Unit/integration/vet/staticcheck проходят.

- C014: PostgreSQL jobs/outbox, SKIP LOCKED, fenced leases, retry+jitter,
  Retry-After, dead-letter и worker. Повтор события дал одну job; просроченная
  lease передана новому worker, старый токен отвергнут, эффект записан один раз.
  Ошибки хранят безопасный код; unknown handler не маскируется успехом.
  C013a: дополнительно проверен составной недопустимый HTTP-метод.

- C015: BlobStore/Mailer ports, приватный os.Root store, лимиты/checksum,
  карантин с отказом при недоступном scanner. SMTP требует TLS в production;
  dev plaintext только loopback. Тестовое письмо получено локальным Mailpit 1.31.5.
  Public URL для blobs отсутствует; path traversal, header injection и plaintext
  production отвергнуты тестами.

- C016: случайные схемы PostgreSQL, fixtures владельцев, thread-safe Clock,
  FakeMailer и HTTP clients с отдельными cookie jars. Неверная миграция отвергнута;
  параллельные схемы не смешали данные. Реальный pool: readiness 200 → 503 при
  закрытии; liveness остаётся 200. Полный check-changed и build прошли.
  CI дополнен отдельными PostgreSQL/Mailpit services и race integration job.
  Удалённый результат этапа 02 проверяется после push; F01–F28 ещё не реализованы.

## Этап 03

- C017: Tailwind 4.3.3, общие цвета, типографика, интервалы, радиусы, тени;
  светлая/тёмная/системная темы через next-themes 0.4.6. Контраст основных
  текстов и статусов проверяется расчётом WCAG AA; reduced motion учитывается.
  Сохраняется только тема, токенов в browser storage нет.

- C018: Button/Input/Select/DatePicker/Card/Progress и Radix Dialog/Tabs/Toast/Tooltip.
  Диалог удерживает и возвращает фокус, поля связаны с labels/errors, native date
  хранит дату без сдвига timezone. Неизвестный прогресс не становится 0%.
  Основа shadcn настроена локально; клавиатура, ошибки и отсутствие метрик проверены.
