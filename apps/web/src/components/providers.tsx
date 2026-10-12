"use client";
import type { ReactNode } from "react";
import { AuthProvider } from "@/lib/auth/provider";
import { ThemeProvider } from "./theme-provider";
import { ToastProvider } from "./ui/toast";
import { UnsavedChangesProvider } from "@/lib/forms/unsaved-changes";
export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <AuthProvider>
        <UnsavedChangesProvider>
          <ToastProvider>{children}</ToastProvider>
        </UnsavedChangesProvider>
      </AuthProvider>
    </ThemeProvider>
  );
}
