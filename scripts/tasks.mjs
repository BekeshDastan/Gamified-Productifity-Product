import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { loadEnvFile } from "node:process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL("../", import.meta.url));
const api = path.join(root, "services/api");
const npmCli =
  process.env.npm_execpath ??
  [
    path.join(path.dirname(process.execPath), "node_modules/npm/bin/npm-cli.js"),
    path.resolve(path.dirname(process.execPath), "../lib/node_modules/npm/bin/npm-cli.js"),
  ].find(existsSync);
if (!npmCli) throw new Error("npm installation not found; invoke the task through npm run");
const env = { ...process.env, GOTOOLCHAIN: "go1.26.9", NEXT_TELEMETRY_DISABLED: "1" };

async function run(command, args, cwd = root) {
  await new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, env, stdio: "inherit" });
    child.once("error", reject);
    child.once("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(`${command} exited with code ${code}`)),
    );
  });
}
const npm = (...args) => run(process.execPath, [npmCli, ...args]);
async function buildGo() {
  mkdirSync(path.join(root, "bin"), { recursive: true });
  await run(
    "go",
    ["build", "-o", path.join(root, "bin") + path.sep, "./cmd/api", "./cmd/worker"],
    api,
  );
}
function requireLocalEnv() {
  const filename = path.join(root, ".env");
  if (!existsSync(filename)) throw new Error("Create .env from .env.example first (see README)");
  loadEnvFile(filename);
  Object.assign(env, process.env, { GOTOOLCHAIN: "go1.26.9", NEXT_TELEMETRY_DISABLED: "1" });
}
async function dev() {
  requireLocalEnv();
  if (env.APP_ENV !== "development") throw new Error("dev requires APP_ENV=development");
  await run("docker", ["compose", "up", "-d", "--wait", "db", "mailpit"]);
  await buildGo();
  await run("go", ["run", "./cmd/migrate"], api);
  const children = [];
  let stopping = false;
  function stop(code) {
    if (stopping) return;
    stopping = true;
    for (const child of children) {
      if (!child.pid || child.exitCode !== null) continue;
      if (process.platform === "win32") {
        spawnSync("taskkill", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore" });
      } else {
        try {
          process.kill(-child.pid, "SIGTERM");
        } catch (error) {
          if (error.code !== "ESRCH") console.error(error.message);
        }
      }
    }
    process.exitCode = code;
  }
  process.once("SIGINT", () => stop(0));
  process.once("SIGTERM", () => stop(0));
  const launch = (command, args, cwd = root) => {
    const child = spawn(command, args, {
      cwd,
      env,
      stdio: "inherit",
      detached: process.platform !== "win32",
    });
    children.push(child);
    child.once("error", (error) => {
      console.error(error.message);
      stop(1);
    });
    child.once("exit", (code) => {
      if (!stopping) {
        console.error("Development process exited; stopping peers");
        stop(code || 1);
      }
    });
  };
  const suffix = process.platform === "win32" ? ".exe" : "";
  launch(path.join(root, `bin/api${suffix}`), []);
  launch(path.join(root, `bin/worker${suffix}`), []);
  launch(process.execPath, [npmCli, "run", "dev:web"]);
}

const commands = {
  dev,
  lint: async () => {
    await npm("run", "check:repo");
    await npm("run", "format:check");
    await npm("run", "lint:web");
    await npm("run", "typecheck");
    await npm("run", "lint:go");
  },
  test: async () => {
    await npm("run", "test:tooling");
    await npm("run", "test:web");
    await run("go", ["test", "./..."], api);
  },
  build: async () => {
    await npm("run", "build:web");
    await buildGo();
  },
  "reconcile-progression": async () => {
    requireLocalEnv();
    await run("go", ["run", "./cmd/reconcile-progression", ...process.argv.slice(3)], api);
  },
  migrate: async () => {
    requireLocalEnv();
    await run("go", ["run", "./cmd/migrate"], api);
  },
  "test-integration": async () => {
    if (existsSync(path.join(root, ".env"))) requireLocalEnv();
    for (const setting of ["TEST_DATABASE_URL", "TEST_SMTP_ADDR", "TEST_MAIL_HTTP"]) {
      if (!env[setting])
        throw new Error(`${setting} must be explicitly configured; see docs/runbook.md`);
    }
    await run("go", ["test", "-tags=integration", "-count=1", "./tests/integration/..."], api);
  },
  "check-changed": async () => {
    await commands.lint();
    await commands.test();
    await commands["test-integration"]();
    await npm("run", "build:web");
    await commands["test-e2e"]();
    await commands["test-auth-e2e"]();
  },
  "test-e2e": async () => {
    await npm("run", "test:e2e", "--workspace", "@ascend/web");
  },
  "test-auth-e2e": async () => {
    if (existsSync(path.join(root, ".env"))) requireLocalEnv();
    for (const setting of ["TEST_DATABASE_URL", "TEST_SMTP_ADDR", "TEST_MAIL_HTTP"])
      if (!env[setting]) throw new Error(`${setting} must be explicitly configured`);
    env.RUN_BROWSER_AUTH = "1";
    await run(
      "go",
      [
        "test",
        "-tags=integration",
        "-run",
        "^TestBrowserAuth$",
        "-count=1",
        "-timeout=8m",
        "./tests/integration/...",
      ],
      api,
    );
  },
};
for (const [name, milestone] of Object.entries({
  seed: "C167",
  "seed-demo": "C194",
  backup: "C197",
  "restore-verify": "C197",
})) {
  commands[name] = async () => {
    throw new Error(
      `${name}: suite not connected yet; planned in ${milestone}. No operation performed.`,
    );
  };
}
try {
  const name = process.argv[2];
  if (!Object.hasOwn(commands, name))
    throw new Error(`Unknown command. Available: ${Object.keys(commands).join(", ")}`);
  await commands[name]();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
