import type { ReactNode } from "react";
import { IconInfo, IconWarn } from "./icons";

export function Banner({ tone, children }: { tone: "info" | "warn"; children: ReactNode }) {
  return (
    <div className={`banner banner-${tone}`}>
      {tone === "info" ? (
        // info banners are a field note on an inked card, not a wash — the icon sits in its
        // own stamped chip (see Config.css's page-local .banner-info / .stamp-i override)
        <span className="stamp-i" aria-hidden="true">
          <IconInfo />
        </span>
      ) : (
        <IconWarn />
      )}
      <span>{children}</span>
    </div>
  );
}
