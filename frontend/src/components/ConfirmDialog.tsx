import { useEffect, useId, useRef, type ReactNode } from "react";
import { IconClose } from "./icons";

export interface DialogProps {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  /** Extra class appended to the <dialog> element (e.g. the command palette's wider shell). */
  className?: string;
  /** Skip the title bar + close button — used by the command palette, whose own search
   *  input serves as the visual header and which closes via Esc/backdrop/selection instead. */
  hideHead?: boolean;
}

/**
 * Generic modal built on the native <dialog> element: showModal() traps focus and Esc fires the
 * native `cancel` event for us, satisfying the keyboard quality bar with no bespoke focus-trap code.
 */
export function Dialog({ open, title, onClose, children, footer, className = "", hideHead = false }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleID = useId();

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      className={["dialog", className].filter(Boolean).join(" ")}
      aria-labelledby={hideHead ? undefined : titleID}
      aria-label={hideHead ? title : undefined}
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      {!hideHead && (
        <div className="dialog-head">
          <h2 id={titleID}>{title}</h2>
          {/* Deliberately no Tooltip here (unlike other icon-only buttons app-wide): native
              showModal() auto-focuses this button as the dialog's initial-focus target, which
              would immediately trigger the tooltip open on every dialog. A Base UI Tooltip's
              own Escape-to-dismiss then consumes the Escape keydown before it can reach the
              dialog's native cancel handling — verified by hand: it silently broke Esc-to-close
              on every dialog in the app. aria-label already covers the a11y need here. */}
          <button type="button" className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close dialog">
            <IconClose />
          </button>
        </div>
      )}
      <div className="dialog-body">{children}</div>
      {footer && <div className="dialog-foot">{footer}</div>}
    </dialog>
  );
}

export interface ConfirmDialogProps {
  open: boolean;
  title: string;
  onClose: () => void;
  onConfirm: () => void;
  confirmLabel?: string;
  danger?: boolean;
  busy?: boolean;
  children: ReactNode;
}

export function ConfirmDialog({
  open,
  title,
  onClose,
  onConfirm,
  confirmLabel = "Confirm",
  danger = false,
  busy = false,
  children,
}: ConfirmDialogProps) {
  return (
    <Dialog
      open={open}
      title={title}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className={danger ? "btn btn-danger" : "btn btn-primary"} onClick={onConfirm} disabled={busy}>
            {busy ? "Working…" : confirmLabel}
          </button>
        </>
      }
    >
      {children}
    </Dialog>
  );
}
