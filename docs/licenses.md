# Каталог лицензий выпуска

Исходные copyright/license notices остаются в установленных пакетах и образах.
Этот каталог помогает проверке состава; он не заменяет тексты лицензий.

- npm production-зависимости и их версии: `npm-production-licenses.json`,
  воспроизводится `node scripts/license-inventory.mjs` из package-lock.json.
- Основные Go-модули: chi, pgx, goose, golang-jwt/jwt — MIT;
  golang.org/x/crypto, x/image, x/sync, x/text — BSD-3-Clause;
  точные версии фиксированы в services/api/go.mod и go.sum. Лицензии модулей
  доступны в Go module cache, полный состав — `go list -m all`.
- Caddy 2.10.2 и Prometheus 3.5.0 — Apache-2.0.
- ClamAV 1.4.3 — GPL-2.0; поставляется отдельным неизменённым scanner-контейнером
  с конфигурацией, исходный проект Cisco-Talos/clamav и уведомления сохранены.
- Restic в backup-образе — BSD-2-Clause, PostgreSQL — PostgreSQL License.
- Debian/Alpine и их системные пакеты имеют отдельные лицензии, доступные
  внутри образов; для Debian см. `/usr/share/doc/*/copyright`.
- Визуальные ресурсы приложения: `assets.md` и `assets/character-licenses.md`.

При распространении образов сохранять notices и соответствующий исходный
код компонентов с такими условиями. Обновление dependency lock или базового
образа требует повторной проверки состава; не считать этот снимок вечным.
