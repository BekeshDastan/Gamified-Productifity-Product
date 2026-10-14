import { settingNavigation } from "@/lib/navigation";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { ArrowRight } from "lucide-react";
export default function SettingsPage() {
  return (
    <>
      <h1 className="page-title">Настройки</h1>
      <p className="page-intro">
        Подстройте ASCEND под свой день. Меняйте только то, что нужно сейчас.
      </p>
      <div className="settings-directory">
        {settingNavigation.map(({ href, label, description, icon: Icon }) => (
          <SafeLink className="directory-row" href={href} key={href}>
            <Icon size={22} aria-hidden="true" />
            <div>
              <h2>{label}</h2>
              <p>{description}</p>
            </div>
            <ArrowRight size={18} aria-hidden="true" />
          </SafeLink>
        ))}
      </div>
    </>
  );
}
