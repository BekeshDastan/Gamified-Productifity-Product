import type { ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
export const metadata = { robots: { index: false, follow: false } };
export default function Layout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
