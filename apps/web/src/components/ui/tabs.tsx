"use client";
import { Tabs as Primitive } from "radix-ui";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
export const Tabs = Primitive.Root;
export function TabsList({ className, ...props }: ComponentProps<typeof Primitive.List>) {
  return <Primitive.List className={cn("ui-tabs-list", className)} {...props} />;
}
export function TabsTrigger({ className, ...props }: ComponentProps<typeof Primitive.Trigger>) {
  return <Primitive.Trigger className={cn("ui-tab", className)} {...props} />;
}
export function TabsContent({ className, ...props }: ComponentProps<typeof Primitive.Content>) {
  return <Primitive.Content className={cn("ui-tab-content", className)} {...props} />;
}
