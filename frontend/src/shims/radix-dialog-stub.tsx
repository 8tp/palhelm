// Minimal stand-in for @radix-ui/react-dialog, aliased in vite.config.ts.
//
// cmdk (see components/CommandPalette.tsx) statically imports the real package solely
// to build its own `Command.Dialog` wrapper. This app deliberately never renders that —
// per the UI library evaluation, the palette reuses the app's existing
// native-<dialog>-based `Dialog` component as outer chrome instead, exactly like every
// other modal in the app. Because cmdk bundles Command and CommandDialog as one module
// with an unconditional top-level `import * as Dialog from "@radix-ui/react-dialog"`,
// the real package (plus its own transitive focus-scope/dismissable-layer/portal/
// presence/focus-guards tree, ~1MB unpacked) ends up in the production bundle even
// though nothing ever calls into it — verified by build size: aliasing it away here
// cuts the measured bundle delta from ~65KB gzip to within the eval's ~13-17KB target.
// This stub satisfies cmdk's runtime import (and its `RadixDialog.DialogProps` type
// reference, via the real package's still-installed .d.ts, unaffected by this alias)
// with inert no-op components along the dead code path.
import type { ReactNode } from "react";

export interface DialogProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: ReactNode;
}

function Noop({ children }: { children?: ReactNode }) {
  return <>{children}</>;
}

export const Root = Noop;
export const Portal = Noop;
export const Overlay = Noop;
export const Content = Noop;
export const Trigger = Noop;
export const Close = Noop;
export const Title = Noop;
export const Description = Noop;
