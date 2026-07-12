import type { ComponentProps, ReactNode } from "react";
import { Menu } from "@base-ui/react/menu";
import { IconKebab } from "./icons";

/**
 * Field-guide-styled dropdown menu — a thin wrapper over Base UI's `Menu.*` compound
 * components (see docs/research/raw/shadcn-eval.md §3). Base UI ships zero CSS; all
 * visual states are driven off its `data-*` attributes (`data-open`/`data-closed` on the
 * popup, `data-highlighted`/`data-disabled` on items) styled in styles/app.css under
 * "dropdown menu". No new mutation logic lives here — callers pass the same handlers
 * that already open the app's existing ConfirmDialog-based flows.
 */
export function DropdownMenu({
  children,
  trigger,
  triggerLabel = "More actions",
  triggerClassName = "btn btn-sm btn-ghost row-kebab",
  align = "end",
  side = "bottom",
}: {
  children: ReactNode;
  trigger?: ReactNode;
  triggerLabel?: string;
  triggerClassName?: string;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom" | "left" | "right";
}) {
  return (
    <Menu.Root>
      <Menu.Trigger type="button" className={triggerClassName} aria-label={triggerLabel}>
        {trigger ?? <IconKebab />}
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner side={side} align={align} sideOffset={6} className="menu-positioner">
          <Menu.Popup className="menu-popup">{children}</Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}

export function DropdownMenuItem({
  danger = false,
  className = "",
  ...props
}: ComponentProps<typeof Menu.Item> & { danger?: boolean }) {
  return <Menu.Item className={["menu-item", danger ? "menu-item-danger" : "", className].filter(Boolean).join(" ")} {...props} />;
}

/** A menu entry that navigates/downloads via a real `<a>` (e.g. backup download). */
export function DropdownMenuLinkItem({
  danger = false,
  className = "",
  ...props
}: ComponentProps<typeof Menu.LinkItem> & { danger?: boolean }) {
  return <Menu.LinkItem className={["menu-item", danger ? "menu-item-danger" : "", className].filter(Boolean).join(" ")} {...props} />;
}

export function DropdownMenuGroupLabel(props: ComponentProps<typeof Menu.GroupLabel>) {
  return <Menu.GroupLabel className="menu-group-label" {...props} />;
}

export function DropdownMenuGroup(props: ComponentProps<typeof Menu.Group>) {
  return <Menu.Group {...props} />;
}
