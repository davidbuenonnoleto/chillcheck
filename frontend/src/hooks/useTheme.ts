import { useState } from "react";

// Light/dark theme without a dependency. ChillCheck already ships a full `.dark`
// palette in index.css; this wires it to a toggle and persists the choice.
export type Theme = "light" | "dark";

const KEY = "chillcheck_theme";

// initialTheme resolves the stored preference, falling back to the OS setting.
export function initialTheme(): Theme {
  const stored = localStorage.getItem(KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

// applyTheme toggles the `.dark` class on <html>, which drives the CSS variables.
export function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

// useTheme reads the live theme from the DOM (set by applyTheme at startup) and
// exposes a toggle that persists the choice.
export function useTheme() {
  const [theme, setTheme] = useState<Theme>(() =>
    document.documentElement.classList.contains("dark") ? "dark" : "light"
  );

  function toggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    applyTheme(next);
    localStorage.setItem(KEY, next);
  }

  return { theme, toggle };
}
