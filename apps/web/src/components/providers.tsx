"use client";
import type { ReactNode } from "react";
import { AuthProvider, useAuth } from "@/lib/auth/provider";
import { ThemeProvider } from "./theme-provider";
import { ToastProvider } from "./ui/toast";
import { UnsavedChangesProvider } from "@/lib/forms/unsaved-changes";
export function Providers({ children, nonce }: { children: ReactNode; nonce?: string }) {
  return (
    <ThemeProvider nonce={nonce}>
      <AuthProvider>
        <UnsavedChangesProvider>
          <AccountToasts>{children}</AccountToasts>
        </UnsavedChangesProvider>
      </AuthProvider>
    </ThemeProvider>
  );
}

function AccountToasts({ children }: { children: ReactNode }) {
  const auth = useAuth();
  return <ToastProvider owner={auth.session?.account.id ?? "public"}>{children}</ToastProvider>;
}
