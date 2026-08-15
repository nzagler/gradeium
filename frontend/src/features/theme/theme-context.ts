import { createContext, useContext } from "react"

export type ThemePreference = "dark" | "light" | "system"

export type ThemeContextValue = {
  preference: ThemePreference
  setPreference: (preference: ThemePreference) => void
}

export const ThemeContext = createContext<ThemeContextValue | null>(null)

export function useTheme() {
  const value = useContext(ThemeContext)
  if (!value) throw new Error("useTheme must be used inside ThemeProvider")
  return value
}
