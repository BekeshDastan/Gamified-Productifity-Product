import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
const css = readFileSync("src/app/globals.css", "utf8");
function luminance(hex: string) {
  const values = hex.match(/[a-f0-9]{2}/gi)!.map((part) => {
    const c = parseInt(part, 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return values[0]! * 0.2126 + values[1]! * 0.7152 + values[2]! * 0.0722;
}
describe("theme contrast", () => {
  for (const selector of [":root", ".dark"]) {
    it(`${selector} provides AA text contrast including semantic statuses`, () => {
      const block = css.slice(css.indexOf(`${selector} {`)).split("}")[0]!;
      const value = (name: string) => block.match(new RegExp(`--${name}: (#[a-f0-9]{6})`))![1]!;
      const pairs = [
        "foreground",
        "muted-foreground",
        "accent",
        "destructive",
        "success",
        "warning",
        "info",
      ].flatMap((name) => [
        [name, "background"],
        [name, "card"],
      ]);
      pairs.push(["primary-foreground", "primary"]);
      for (const [text, background] of pairs) {
        const a = luminance(value(text!)),
          b = luminance(value(background!));
        expect(
          (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05),
          `${text}/${background}`,
        ).toBeGreaterThanOrEqual(4.5);
      }
    });
  }
});
