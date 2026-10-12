import type { ComponentProps } from "react";
import { Slot } from "radix-ui";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const variants = cva("ui-button", {
  variants: {
    variant: {
      primary: "ui-button-primary",
      secondary: "ui-button-secondary",
      ghost: "ui-button-ghost",
      danger: "ui-button-danger",
    },
    size: { default: "", small: "ui-button-small", icon: "ui-button-icon" },
  },
  defaultVariants: { variant: "primary", size: "default" },
});
export function Button({
  className,
  variant,
  size,
  asChild = false,
  type = "button",
  ...props
}: ComponentProps<"button"> & VariantProps<typeof variants> & { asChild?: boolean }) {
  const Component = asChild ? Slot.Root : "button";
  return (
    <Component
      type={asChild ? undefined : type}
      className={cn(variants({ variant, size }), className)}
      {...props}
    />
  );
}
