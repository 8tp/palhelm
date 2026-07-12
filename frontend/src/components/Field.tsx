import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from "react";

interface FieldShellProps {
  label: string;
  hint?: ReactNode;
  children: ReactNode;
}

export function FieldShell({ label, hint, children }: FieldShellProps) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
      {hint}
    </div>
  );
}

export interface FieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  hint?: ReactNode;
  mono?: boolean;
}

export function Field({ label, hint, mono, className = "", id, ...rest }: FieldProps) {
  const inputId = id ?? `field-${label.replace(/\s+/g, "-").toLowerCase()}`;
  return (
    <div className="field">
      <label htmlFor={inputId}>{label}</label>
      <input id={inputId} className={["input", mono ? "input-mono" : "", className].filter(Boolean).join(" ")} {...rest} />
      {hint}
    </div>
  );
}

export interface SelectFieldProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  hint?: ReactNode;
}

export function SelectField({ label, hint, className = "", id, children, ...rest }: SelectFieldProps) {
  const selectId = id ?? `field-${label.replace(/\s+/g, "-").toLowerCase()}`;
  return (
    <div className="field">
      <label htmlFor={selectId}>{label}</label>
      <select id={selectId} className={["input", className].filter(Boolean).join(" ")} {...rest}>
        {children}
      </select>
      {hint}
    </div>
  );
}

export function SearchField(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <div className="search">
      <svg width={14} height={14} viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth={1.5} aria-hidden="true">
        <circle cx="6" cy="6" r="4.2" />
        <path d="M9.5 9.5L13 13" strokeLinecap="round" />
      </svg>
      <input className="input" type="search" {...props} />
    </div>
  );
}
