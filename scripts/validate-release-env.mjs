import { parseEnv } from "node:util";
import { readFileSync } from "node:fs";
const [file, mode] = process.argv.slice(2);
if (!file) throw new Error("Provide a private release env file; values are never printed");
if (mode !== undefined && mode !== "--local") throw new Error("Unknown validation mode");
const env = parseEnv(readFileSync(file, "utf8"));
const failures = [];
const requireSetting = (name) => {
  if (!env[name]?.trim()) failures.push(`${name} is required`);
};
for (const name of [
  "APP_ORIGIN",
  "APP_DOMAIN",
  "RELEASE_TAG",
  "RELEASE_ENV_FILE",
  "POSTGRES_PASSWORD",
  "DATABASE_URL",
  "AUTH_SIGNING_KEY",
])
  requireSetting(name);
if (env.APP_ENV !== "production") failures.push("APP_ENV must be production");
if (env.AUTH_INSECURE_LOCAL_COOKIES !== "false") failures.push("Secure cookies must be enabled");
if (!/^[A-Za-z0-9+/]{43}=$/.test(env.AUTH_SIGNING_KEY ?? ""))
  failures.push("AUTH_SIGNING_KEY must encode 32 random bytes");
if ((env.POSTGRES_PASSWORD ?? "").length < 24)
  failures.push("POSTGRES_PASSWORD must contain at least 24 random characters");
try {
  const origin = new URL(env.APP_ORIGIN);
  if (origin.hostname !== env.APP_DOMAIN)
    failures.push("APP_DOMAIN must match the APP_ORIGIN hostname");
  if (
    origin.protocol !== "https:" ||
    origin.pathname !== "/" ||
    origin.username ||
    origin.password ||
    origin.search ||
    origin.hash
  )
    failures.push("APP_ORIGIN must be a clean HTTPS origin");
  if (mode !== "--local" && ["localhost", "127.0.0.1"].includes(origin.hostname))
    failures.push("Public release requires the actual domain");
  const database = new URL(env.DATABASE_URL);
  if (
    database.hostname !== "db" ||
    database.pathname !== "/ascend" ||
    decodeURIComponent(database.password) !== env.POSTGRES_PASSWORD
  )
    failures.push("DATABASE_URL must match the private compose database and password");
} catch {
  failures.push("Invalid origin or database URL");
}
if (mode !== "--local") {
  for (const name of [
    "SMTP_HOST",
    "SMTP_PORT",
    "MAIL_FROM",
    "OPS_BEARER_TOKEN",
    "METRICS_TOKEN_FILE",
    "BACKUP_ENV_FILE",
  ])
    requireSetting(name);
}
if (env.OPS_BEARER_TOKEN && env.OPS_BEARER_TOKEN.length < 43)
  failures.push("OPS_BEARER_TOKEN must contain at least 43 random characters");
if (failures.length) throw new Error(failures.join("; "));
console.log(
  mode === "--local"
    ? "Local production-mode configuration valid; external mail, domain and off-site storage are not accepted by this check."
    : "Release configuration valid; live mail, restore and alert delivery still require acceptance.",
);
