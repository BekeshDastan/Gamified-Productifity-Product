"use client";
import { ThemeProvider as Provider } from "next-themes";
import type { ReactNode } from "react";

export function ThemeProvider({ children, nonce }: { children: ReactNode; nonce?: string }) {
  return (
    <Provider
      nonce={nonce}
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
      storageKey="ascend-theme"
    >
      {children}
    </Provider>
  );
}
