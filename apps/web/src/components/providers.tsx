"use client";
import type { ReactNode } from "react";
import { AccountQueryProvider } from "@/lib/api/query-provider";
import { ThemeProvider } from "./theme-provider";
import { ToastProvider } from "./ui/toast";
export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <AccountQueryProvider accountId={null}>
        <ToastProvider>{children}</ToastProvider>
      </AccountQueryProvider>
    </ThemeProvider>
  );
}
