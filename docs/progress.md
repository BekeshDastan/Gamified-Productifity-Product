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

- C019: sidebar/header, mobile Dialog и нижняя навигация; skip-link, aria-current.
  Доступны только обзор, описание и реальное локальное оформление. Будущие модули
  помечены как недоступные и не ведут на фиктивные экраны. Проверены меню и Escape.

- C020: openapi-fetch 0.17.0 и TanStack Query 5.104.2, same-origin cookies,
  request ID, безопасные 401/409/429, Retry-After и отказ при неверном envelope.
  Нет автоматического retry mutations/4xx. Кэш отдельный на account ID,
  старый очищается и отменяется при смене. SSR запрещён для browser client.
  Тесты отвергают чужой origin, HTML/неполный ответ и смешивание аккаунтов.

- C021: RHF 7.90.0 + Zod 4.6.6/resolvers 5.9.1; единая валидация и synchronous
  submit lock до начала проверки. Поля заблокированы при сохранении, ошибки
  сервера сопоставляются только с разрешёнными именами, ввод сохранён при отказе.
  Защищены внутренние ссылки и reload/закрытие; back/forward также защищены в
  браузерах с cancelable Navigation API. В остальных браузерах SPA back/forward
  требует дальнейшей проверки; личные формы появятся вместе с API модулей.

- C022: loading/skeleton, empty, 404, unauthorized и безопасные error boundaries;
  retry использует новый запрос. Конфликт версии не перезаписывает ввод автоматически,
  при отказе reload диалог остаётся открытым. Guide проверяет реальный health/ready
  через same-origin proxy и общий клиент; неготовый/невалидный ответ не считается успехом.

- C023: лендинг с собственным локальным SVG, объяснением будущих функций и
  приватности; ссылки входа/регистрации ведут на честное сообщение о разработке.
  Доступный CTA открывает реальный обзор. Лицензии Lucide/Radix и каталог ресурсов
  сохранены локально и доступны из футера; CDN и внешние изображения не нужны.

- C024: production Playwright/Chromium 1.64.0, 13 сценариев: четыре ширины и обе
  темы с axe WCAG, mobile focus trap/return для двух инициаторов, keyboard,
  persisted/system theme, retry, 404 и reduced motion. Скриншоты осмотрены.
  Добавлена строгая проверка типов тестов, CI запускает E2E; check-changed
  включает весь существующий набор, web build и E2E. Ошибка поля получает фокус
  после разблокировки fieldset. CI этапа проверяется после push.

Этап 03: удалённый CI web/Go/integration success, run 38211823861.
Объединён в main, восемь логических коммитов сохранены.

## Этап 04

- C025: users с нормализованным уникальным email и серверной role по умолчанию;
  регистрация и audit атомарны. Argon2id: 64 MiB, t=3, p=2, случайная соль 16 байт,
  ключ 32 байта; максимум две параллельные операции хеширования на процесс.
  Пароль 12–128 Unicode-символов / до 512 байт, без обрезания пробелов.
  Публичный HTTP endpoint пока не подключён: сначала C026–C027 (сессии, CSRF,
  ограничения попыток). Это серверный шаг; экран регистрации ещё недоступен.
  Проверено: 8 параллельных регистраций дают 1 user и 1 audit; невалидный ввод
  не пишет строки; отказ audit откатывает создание. Хеш исключён из JSON моделей
  sqlc; повреждённые параметры PHC отвергаются до выделения памяти. Go unit,
  integration, vet/Staticcheck, web typecheck и Go build прошли.

- C026: вход проверяет Argon2id, неизвестный email проходит равную по стоимости
  проверку. Серверная сессия 7/30 дней, JWT HS256 на 15 минут с обязательными
  sub/sid/iss/aud/iat/exp/jti; роль и действительность сессии проверяются в БД.
  Refresh — случайные 256 бит, хранится SHA-256. Production cookie __Host-,
  Secure/HttpOnly/SameSite=Lax, локальный HTTP разрешается явно только на loopback.
  Без «Запомнить меня» cookie живут до закрытия браузера, абсолютный срок сессии 7 дней.
  Пароль повторно проверяется под блокировкой пользователя после Argon2id, создание
  сессии, refresh и audit атомарно. HTTP подключается следующим шагом C027.
  Проверены неверные claims/алгоритм/ключ, истечение, отзыв, актуальная роль,
  изоляция двух аккаунтов, SHA-256 в БД и откат при отказе audit. Unit/integration прошли.
  CI C025 завершился успешно: run 38212471251.
