"use client";
import { ErrorState } from "@/components/ui/states";
export default function ErrorPage({
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  return <ErrorState retry={retry} />;
}
