import { randomBytes } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

// Only creates a new, ignored file. Existing keys and volumes are never reset.
const directory = path.resolve(".data/release-local");
mkdirSync(directory, { recursive: true, mode: 0o700 });
const filename = path.join(directory, "release.env");
const password = randomBytes(32).toString("hex");
const backupFile = path.join(directory, "backup.env");
const metricsFile = path.join(directory, "metrics-token");
const metricsToken = randomBytes(32).toString("base64");
writeFileSync(
  filename,
  [
    "APP_ENV=production",
    "APP_DOMAIN=localhost",
    "APP_ORIGIN=https://localhost:3443",
    "RELEASE_TAG=stage25",
    `RELEASE_ENV_FILE=${filename.replaceAll("\\", "/")}`,
    `POSTGRES_PASSWORD=${password}`,
    `DATABASE_URL=postgres://ascend:${password}@db:5432/ascend?sslmode=disable`,
    `AUTH_SIGNING_KEY=${randomBytes(32).toString("base64")}`,
    `OPS_BEARER_TOKEN=${metricsToken}`,
    `METRICS_TOKEN_FILE=${metricsFile.replaceAll("\\", "/")}`,
    `BACKUP_ENV_FILE=${backupFile.replaceAll("\\", "/")}`,
    "AUTH_INSECURE_LOCAL_COOKIES=false",
    "EDGE_BIND=127.0.0.1",
    "HTTP_PORT=3080",
    "HTTPS_PORT=3443",
    "LOG_LEVEL=info",
    "SMTP_HOST=",
    "SMTP_PORT=587",
    "MAIL_FROM=",
    "FEATURE_MYSTERY_REWARDS=true",
    "",
  ].join("\n"),
  { flag: "wx", mode: 0o600 },
);
writeFileSync(
  backupFile,
  `RESTIC_REPOSITORY=/repository\nRESTIC_PASSWORD=${randomBytes(32).toString("base64")}\n`,
  { flag: "wx", mode: 0o600 },
);
writeFileSync(metricsFile, metricsToken, { flag: "wx", mode: 0o644 });
console.log("Created .data/release-local/release.env; credentials were not printed.");
