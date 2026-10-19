import { spawnSync } from "node:child_process";
import path from "node:path";

const [command, envFile, project] = process.argv.slice(2);
if (
  !["init", "backup", "restore-drill", "retention-preview"].includes(command) ||
  !envFile ||
  !/^ascend-release(?:-[a-z0-9-]+)?$/.test(project ?? "")
)
  throw new Error(
    "Usage: node scripts/release-backup.mjs init|backup|restore-drill|retention-preview ENV_FILE ascend-release[-NAME]",
  );
const args = [
  "compose",
  "--env-file",
  path.resolve(envFile),
  "-f",
  "deploy/compose.yaml",
  "-f",
  "deploy/compose.backup.yaml",
  "-p",
  project,
];
function compose(extra, capture = false) {
  const result = spawnSync("docker", [...args, ...extra], {
    encoding: "utf8",
    stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.error || result.status !== 0)
    throw new Error("Backup operation failed; inspect the preceding redacted service output");
  return result.stdout?.trim() ?? "";
}
compose(["build", "backup"]);
if (command === "backup") {
  const running = compose(["ps", "--services", "--status", "running"], true).split(/\r?\n/);
  const writers = ["edge", "api", "worker"].filter((name) => running.includes(name));
  try {
    if (writers.length) compose(["stop", ...writers]);
    const remaining = compose(["ps", "--services", "--status", "running"], true).split(/\r?\n/);
    if (remaining.some((name) => ["edge", "api", "worker"].includes(name)))
      throw new Error("Writers did not stop; backup cancelled");
    compose(["run", "--rm", "--no-deps", "-e", "BACKUP_WRITERS_STOPPED=1", "backup", "backup"]);
  } finally {
    if (writers.length) compose(["start", ...writers]);
  }
} else if (command === "restore-drill") {
  compose(["up", "-d", "--wait", "restore-db"]);
  compose([
    "run",
    "--rm",
    "--no-deps",
    "-e",
    "PGHOST=restore-db",
    "-e",
    "PGDATABASE=ascend_restore",
    "backup",
    "restore-drill",
  ]);
} else {
  compose(["run", "--rm", "--no-deps", "backup", command]);
}
