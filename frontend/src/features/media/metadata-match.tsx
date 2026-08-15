import { useState, type FormEvent } from "react"
import { LoaderCircle, Search } from "lucide-react"

import { refreshMedia, searchProvider, type MediaDetail, type MediaDomain, type ProviderSearchResult } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/features/media/modal"

const providerLabels: Record<MediaDomain, string> = {
  games: "IGDB",
  movies: "TMDB",
  tv: "TVDB",
}

export function MetadataMatchManager({ domain, detail, changed, close }: { domain: MediaDomain; detail: MediaDetail; changed: (value: MediaDetail) => void; close: () => void }) {
  const provider = providerLabels[domain]
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
    try { setResults((await searchProvider(domain, query.trim(), 1)).results) }
    catch (cause) { setError(cause instanceof Error ? cause.message : `${provider} search failed.`) }
    finally { setSearching(false) }
  }

  async function choose(result: ProviderSearchResult) {
    if (result.providerId === detail.providerId || (result.localId && result.localId !== detail.id)) return
    setSaving(result.providerId)
    setError(null)
    try {
      changed(await refreshMedia(domain, detail.id, result.providerId))
      close()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `The ${provider} match could not be changed.`)
    } finally {
      setSaving(null)
    }
  }

  return (
    <Modal title={`Change ${provider} match`} description={`Choose which ${provider} item supplies metadata for this Gradeium entry. Your personal status, rating, date added, and supported progress stay attached to the same Gradeium item.`} close={close} wide>
      <div className="space-y-4">
        <form className="flex gap-2" onSubmit={(event) => void search(event)}>
          <label className="relative min-w-0 flex-1">
            <span className="sr-only">Search {provider}</span>
            <Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" />
            <Input autoFocus className="pl-9" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Search ${provider}`} />
          </label>
          <Button type="submit" disabled={searching || query.trim().length < 2}>{searching && <LoaderCircle className="animate-spin" />}Search</Button>
        </form>
        <p className="text-xs text-muted-foreground">Current {provider} ID: {detail.providerId}. Changing the match clears old artwork pins and refreshes metadata from the selected item.</p>
        <div className="max-h-[55vh] space-y-2 overflow-y-auto">
          {results.map((result) => {
            const current = result.providerId === detail.providerId
            const otherLocal = Boolean(result.localId && result.localId !== detail.id)
            return (
              <div key={result.providerId} className="grid grid-cols-[3.25rem_minmax(0,1fr)_auto] items-center gap-3 rounded-lg border p-2">
                <div className="aspect-[2/3] overflow-hidden rounded bg-muted">{result.artworkUrl && <img src={result.artworkUrl} alt="" className="size-full object-cover" />}</div>
                <div className="min-w-0"><p className="font-medium">{result.title}</p><p className="text-xs text-muted-foreground">{[result.year, result.developer ?? result.director ?? result.network, `${provider} ${result.providerId}`].filter(Boolean).join(" · ")}</p></div>
                <Button type="button" size="sm" variant="outline" disabled={current || otherLocal || saving !== null} onClick={() => void choose(result)}>
                  {saving === result.providerId ? <LoaderCircle className="animate-spin" /> : null}
                  {current ? "Current" : otherLocal ? "Already tracked" : "Use match"}
                </Button>
              </div>
            )
          })}
          {!searching && results.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">Search {provider} to choose a different metadata match.</p>}
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  )
}
