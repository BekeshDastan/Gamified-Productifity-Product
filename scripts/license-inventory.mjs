import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
const lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
const packages = [];
for (const [location, entry] of Object.entries(lock.packages)) {
  if (!location.includes("node_modules/") || entry.dev || entry.link) continue;
  let license = entry.license;
  let name = entry.name ?? location.split("node_modules/").at(-1);
  if (!license) {
    try {
      const installed = JSON.parse(readFileSync(path.join(location, "package.json"), "utf8"));
      license = installed.license;
      name = installed.name ?? name;
    } catch {
      /* Optional platform packages may not be installed. */
    }
  }
  packages.push({ name, version: entry.version, license: license ?? "REVIEW_REQUIRED", location });
}
packages.sort((a, b) => a.location.localeCompare(b.location, "en"));
writeFileSync(
  "docs/npm-production-licenses.json",
  JSON.stringify({ source: "package-lock.json", packages }, null, 2) + "\n",
);
console.log(
  `Recorded ${packages.length} production package entries; ${packages.filter((entry) => entry.license === "REVIEW_REQUIRED").length} require metadata review.`,
);
