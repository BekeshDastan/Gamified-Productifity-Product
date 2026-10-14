import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Providers } from "@/components/providers";
import "./globals.css";
import "./product.css";

export const metadata: Metadata = {
  title: "ASCEND — Level Up Your Life",
  description: "Личная дисциплина, реальные действия и честный прогресс.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
