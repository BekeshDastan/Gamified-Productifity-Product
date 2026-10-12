import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

export function inspectFile(filename, contents) {
  const issues = [];
  const basename = path.posix.basename(filename);
  if ((basename === ".env" || basename.startsWith(".env.")) && !basename.endsWith(".example")) {
    issues.push("local environment file");
  }
  if (/(^|\/)(node_modules|\.next|\.data|bin)(\/|$)|\.(pem|key|dump|exe)$/i.test(filename)) {
    issues.push("private or generated file");
  }
  if (/-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/.test(contents)) {
    issues.push("private key content");
  }
  if (/\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{60,})\b/.test(contents)) {
    issues.push("GitHub credential");
  }
  return issues;
}

export function checkRepository() {
  const root = fileURLToPath(new URL("../", import.meta.url));
  const filenames = [
    ...new Set(
      execFileSync(
        "git",
        [
          "-c",
          `safe.directory=${root.replace(/\\/g, "/").replace(/\/$/, "")}`,
          "ls-files",
          "--cached",
          "--others",
          "--exclude-standard",
          "-z",
        ],
        { cwd: root, encoding: "utf8" },
      )
        .split("\0")
        .filter(Boolean),
    ),
  ];
  let failures = 0;
  for (const filename of filenames) {
    if (!existsSync(path.join(root, filename))) continue;
    const contents = readFileSync(path.join(root, filename), "utf8");
    for (const issue of inspectFile(filename, contents)) {
      console.error(`${filename}: ${issue} (value redacted)`);
      failures++;
    }
  }
  if (failures) throw new Error(`Repository policy rejected ${failures} finding(s)`);
  console.log(`Repository policy: ${filenames.length} tracked files checked`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  checkRepository();
}
