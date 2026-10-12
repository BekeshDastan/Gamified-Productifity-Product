"use client";
import { Tooltip as Primitive } from "radix-ui";
import type { ReactNode } from "react";
export function Tooltip({ content, children }: { content: string; children: ReactNode }) {
  return (
    <Primitive.Provider delayDuration={250}>
      <Primitive.Root>
        <Primitive.Trigger asChild>{children}</Primitive.Trigger>
        <Primitive.Portal>
          <Primitive.Content className="ui-tooltip" sideOffset={8}>
            {content}
            <Primitive.Arrow className="ui-tooltip-arrow" />
          </Primitive.Content>
        </Primitive.Portal>
      </Primitive.Root>
    </Primitive.Provider>
  );
}
