import { useEffect, useState, type KeyboardEvent } from "react"
import { Laptop, LoaderCircle, Moon, Sun, type LucideIcon } from "lucide-react"

import {
  getLibraryPreferences,
  updateLibraryPreferences,
  type LibraryPreferences,
} from "@/api/client"
import { useTheme, type ThemePreference } from "@/features/theme/theme-context"
import { cn } from "@/lib/utils"

const choices: Array<{
  value: ThemePreference
  label: string
  description: string
  icon: LucideIcon
}> = [
  { value: "dark", label: "Dark", description: "Use Gradeium's restrained dark appearance.", icon: Moon },
  { value: "light", label: "Light", description: "Use the light appearance.", icon: Sun },
  { value: "system", label: "System", description: "Follow this device's appearance setting.", icon: Laptop },
]

export function AppearanceSettings() {
  const { preference, setPreference } = useTheme()
  const [preferences, setPreferences] = useState<LibraryPreferences | null>(null)
  const [saving, setSaving] = useState<ThemePreference | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    void getLibraryPreferences()
      .then((value) => {
        if (!active) return
        setPreferences(value)
        setPreference(value.theme)
      })
      .catch((cause: unknown) => {
        if (active) setError(cause instanceof Error ? cause.message : "Appearance could not be loaded.")
      })
    return () => { active = false }
  }, [setPreference])

  async function choose(theme: ThemePreference) {
    if (!preferences || saving) return
    setSaving(theme)
    setError(null)
    try {
      const saved = await updateLibraryPreferences({ ...preferences, theme })
      setPreferences(saved)
      setPreference(saved.theme)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Appearance could not be saved.")
    } finally {
      setSaving(null)
    }
  }

  function moveChoice(event: KeyboardEvent<HTMLButtonElement>, current: number) {
    let next = current
    if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (current + 1) % choices.length
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp") next = (current - 1 + choices.length) % choices.length
    else if (event.key === "Home") next = 0
    else if (event.key === "End") next = choices.length - 1
    else return
    event.preventDefault()
    const buttons = event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]')
    buttons?.[next]?.focus()
    void choose(choices[next].value)
  }

  return (
    <section className="rounded-lg border bg-card shadow-xs" aria-labelledby="appearance-heading">
      <div className="border-b px-5 py-4 sm:px-6">
        <h2 id="appearance-heading" className="font-semibold">Appearance</h2>
        <p className="mt-1 text-sm text-muted-foreground">Choose how Gradeium looks for your account. Dark is used when no preference has been saved.</p>
      </div>
      <div className="p-5 sm:p-6">
        {!preferences && !error && <div aria-label="Loading appearance" className="h-28 animate-pulse rounded-lg bg-muted" />}
        {preferences && (
          <div className="grid gap-3 sm:grid-cols-3" role="radiogroup" aria-label="Theme">
            {choices.map((choice, index) => {
              const Icon = choice.icon
              const selected = preference === choice.value
              return (
                <button
                  key={choice.value}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  tabIndex={selected ? 0 : -1}
                  disabled={saving !== null}
                  onClick={() => void choose(choice.value)}
                  onKeyDown={(event) => moveChoice(event, index)}
                  className={cn(
                    "min-h-32 rounded-lg border bg-background p-4 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60",
                    selected && "border-foreground ring-1 ring-foreground",
                  )}
                >
                  <span className="flex items-center justify-between gap-3">
                    <Icon aria-hidden="true" className="size-5" />
                    {saving === choice.value && <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />}
                  </span>
                  <span className="mt-4 block text-sm font-medium">{choice.label}</span>
                  <span className="mt-1 block text-xs leading-5 text-muted-foreground">{choice.description}</span>
                </button>
              )
            })}
          </div>
        )}
        {error && <p className="mt-4 text-sm text-destructive" role="alert">{error}</p>}
      </div>
    </section>
  )
}
