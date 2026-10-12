import { cpSync, existsSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

const web = fileURLToPath(new URL("../apps/web/", import.meta.url));
const standalone = path.join(web, ".next/standalone/apps/web");
const server = path.join(standalone, "server.js");
if (!existsSync(server)) throw new Error("Build web before starting the production server");
cpSync(path.join(web, ".next/static"), path.join(standalone, ".next/static"), { recursive: true });
if (existsSync(path.join(web, "public")))
  cpSync(path.join(web, "public"), path.join(standalone, "public"), { recursive: true });
process.env.HOSTNAME ??= "127.0.0.1";
await import(pathToFileURL(server).href);
