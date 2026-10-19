import type { Metadata } from "next";
import { headers } from "next/headers";
import type { ReactNode } from "react";
import { Providers } from "@/components/providers";
import "./globals.css";
import "./product.css";
import "./experience.css";
import "./interaction.css";
import "./workspaces.css";
import "./social.css";

export const metadata: Metadata = {
  title: "ASCEND — Level Up Your Life",
  description: "Личная дисциплина, реальные действия и честный прогресс.",
};

export default async function RootLayout({ children }: { children: ReactNode }) {
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    <html lang="ru" suppressHydrationWarning>
      <body>
        <Providers nonce={nonce}>{children}</Providers>
      </body>
    </html>
  );
}
