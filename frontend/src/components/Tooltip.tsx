import type { ReactElement, ReactNode, RefObject } from "react";
import { Tooltip as BaseTooltip } from "@base-ui/react/tooltip";

/**
 * Shared delay group (see docs/research/raw/shadcn-eval.md §1): mount once near the app
 * root so hovering from one icon button to the next re-triggers instantly instead of
 * re-waiting the full delay, matching how a native OS tooltip group behaves.
 */
export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <BaseTooltip.Provider delay={400} closeDelay={0}>
      {children}
    </BaseTooltip.Provider>
  );
}

/**
 * Wraps a single interactive child (usually an icon-only button) with a field-guide
 * tooltip — replaces bare `title=""` attributes: a 400ms hover delay, keyboard-focus
 * trigger, and Escape-to-dismiss all come from Base UI's `Tooltip.Root` for free.
 * `children` must be a single element that accepts a ref and forwards extra props.
 */
export function Tooltip({
  label,
  children,
  side = "top",
  container,
}: {
  label: ReactNode;
  children: ReactElement;
  side?: "top" | "bottom" | "left" | "right";
  /**
   * Portal target override. Base UI requires `Tooltip.Portal` (it throws without one),
   * and its default target — `document.body` — sits *below* a native `<dialog>`'s
   * browser top layer regardless of z-index, so a hover-triggered tooltip on a button
   * inside an open dialog would render invisibly underneath it. Pass the enclosing
   * `<dialog>`'s ref to portal inside it instead. NOTE: don't use Tooltip at all on a
   * dialog's own default-focus target (e.g. its close button) — `showModal()` autofocuses
   * it, which opens the tooltip, and its Escape-to-dismiss then consumes the Escape
   * keydown before it reaches the dialog's native cancel handling (verified by hand:
   * this silently broke Esc-to-close app-wide). See ConfirmDialog.tsx's close button.
   */
  container?: RefObject<HTMLElement | null>;
}) {
  if (!label) return children;
  return (
    <BaseTooltip.Root>
      <BaseTooltip.Trigger render={children} />
      <BaseTooltip.Portal container={container}>
        <BaseTooltip.Positioner side={side} sideOffset={6} className="tooltip-positioner">
          <BaseTooltip.Popup className="tooltip-popup">{label}</BaseTooltip.Popup>
        </BaseTooltip.Positioner>
      </BaseTooltip.Portal>
    </BaseTooltip.Root>
  );
}
