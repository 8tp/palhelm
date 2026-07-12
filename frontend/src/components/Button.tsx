import type { ButtonHTMLAttributes } from "react";

export type ButtonVariant = "default" | "primary" | "danger" | "danger-solid" | "ghost";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  sm?: boolean;
}

export function Button({ variant = "default", sm = false, className = "", ...rest }: ButtonProps) {
  const cls = ["btn", variant !== "default" ? `btn-${variant}` : "", sm ? "btn-sm" : "", className]
    .filter(Boolean)
    .join(" ");
  return <button className={cls} {...rest} />;
}
