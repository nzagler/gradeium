import { useEffect, useState, type FormEvent } from "react"
import { FileDown, LoaderCircle } from "lucide-react"

import {
  downloadRatingsCSV,
  getLibraryPreferences,
  updateLibraryPreferences,
  type LibraryPreferences,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { sortOptions } from "@/features/media/format"
import { ratingScaleOptions } from "@/features/media/rating-scale"
import { MetadataMaintenance } from "@/features/settings/metadata-maintenance"

export function LibrarySettings() {
  const [value, setValue] = useState<LibraryPreferences | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let active = true
    void getLibraryPreferences()
      .then((preferences) => {
        if (active) setValue(preferences)
      })
      .catch((cause: unknown) => {
        if (active) {
          setError(cause instanceof Error ? cause.message : "Preferences could not be loaded.")
        }
      })
    return () => { active = false }
  }, [])

  async function save(event: FormEvent) {
    event.preventDefault()
    if (!value) return
    setSaving(true)
    setError(null)
    setMessage(null)
    try {
      const saved = await updateLibraryPreferences(value)
      setValue(saved)
      window.dispatchEvent(new CustomEvent("gradeium:rating-scale", { detail: saved.ratingScale }))
      setMessage("Library defaults saved.")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Preferences could not be saved.")
    } finally {
      setSaving(false)
    }
  }

  if (!value && !error) {
    return <div aria-label="Loading Library settings" className="h-40 animate-pulse rounded-lg bg-muted" />
  }

  return (
    <div className="space-y-6">
      <section className="rounded-lg border bg-card shadow-xs">
        <div className="border-b px-5 py-4">
          <h2 className="font-semibold">Library defaults</h2>
          <p className="mt-1 text-sm text-muted-foreground">Applied when a Library view has no explicit URL sort or view selection.</p>
        </div>
        {value && (
          <form className="space-y-5 p-5" onSubmit={(event) => void save(event)}>
            <div>
              <label htmlFor="default-library-sort" className="text-sm font-medium">Default sort</label>
              <select id="default-library-sort" className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={value.defaultLibrarySort} onChange={(event) => setValue({ ...value, defaultLibrarySort: event.target.value })}>
                {sortOptions.map(([key, label]) => <option key={key} value={key}>{label}</option>)}
              </select>
            </div>
            <div>
              <label htmlFor="personal-rating-scale" className="text-sm font-medium">Personal rating scale</label>
              <select id="personal-rating-scale" className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={value.ratingScale} onChange={(event) => setValue({ ...value, ratingScale: event.target.value as LibraryPreferences["ratingScale"] })}>
                {ratingScaleOptions.map((option) => <option key={option.value} value={option.value}>{option.label} (for example {option.example})</option>)}
              </select>
              <p className="mt-1 text-xs text-muted-foreground">Changing the scale reinterprets display only. Stored ratings and sorting remain unchanged.</p>
            </div>
            <div>
              <label htmlFor="preferred-library-view" className="text-sm font-medium">Preferred view</label>
              <select id="preferred-library-view" className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={value.preferredView} onChange={(event) => setValue({ ...value, preferredView: event.target.value as "grid" | "list" })}>
                <option value="grid">Artwork grid</option>
                <option value="list">Compact list</option>
              </select>
            </div>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {message && <p role="status" className="text-sm text-muted-foreground">{message}</p>}
            <Button type="submit" disabled={saving}>
              {saving && <LoaderCircle aria-hidden="true" className="animate-spin" />}
              {saving ? "Saving…" : "Save defaults"}
            </Button>
          </form>
        )}
        {!value && error && <p role="alert" className="p-5 text-sm text-destructive">{error}</p>}
      </section>

      <MetadataMaintenance />

      <section className="rounded-lg border bg-card p-5 shadow-xs">
        <h2 className="font-semibold">Ratings CSV</h2>
        <p className="mt-1 text-sm text-muted-foreground">Download a UTF-8 CSV with canonical 0–100 values, ratings on your selected display scale, and rating reasons.</p>
        <Button
          className="mt-4"
          type="button"
          variant="outline"
          onClick={() => void downloadRatingsCSV().catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "CSV export failed."))}
        >
          <FileDown aria-hidden="true" />
          Download ratings CSV
        </Button>
      </section>
    </div>
  )
}
