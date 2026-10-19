import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";

const args = [
  "compose",
  "--env-file",
  ".data/release-local/release.env",
  "-f",
  "deploy/compose.yaml",
  "-f",
  "deploy/compose.monitoring.yaml",
  "-p",
  "ascend-release-local",
];
function compose(...extra) {
  const result = spawnSync("docker", [...args, ...extra], { stdio: "inherit" });
  assert.equal(result.status, 0, "Local fault drill command failed");
}
async function workerAlert() {
  const response = await fetch("http://127.0.0.1:19090/api/v1/alerts", {
    signal: AbortSignal.timeout(5000),
  });
  assert.ok(response.ok);
  return (await response.json()).data.alerts.some(
    (alert) => alert.labels.alertname === "AscendWorkerStopped" && alert.state === "firing",
  );
}
async function waitFor(expected, seconds) {
  const start = Date.now();
  while (Date.now() - start < seconds * 1000) {
    if ((await workerAlert()) === expected) return Math.round((Date.now() - start) / 1000);
    await new Promise((resolve) => setTimeout(resolve, 5000));
  }
  throw new Error("Worker alert did not reach the expected state");
}
assert.equal(await workerAlert(), false, "Begin only with a healthy worker");
let firingSeconds;
try {
  compose("stop", "worker");
  firingSeconds = await waitFor(true, 210);
  console.log(`Stopped worker produced a firing alert after ${firingSeconds}s.`);
} finally {
  compose("start", "worker");
}
const recoverySeconds = await waitFor(false, 90);
writeFileSync(
  ".data/release-local/operations-result.json",
  JSON.stringify({ workerStoppedAlert: true, firingSeconds, recoverySeconds }, null, 2),
);
console.log(`Worker alert resolved after recovery in ${recoverySeconds}s.`);
