# Зависимости и воспроизводимость

Выбор проверен 12 октября 2026 года по официальной документации и registry
пакетов. Версии верхнего уровня точные; транзитивные версии — в `package-lock.json`
и `go.sum`. Установка web только через `npm ci` после первого `npm install`.

| Компонент                   | Версия           | Причина                                               |
| --------------------------- | ---------------- | ----------------------------------------------------- |
| Node.js                     | 22.22.0          | Установленное совместимое LTS-окружение               |
| npm                         | 10.9.4           | Lockfile v3 и npm workspaces                          |
| Next.js                     | 16.4.0           | App Router, официальная стабильная версия             |
| React / React DOM           | 19.3.0           | Совместимые peer dependencies Next.js                 |
| TypeScript                  | 5.9.3            | Проверенный strict toolchain, минимум Next.js — 5.1   |
| Go                          | 1.26.9           | Поддерживаемая ветка, совместима со Staticcheck 0.8.1 |
| chi                         | 5.3.2            | net/http-совместимый роутер                           |
| PostgreSQL                  | 17.11-bookworm   | Поддерживаемая major-версия; фиксированный образ      |
| ESLint / eslint-config-next | 10.12.0 / 16.4.0 | Статический анализ React и TypeScript                 |
| Prettier                    | 3.9.10           | Форматирование исходных файлов                        |
| Staticcheck                 | 0.8.1            | Дополнительный анализ Go                              |

Go 1.27.2 проверялся при bootstrap, но текущий Staticcheck не читает его новый
export format. До обновления линтера используется поддерживаемый Go 1.26.9.
Переменная `GOTOOLCHAIN` в командах проверок фиксирует эту версию независимо от
установленного системного Go. Глобальные настройки Go/Node не изменяются.

Серверная платформа C010–C011: pgx 5.11.0, goose 3.29.0, sqlc 1.31.1.
Версии подтверждены официальным Go module proxy; sqlc запускается из
`sqlc/sqlc:1.31.1`, generated Go-код хранится в репозитории.

Тестовые инструменты: Vitest 5.0.3, Testing Library React 16.3.3,
jest-dom 6.9.1, jsdom 26.1.0 (совместим с установленным Node 22.22.0).
OpenAPI types: openapi-typescript 7.13.0. Локальная почта: Mailpit 1.31.5.

## Политика обновления

Не использовать `latest` в сборках и образах. Обновления — отдельным коммитом
после typecheck, lint, unit/integration и build. Проверять advisories и лицензии.
Автоматический `audit fix --force` запрещён: совместимость проверяется явно.
Инструменты тестирования и доменные зависимости добавляются по мере реализации,
с точными версиями. Секреты и приватные файлы не являются зависимостями.

## Источники

- [Next.js: установка и системные требования](https://nextjs.org/docs/app/getting-started/installation)
- [Go: поддерживаемые загрузки](https://go.dev/dl/)
- [PostgreSQL: сроки поддержки](https://www.postgresql.org/support/versioning/)
- [Staticcheck: релизы](https://github.com/dominikh/go-tools/releases)

## Конфигурация

`APP_ENV` принимает `development`, `test`, `production`. `APP_ORIGIN`,
`DATABASE_URL` обязательны; `API_PORT` по умолчанию 8080. Production требует HTTPS
origin и отвергает известный dev-пароль. Ошибки содержат имя настройки, без её
значения. Реальные session/OAuth/SMTP/S3 ключи добавляются только вместе с
адаптером, которому они нужны; в публичный frontend они не передаются.

## Открытые advisories инструментов разработки

На 2026-10-12 полный `npm audit` сообщает 5 связанных записей из одной цепочки:
`eslint-config-next → @next/eslint-plugin-next → fast-glob → micromatch → braces`.
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm) описывает
истощение стека на глубоко вложенных glob-шаблонах. Исправленная версия braces
пока не опубликована (registry: 3.0.3). Это dev-зависимости, пользовательский ввод
к glob-шаблонам линтера не поступает. Production audit выполняется отдельно.
Переход на старый несовместимый Next.js ради предложенного audit downgrade не
выполняется. До публичного выпуска повторно проверить advisory и исправление.

## Этап 03: оформление

На 12.10.2026 npm registry: Tailwind CSS и PostCSS plugin 4.3.3,
next-themes 0.4.6. Версии закреплены в lockfile. Системный шрифт не требует
загрузки или внешнего CDN.

Источники: [Tailwind + Next.js](https://tailwindcss.com/docs/installation/framework-guides/nextjs),
[next-themes](https://github.com/pacocoursey/next-themes),
[shadcn manual](https://ui.shadcn.com/docs/installation/manual).
