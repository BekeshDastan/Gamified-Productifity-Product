import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const cwd = fileURLToPath(new URL("../services/api/", import.meta.url));
const env = { ...process.env, GOTOOLCHAIN: "go1.26.9" };
function run(command, args, capture = false) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    encoding: "utf8",
    stdio: capture ? "pipe" : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
  return result.stdout;
}

const unformatted = run("gofmt", ["-l", "cmd", "internal"], true).trim();
if (unformatted) {
  console.error(`Run gofmt on:\n${unformatted}`);
  process.exit(1);
}
run("go", ["vet", "./..."]);
run("go", ["run", "honnef.co/go/tools/cmd/staticcheck@v0.8.1", "./..."]);
