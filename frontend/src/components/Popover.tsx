import type { ComponentProps, ReactElement, ReactNode } from "react";
import { Popover as BasePopover } from "@base-ui/react/popover";

/**
 * Field-guide-styled popover wrapper (Base UI `Popover.*`), built per the shadcn/ui
 * adoption evaluation (the UI library evaluation) alongside DropdownMenu and
 * Tooltip. Kept generic and currently unused by any screen: nothing in the app's
 * in-scope surface (Players/Backups tables, app-root wiring) has an existing gap that
 * needs it yet — see the eval's own note that Popover is "worth it, secondary... lower
 * priority since nothing in the spec explicitly needs it yet." Ready for the Map
 * layer-picker or a metric detail popover called out in the eval once that screen work
 * is in scope, without inventing new product UI here.
 */
export function Popover({
  children,
  trigger,
  align = "center",
  side = "bottom",
}: {
  children: ReactNode;
  trigger: ReactElement;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom" | "left" | "right";
}) {
  return (
    <BasePopover.Root>
      <BasePopover.Trigger className="btn btn-sm btn-ghost" render={trigger} />
      <BasePopover.Portal>
        <BasePopover.Positioner side={side} align={align} sideOffset={8} className="popover-positioner">
          <BasePopover.Popup className="popover-popup">{children}</BasePopover.Popup>
        </BasePopover.Positioner>
      </BasePopover.Portal>
    </BasePopover.Root>
  );
}

export function PopoverTitle(props: ComponentProps<typeof BasePopover.Title>) {
  return <BasePopover.Title className="popover-title" {...props} />;
}

export function PopoverDescription(props: ComponentProps<typeof BasePopover.Description>) {
  return <BasePopover.Description className="popover-description" {...props} />;
}
