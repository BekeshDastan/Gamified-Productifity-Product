# Запуск текущего функционала

Объём: этапы 1–21. По решению владельца этапы 22–24 отложены:
Google/GitHub/ICS, общий поиск, перенос/удаление аккаунта и расширенная
административная панель не входят в этот выпуск. Этап 25 проверяется для этого
сокращённого объёма. Локальная приёмка не означает публикацию сервиса.

## Локальный production-стенд

Требуются Docker Compose, Node 22.22, npm ci и Chromium Playwright.
Команды выполнять из корня. Рабочая dev-база и `.env` не используются.

```sh
node scripts/prepare-release-local.mjs
node scripts/validate-release-env.mjs .data/release-local/release.env --local
docker compose --env-file .data/release-local/release.env -f deploy/compose.yaml -p ascend-release-local up -d --build
node scripts/release-smoke.mjs
```

Первый шаг создаёт ключи только один раз: повтор намеренно не перезаписывает их.
Адрес https://localhost:3443. Caddy использует отдельный локальный CA;
smoke проверяет сертификат через его CA без изменения системного доверия.
Браузерное исключение сертификата ограничено тестовым контекстом.
Снимок HTTPS-интерфейса: `.data/release-local/https-quests.png`.
При первом запуске дождаться загрузки сигнатур scanner; до этого вложения
остаются в quarantine, выдача недоступна. SMTP на этом стенде не настроен:
внешние письма не отправляются. Полный email-flow проверяется отдельным
`npm run test:auth-e2e` с настоящим локальным Mailpit.

## Будущий сервер

1. Подготовить Linux-сервер, DNS и входящие 80/443. Скопировать env.example
   из deploy в приватный файл вне Git, права 0600. Создать новые случайные
   ключи/пароль; не переносить локальные тестовые секреты.
2. Заполнить реальный SMTP с STARTTLS, домен, backup и monitoring настройки.
   Секреты backup хранить в отдельном BACKUP_ENV_FILE. Проверить конфигурацию
   `node scripts/validate-release-env.mjs /srv/ascend/.env.production`.
3. Использовать проверенный SHA как RELEASE_TAG. Собрать образы из этого SHA
   либо перенести проверенные образы; не использовать плавающий latest.
4. `docker compose --env-file /srv/ascend/.env.production -f deploy/compose.yaml
-p ascend-release up -d --build`. Сначала здоровая БД, затем forward migration
   job, API/worker, web и edge. Dev seed в образах отсутствует.
5. Подключить monitoring overlay. Проверить регистрацию, подтверждение почты,
   reset, сохранение/перезагрузку, файл, backup/restore и доставку сигналов
   оператору. Только после этого открывать публичную регистрацию.

API/worker/web/edge работают без root и capabilities; корневая ФС read-only.
PostgreSQL запускает сервер от postgres, штатный entrypoint инициализирует volume.
ClamAV использует штатный entrypoint для сигнатур и принимает запросы в сетевом
пространстве API через loopback. Порты БД/API/scanner не публикуются. Единственный
доверенный proxy peer — 172.30.25.10/32; edge перезаписывает X-Real-IP.
Если подсеть занята, поменять сеть и доверенный адрес согласованно.
Перед внешним CDN сначала настроить его доверенные сети, не принимать произвольный XFF.

TLS/HSTS обеспечивает Caddy; настройки CSP с nonce — Next.js.
Скрипты не разрешают unsafe-inline/unsafe-eval в production; inline CSS остаётся
для календаря, графиков и позиционирования диалогов. Это осознанное ограничение CSP.
Метрики доступны только на приватном порту с отдельным bearer token.
Логи API не содержат тело, query string, пароли и токены.

## Обновление и возврат версии

Перед обновлением сделать проверенный backup. Сохранить image digest текущего
выпуска. Миграции выполняются отдельным job, down автоматически не запускается.
При ошибке миграции приложение не стартует. Откат образов допустим лишь при
совместимости схемы; иначе восстановить согласованный снимок в новую БД/volume,
проверить его и только затем переключить конфигурацию. Никакого down --volumes.
Остановка локального стенда: compose stop; volumes и ключи остаются сохранены.

## Основания решений

- [Next.js CSP](https://nextjs.org/docs/app/guides/content-security-policy)
- [Caddy reverse proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)
- [Restic backup](https://restic.readthedocs.io/en/stable/040_backup.html)
