"use client";
import { Dialog as Primitive } from "radix-ui";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "./button";
import { cn } from "@/lib/utils";

export function Dialog({
  title,
  description,
  trigger,
  children,
  open,
  onOpenChange,
  className,
}: {
  title: string;
  description: string;
  trigger?: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
}) {
  return (
    <Primitive.Root open={open} onOpenChange={onOpenChange}>
      {trigger && <Primitive.Trigger asChild>{trigger}</Primitive.Trigger>}
      <Primitive.Portal>
        <Primitive.Overlay className="ui-overlay" />
        <Primitive.Content className={cn("ui-dialog", className)}>
          <div className="ui-dialog-heading">
            <Primitive.Title>{title}</Primitive.Title>
            <Primitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label="Закрыть">
                <X size={20} aria-hidden="true" />
              </Button>
            </Primitive.Close>
          </div>
          <Primitive.Description className="ui-hint">{description}</Primitive.Description>
          {children}
        </Primitive.Content>
      </Primitive.Portal>
    </Primitive.Root>
  );
}
