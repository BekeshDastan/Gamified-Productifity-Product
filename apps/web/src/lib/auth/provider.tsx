"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { clearPrivateDrafts } from "@/lib/private-drafts";
import { ApiError } from "@/lib/api/client";
import { authClient, authParams, loadSession, withAuthLock, type Session } from "./session";

type Auth = {
  session: Session | null | undefined;
  error: string | null;
  adopt: (session: Session | null) => void;
  reload: () => Promise<void>;
  logout: (all?: boolean) => Promise<void>;
};
const Context = createContext<Auth | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>();
  const [error, setError] = useState<string | null>(null);
  const channel = useRef<BroadcastChannel | null>(null);
  const generation = useRef(0);
  const invalidate = useCallback(() => {
    generation.current++;
  }, []);
  const reload = useCallback(async () => {
    const ticket = ++generation.current;
    try {
      const result = await loadSession();
      if (ticket === generation.current) {
        if (!result) clearPrivateDrafts();
        setSession(result);
        setError(null);
      }
    } catch {
      if (ticket === generation.current) {
        setSession((old) => old ?? null);
        setError("Не удалось проверить вход. Проверьте связь и повторите.");
      }
    }
  }, []);
  const adopt = useCallback((result: Session | null) => {
    generation.current++;
    clearPrivateDrafts();
    setSession(result);
    setError(null);
    channel.current?.postMessage(result ? "changed" : "signed-out");
  }, []);
  const logout = useCallback(
    async (all = false) => {
      await withAuthLock(async () => {
        const client = await authClient();
        try {
          await client.POST(all ? "/api/v1/auth/logout-all" : "/api/v1/auth/logout", {
            params: authParams(),
            body: {},
          });
        } catch (error) {
          if (!(error instanceof ApiError && error.status === 401)) throw error;
        }
        adopt(null);
      });
    },
    [adopt],
  );
  useEffect(() => {
    const broadcast =
      typeof BroadcastChannel !== "undefined" ? new BroadcastChannel("ascend-auth") : null;
    channel.current = broadcast;
    if (broadcast)
      broadcast.onmessage = (event: MessageEvent<unknown>) => {
        if (event.data === "signed-out") {
          generation.current++;
          clearPrivateDrafts();
          setSession(null);
          setError(null);
        } else if (event.data === "changed") {
          generation.current++;
          clearPrivateDrafts();
          setSession(undefined);
          void reload();
        }
      };
    const focus = () => {
      if (document.visibilityState === "visible") void reload();
    };
    window.addEventListener("focus", focus);
    document.addEventListener("visibilitychange", focus);
    let active = true;
    queueMicrotask(() => {
      if (active) void reload();
    });
    return () => {
      active = false;
      invalidate();
      broadcast?.close();
      channel.current = null;
      window.removeEventListener("focus", focus);
      document.removeEventListener("visibilitychange", focus);
    };
  }, [reload, invalidate]);
  return (
    <Context.Provider value={{ session, error, adopt, reload, logout }}>
      {children}
    </Context.Provider>
  );
}
export function useAuth() {
  const auth = useContext(Context);
  if (!auth) throw new Error("AuthProvider required");
  return auth;
}
