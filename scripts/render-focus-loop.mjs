// Render the original vector scene into an eight-second, silent, seamless WebM.
// Usage: node scripts/render-focus-loop.mjs /path/to/ffmpeg
import sharp from "sharp";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { fileURLToPath } from "node:url";
import { mkdir, writeFile } from "node:fs/promises";

const destination = new URL("../apps/web/public/art/", import.meta.url);
await mkdir(destination, { recursive: true });
function scene(phase) {
  const orbit = (x, y, rx, ry, offset, size, color) =>
    `<circle cx="${x + Math.cos(phase + offset) * rx}" cy="${y + Math.sin(phase + offset) * ry}" r="${size}" fill="${color}"/>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="400" viewBox="0 0 640 400"><defs><linearGradient id="sky" x2="1" y2="1"><stop stop-color="#38356f"/><stop offset="1" stop-color="#8283c7"/></linearGradient><radialGradient id="sun" cx=".3" cy=".2"><stop stop-color="#ffe6d1"/><stop offset=".65" stop-color="#ecaf9e"/><stop offset="1" stop-color="#cb8a93"/></radialGradient><linearGradient id="hill" x2="0" y2="1"><stop stop-color="#b4a8dc"/><stop offset="1" stop-color="#69639d"/></linearGradient></defs><rect width="640" height="400" fill="url(#sky)"/><g fill="#f8f0ff" opacity=".6"><circle cx="81" cy="68" r="2"/><circle cx="150" cy="108" r="1.5"/><circle cx="546" cy="62" r="2"/><circle cx="492" cy="114" r="1.5"/><circle cx="375" cy="45" r="1.5"/><circle cx="232" cy="39" r="2"/></g><ellipse cx="324" cy="195" rx="200" ry="76" fill="none" stroke="#d8cff7" stroke-opacity=".35" transform="rotate(-18 324 195)"/><ellipse cx="324" cy="195" rx="165" ry="51" fill="none" stroke="#d8cff7" stroke-opacity=".18" transform="rotate(18 324 195)"/>${orbit(324, 178, 190, 67, 0, 13, "#b3e2cf")}${orbit(324, 178, 160, 55, Math.PI, 8, "#eed7a8")}<circle cx="324" cy="178" r="${64 + Math.sin(phase) * 2}" fill="url(#sun)"/><ellipse cx="320" cy="290" rx="67" ry="8" fill="#31254d" opacity=".12"/><path d="M0 320 Q80 210 180 300 T370 290 T640 270 V400 H0Z" fill="url(#hill)"/><path d="M0 350 Q130 275 270 350 T640 320 V400 H0Z" fill="#4e477d"/><path d="M250 400 Q280 350 355 337 Q397 325 356 310 Q321 299 352 288" fill="none" stroke="#dbd4ee" stroke-width="4" stroke-linecap="round" opacity=".65"/><g opacity=".5"><ellipse cx="${100 + Math.sin(phase) * 12}" cy="240" rx="54" ry="8" fill="#d0c5e8"/><ellipse cx="${525 - Math.sin(phase) * 16}" cy="232" rx="63" ry="7" fill="#d0c5e8"/></g></svg>`;
}
await writeFile(new URL("orbit-poster.svg", destination), scene(0));
if (!process.argv[2]) throw new Error("Pass a local ffmpeg executable path.");
const video = spawn(
  process.argv[2],
  [
    "-y",
    "-f",
    "image2pipe",
    "-c:v",
    "mjpeg",
    "-r",
    "24",
    "-i",
    "pipe:0",
    "-an",
    "-c:v",
    "libvpx-vp9",
    "-b:v",
    "0",
    "-crf",
    "36",
    "-pix_fmt",
    "yuv420p",
    fileURLToPath(new URL("orbit-loop.webm", destination)),
  ],
  { stdio: ["pipe", "ignore", "pipe"] },
);
let errors = "";
video.stderr.on("data", (chunk) => (errors += chunk));
video.stdin.on("error", () => {});
const completed = once(video, "close");
for (let i = 0; i < 192; i++) {
  const frame = await sharp(Buffer.from(scene((i / 192) * Math.PI * 2)))
    .jpeg({ quality: 90 })
    .toBuffer();
  if (video.exitCode !== null) throw new Error(errors);
  if (!video.stdin.write(frame))
    await Promise.race([
      once(video.stdin, "drain"),
      completed.then(() => {
        throw new Error(errors);
      }),
    ]);
}
video.stdin.end();
const [code] = await completed;
if (code !== 0) throw new Error(errors);
console.log("Rendered orbit-loop.webm: 8s / 640×400 / 24fps / no audio.");
