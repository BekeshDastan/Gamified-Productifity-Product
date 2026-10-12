"use client";
import { Toast as Primitive } from "radix-ui";
import { createContext, useContext, useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
type Notice = { title: string; description?: string; tone?: "success" | "error" };
const Context = createContext<((notice: Notice) => void) | null>(null);
export function ToastProvider({ children }: { children: ReactNode }) {
  const nextId = useRef(0);
  const [notices, setNotices] = useState<(Notice & { id: number })[]>([]);
  function notify(notice: Notice) {
    const id = ++nextId.current;
    setNotices((items) => [...items.slice(-2), { ...notice, id }]);
  }
  return (
    <Context.Provider value={notify}>
      <Primitive.Provider duration={6000} swipeDirection="right">
        {children}
        {notices.map((notice) => (
          <Primitive.Root
            key={notice.id}
            className="ui-toast"
            data-tone={notice.tone}
            onOpenChange={(open) => {
              if (!open) setNotices((items) => items.filter((item) => item.id !== notice.id));
            }}
          >
            <div>
              <Primitive.Title>
                {notice.tone === "success" ? "✓ " : notice.tone === "error" ? "! " : ""}
                {notice.title}
              </Primitive.Title>
              {notice.description && (
                <Primitive.Description className="ui-hint">
                  {notice.description}
                </Primitive.Description>
              )}
            </div>
            <Primitive.Close asChild>
              <Button size="icon" variant="ghost" aria-label="Закрыть уведомление">
                <X size={16} aria-hidden="true" />
              </Button>
            </Primitive.Close>
          </Primitive.Root>
        ))}
        <Primitive.Viewport className="ui-toast-viewport" label="Уведомления (F8)" />
      </Primitive.Provider>
    </Context.Provider>
  );
}
export function useToast() {
  const notify = useContext(Context);
  if (!notify) throw new Error("ToastProvider is required");
  return notify;
}
