"use client";
import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useAuth } from "@/lib/auth/provider";
import { trackView } from "@/lib/navigator";
export function TaskViewObserver() {
  const path = usePathname(),
    auth = useAuth(),
    account = auth.session?.account.id;
  useEffect(() => {
    const id = path.match(/^\/app\/quests\/([0-9a-f-]{36})$/)?.[1];
    if (!id || !account) return;
    let active = true;
    const send = () => {
      if (active) void trackView(id, document.visibilityState === "visible").catch(() => {});
    };
    send();
    const timer = setInterval(send, 30000);
    document.addEventListener("visibilitychange", send);
    return () => {
      active = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", send);
      void trackView(id, false).catch(() => {});
    };
  }, [path, account]);
  return null;
}
