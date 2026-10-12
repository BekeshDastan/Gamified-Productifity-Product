import { AppShell } from "@/components/app-shell";
import OnboardingPage from "../app/onboarding/page";
export const metadata = { robots: { index: false, follow: false } };
export default function InitialSetup() {
  return (
    <AppShell>
      <OnboardingPage />
    </AppShell>
  );
}
