// Theme switching: explicit data-theme="dark|light" on <html> overrides the OS preference in
// both directions (see tokens.css); "system" removes the attribute so the media query applies.

export type ThemeChoice = "system" | "dark" | "light";

const STORAGE_KEY = "palhelm.theme";

export function getThemeChoice(): ThemeChoice {
  const v = localStorage.getItem(STORAGE_KEY);
  return v === "dark" || v === "light" ? v : "system";
}

export function applyThemeChoice(choice: ThemeChoice): void {
  if (choice === "system") {
    localStorage.removeItem(STORAGE_KEY);
    document.documentElement.removeAttribute("data-theme");
  } else {
    localStorage.setItem(STORAGE_KEY, choice);
    document.documentElement.setAttribute("data-theme", choice);
  }
}

/** Apply the persisted choice on boot (call once before first render). */
export function initTheme(): void {
  const choice = getThemeChoice();
  if (choice !== "system") document.documentElement.setAttribute("data-theme", choice);
}
