import { useEffect, useMemo, useState, type ReactNode } from "react"

import { ThemeContext, type ThemePreference } from "@/features/theme/theme-context"

const storageKey = "gradeium-theme"
const darkMediaQuery = "(prefers-color-scheme: dark)"

function storedPreference(): ThemePreference {
  const value = window.localStorage.getItem(storageKey)
  return value === "light" || value === "system" || value === "dark"
    ? value
    : "dark"
}

function applyTheme(preference: ThemePreference) {
  const dark =
    preference === "dark" ||
    (preference === "system" && window.matchMedia(darkMediaQuery).matches)
  document.documentElement.classList.toggle("dark", dark)
  document.documentElement.style.colorScheme = dark ? "dark" : "light"
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(storedPreference)

  useEffect(() => {
    applyTheme(preference)
    window.localStorage.setItem(storageKey, preference)
    if (preference !== "system") return

    const media = window.matchMedia(darkMediaQuery)
    const update = () => applyTheme("system")
    media.addEventListener("change", update)
    return () => media.removeEventListener("change", update)
  }, [preference])

  const value = useMemo(
    () => ({ preference, setPreference: setPreferenceState }),
    [preference],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
