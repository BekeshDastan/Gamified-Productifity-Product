import type { Metadata } from "next";
import { NotificationUnsubscribe } from "@/components/notification-unsubscribe";
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};
export default function UnsubscribePage() {
  return <NotificationUnsubscribe />;
}
