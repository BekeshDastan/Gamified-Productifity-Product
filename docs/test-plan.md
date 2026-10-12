# Проверки текущей реализации

Реализованы C001–C024. Остальные пункты ТЗ ещё не считаются готовыми.

## Локальные наборы

- `npm run lint`: проверка секретов/генерируемых файлов, Prettier, ESLint,
  TypeScript, gofmt, vet, Staticcheck.
- `npm test`: проверки компонентов, контраста, keyboard/focus, форм и API-клиента, policy guard,
  конфигурация, HTTP envelope/лимиты/ошибки, доступ владельца/версии, privacy logs,
  shutdown, приватные blobs, SMTP security, тестовые HTTP clients.
- `npm run test:integration`: реальные транзакции PostgreSQL, повторяемые
  миграции, конкурентная идемпотентность, leases/retry/dead-letter/outbox,
  изоляция схем, отказ ошибочной миграции, локальный Mailpit.
- `npm run check-changed`: весь указанный выше набор; не пропускает существующие
  integration tests. Требует настроенного локального стенда.
- `npm run build`: production Next.js и Go API/worker.
- `npm run test:e2e`: production Chromium, 13 сценариев оболочки: 360/768/1280/1440 px
  в обеих темах, axe WCAG, keyboard, оба инициатора мобильного меню, system theme,
  persistence, API retry, 404/auth availability и reduced motion. UI fixtures
  не считаются проверкой реального API; Go integration проверяет БД отдельно.
- `check-changed` также собирает web и запускает E2E. Установить Chromium заранее.

## Что уже проверено вручную

Стартовая страница dev/production и CSS возвращают HTTP 200. Общий dev-запуск
поднимает PostgreSQL, mail receiver, применяет forward migrations, запускает
API/worker/web. Остановка приложения сохраняет volume базы. Контрольная запись
пережила перезапуск PostgreSQL. Mailer доставил тестовое письмо в локальный
приёмник. Секретные значения не выводятся в диагностике приложения.

## Что ещё впереди

Полная F01-auth, профиль, доменные пользовательские E2E, нагрузки,
restore rehearsal и live smoke внешних провайдеров ещё не реализованы.
Эти проверки нельзя заменить текущими тестами инфраструктуры. Release-теги
v0.1/v1.0 пока не создаются.
