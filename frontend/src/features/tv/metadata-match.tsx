import { useState, type FormEvent } from "react"
import { LoaderCircle, Search } from "lucide-react"

import { searchProvider, type MediaDetail, type ProviderSearchResult } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/features/media/modal"
import { rematchTV } from "@/features/tv/api"

export function TVMetadataMatchButton({ detail, changed }: { detail: MediaDetail; changed: (value: MediaDetail) => void }) {
  const [open, setOpen] = useState(false)
  return <><Button type="button" variant="outline" size="sm" onClick={() => setOpen(true)}>Change TVDB match</Button>{open && <TVMetadataMatch detail={detail} changed={changed} close={() => setOpen(false)} />}</>
}

function TVMetadataMatch({ detail, changed, close }: { detail: MediaDetail; changed: (value: MediaDetail) => void; close: () => void }) {
  const [query, setQuery] = useState(detail.title)
  const [results, setResults] = useState<ProviderSearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [saving, setSaving] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function search(event: FormEvent) {
    event.preventDefault()
    if (query.trim().length < 2) return
    setSearching(true)
    setError(null)
    try { setResults((await searchProvider("tv", query.trim(), 1)).results) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "TVDB search failed.") }
    finally { setSearching(false) }
  }

  async function choose(result: ProviderSearchResult) {
    if (result.providerId === detail.providerId || (result.localId && result.localId !== detail.id)) return
    setSaving(result.providerId)
    setError(null)
    try {
      changed(await rematchTV(detail.id, result.providerId))
      close()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The TVDB match could not be changed.")
    } finally {
      setSaving(null)
    }
  }

  return (
    <Modal title="Change TVDB match" description="Choose which TVDB series supplies metadata for this Gradeium show. Your status, rating, date added, and episode progress stay attached to the same Gradeium entry." close={close} wide>
      <div className="space-y-4">
        <form className="flex gap-2" onSubmit={(event) => void search(event)}>
          <label className="relative min-w-0 flex-1">
            <span className="sr-only">Search TVDB</span>
            <Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" />
            <Input autoFocus className="pl-9" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search TVDB" />
          </label>
          <Button type="submit" disabled={searching || query.trim().length < 2}>{searching && <LoaderCircle className="animate-spin" />}Search</Button>
        </form>
        <p className="text-xs text-muted-foreground">Current TVDB ID: {detail.providerId}. Changing the match clears old artwork pins and refreshes metadata from the selected series.</p>
        <div className="max-h-[55vh] space-y-2 overflow-y-auto">
          {results.map((result) => {
            const current = result.providerId === detail.providerId
            const otherLocal = Boolean(result.localId && result.localId !== detail.id)
            return (
              <div key={result.providerId} className="grid grid-cols-[3.25rem_minmax(0,1fr)_auto] items-center gap-3 rounded-lg border p-2">
                <div className="aspect-[2/3] overflow-hidden rounded bg-muted">{result.artworkUrl && <img src={result.artworkUrl} alt="" className="size-full object-cover" />}</div>
                <div className="min-w-0"><p className="font-medium">{result.title}</p><p className="text-xs text-muted-foreground">{[result.year, result.network, `TVDB ${result.providerId}`].filter(Boolean).join(" · ")}</p></div>
                <Button type="button" size="sm" variant={current ? "secondary" : "outline"} disabled={current || otherLocal || saving !== null} onClick={() => void choose(result)}>
                  {saving === result.providerId ? <LoaderCircle className="animate-spin" /> : null}
                  {current ? "Current" : otherLocal ? "Already tracked" : "Use match"}
                </Button>
              </div>
            )
          })}
          {!searching && results.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">Search TVDB to choose a different series.</p>}
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  )
}
