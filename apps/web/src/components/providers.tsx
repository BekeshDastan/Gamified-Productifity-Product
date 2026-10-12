"use client";
import type { ReactNode } from "react";
import { AccountQueryProvider } from "@/lib/api/query-provider";
import { ThemeProvider } from "./theme-provider";
import { ToastProvider } from "./ui/toast";
import { UnsavedChangesProvider } from "@/lib/forms/unsaved-changes";
export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <AccountQueryProvider accountId={null}>
        <UnsavedChangesProvider>
          <ToastProvider>{children}</ToastProvider>
        </UnsavedChangesProvider>
      </AccountQueryProvider>
    </ThemeProvider>
  );
}
