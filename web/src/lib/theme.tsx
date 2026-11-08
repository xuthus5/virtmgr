import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export const THEME_STORAGE_KEY = "virtmgr-theme";

export type Theme = "light" | "dark";
type Preference = Theme | "system";

type ThemeContextValue = {
  theme: Theme;
  preference: Preference;
  setPreference: (preference: Preference) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function systemTheme(): Theme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function readPreference(): Preference {
  const stored = localStorage.getItem(THEME_STORAGE_KEY);
  return stored === "light" || stored === "dark" || stored === "system" ? stored : "system";
}

function resolveTheme(preference: Preference): Theme {
  return preference === "system" ? systemTheme() : preference;
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.dataset.theme = theme;
  root.style.colorScheme = theme;
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", theme === "dark" ? "#10151f" : "#0B63CE");
}

export function initializeTheme() {
  applyTheme(resolveTheme(readPreference()));
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<Preference>(() => readPreference());
  const theme = useMemo(() => resolveTheme(preference), [preference]);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    if (preference !== "system") return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => applyTheme(resolveTheme("system"));
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [preference]);

  const value = useMemo<ThemeContextValue>(() => ({
    theme,
    preference,
    setPreference: (next) => {
      localStorage.setItem(THEME_STORAGE_KEY, next);
      setPreferenceState(next);
    },
    toggleTheme: () => {
      const next = theme === "dark" ? "light" : "dark";
      localStorage.setItem(THEME_STORAGE_KEY, next);
      setPreferenceState(next);
    },
  }), [preference, theme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (!value) throw new Error("useTheme must be used inside ThemeProvider");
  return value;
}
