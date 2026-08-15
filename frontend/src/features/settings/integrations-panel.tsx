import { useEffect, useState, type FormEvent } from "react"
import { CircleAlert, CircleCheck, Download, LoaderCircle, PlugZap, RefreshCw } from "lucide-react"

import {
  configureIntegration,
  getIntegrations,
  getJellyfinLibraries,
  syncJellyfin,
  testIntegration,
  type IntegrationConfiguration,
  type IntegrationView,
  type JellyfinLibrary,
  type JellyfinSyncResult,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

const providerCopy = {
  igdb: { name: "IGDB", purpose: "Game search, metadata, artwork, and community user ratings.", secretLabel: "Twitch client secret" },
  tmdb: { name: "TMDB", purpose: "Movie metadata and verified TV community ratings.", secretLabel: "API Read Access Token" },
  tvdb: { name: "TVDB", purpose: "Authoritative TV series, season, episode, cast, and artwork metadata.", secretLabel: "TVDB API key" },
  jellyfin: { name: "Jellyfin", purpose: "Manual, add-only movie and TV imports using canonical provider IDs.", secretLabel: "Jellyfin API key" },
} as const

export function IntegrationsPanel() {
  const [values, setValues] = useState<IntegrationView[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      setValues((await getIntegrations()).integrations)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Integrations could not be loaded.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let active = true
    void getIntegrations()
      .then((response) => { if (active) setValues(response.integrations) })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : "Integrations could not be loaded.") })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  if (loading) return <div className="h-48 animate-pulse rounded-lg bg-muted" />
  if (error) return <div className="rounded-lg border bg-card p-6"><p className="text-sm">{error}</p><Button className="mt-3" variant="outline" onClick={() => void load()}>Try again</Button></div>
  return <div className="space-y-4">{values.map((value) => <ProviderCard key={value.provider} value={value} changed={(next) => setValues((current) => current.map((item) => item.provider === next.provider ? next : item))} />)}</div>
}

function ProviderCard({ value, changed }: { value: IntegrationView; changed: (value: IntegrationView) => void }) {
  const copy = providerCopy[value.provider]
  const [input, setInput] = useState<IntegrationConfiguration>({
    enabled: value.enabled,
    clientId: value.clientId ?? "",
    secret: "",
    removeSecret: false,
    pin: "",
    removePin: false,
    baseUrl: value.baseUrl ?? "",
    libraryMappings: value.libraryMappings ?? [],
  })
  const [libraries, setLibraries] = useState<JellyfinLibrary[]>([])
  const [syncResult, setSyncResult] = useState<JellyfinSyncResult | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  async function save(event: FormEvent) {
    event.preventDefault()
    setSaving(true)
    setMessage(null)
    setFailed(false)
    try {
      const next = await configureIntegration(value.provider, input)
      changed(next)
      setInput({ ...input, baseUrl: next.baseUrl ?? input.baseUrl, secret: "", pin: "", removeSecret: false, removePin: false })
      setMessage("Configuration saved. Test the connection before using this provider.")
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "Configuration could not be saved.")
      setFailed(true)
    } finally {
      setSaving(false)
    }
  }

  async function test() {
    setTesting(true)
    setMessage(null)
    setFailed(false)
    try {
      const result = await testIntegration(value.provider)
      setMessage(result.message)
      changed({ ...value, state: "connected", lastTest: { provider: value.provider, ...result } })
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "Connection test failed.")
      setFailed(true)
      changed({ ...value, state: "error" })
    } finally {
      setTesting(false)
    }
  }

  async function discover() {
    setDiscovering(true)
    setMessage(null)
    setFailed(false)
    try {
      const response = await getJellyfinLibraries()
      setLibraries(response.libraries)
      setInput((current) => {
        const mappings = [...current.libraryMappings]
        for (const library of response.libraries) {
          if (mappings.some((mapping) => mapping.libraryId === library.id)) continue
          const collectionType = library.collectionType?.toLowerCase()
          const domain = library.domain ?? (collectionType === "movies" ? "movies" : collectionType === "tvshows" ? "tv" : undefined)
          if (domain) mappings.push({ libraryId: library.id, domain })
        }
        return { ...current, libraryMappings: mappings }
      })
      setMessage(`Discovered ${response.libraries.length} Jellyfin ${response.libraries.length === 1 ? "library" : "libraries"}. Choose mappings, then save configuration.`)
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "Jellyfin libraries could not be loaded.")
      setFailed(true)
    } finally {
      setDiscovering(false)
    }
  }

  async function importNow() {
    setSyncing(true)
    setSyncResult(null)
    setMessage(null)
    setFailed(false)
    try {
      const result = await syncJellyfin()
      setSyncResult(result)
      setMessage(`Scanned ${result.scanned}; added ${result.moviesAdded} movies and ${result.tvShowsAdded} TV shows.`)
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "Jellyfin import could not be completed.")
      setFailed(true)
    } finally {
      setSyncing(false)
    }
  }

  function mapLibrary(libraryId: string, domain: "" | "movies" | "tv") {
    setInput((current) => ({
      ...current,
      libraryMappings: domain
        ? [...current.libraryMappings.filter((mapping) => mapping.libraryId !== libraryId), { libraryId, domain }]
        : current.libraryMappings.filter((mapping) => mapping.libraryId !== libraryId),
    }))
  }

  const stateLabel = value.state.replaceAll("_", " ")
  const availableLibraries: JellyfinLibrary[] = libraries.length > 0 ? libraries : (value.libraryMappings ?? []).map((mapping) => ({ id: mapping.libraryId, name: mapping.libraryId, domain: mapping.domain }))

  return (
    <section className="rounded-lg border bg-card shadow-xs">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b px-5 py-4">
        <div><div className="flex items-center gap-2"><PlugZap className="size-4" /><h2 className="font-semibold">{copy.name}</h2></div><p className="mt-1 text-sm text-muted-foreground">{copy.purpose}</p></div>
        <span className={`rounded-full border px-2.5 py-1 text-xs font-medium capitalize ${value.state === "connected" ? "border-emerald-300 text-emerald-700 dark:text-emerald-300" : value.state === "error" ? "border-red-300 text-destructive" : "text-muted-foreground"}`}>{stateLabel}</span>
      </div>
      <form className="space-y-4 p-5" onSubmit={(event) => void save(event)}>
        <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={input.enabled} onChange={(event) => setInput({ ...input, enabled: event.target.checked })} className="size-4" />Enable {copy.name}</label>
        {value.provider === "igdb" && <div><Label htmlFor="igdb-client-id">Twitch client ID</Label><Input id="igdb-client-id" className="mt-2" value={input.clientId} onChange={(event) => setInput({ ...input, clientId: event.target.value })} autoComplete="off" /></div>}
        {value.provider === "jellyfin" && <div><Label htmlFor="jellyfin-base-url">Jellyfin server URL</Label><Input id="jellyfin-base-url" className="mt-2" type="url" value={input.baseUrl} onChange={(event) => setInput({ ...input, baseUrl: event.target.value })} placeholder="http://jellyfin.local:8096" autoComplete="url" /><p className="mt-1 text-xs text-muted-foreground">HTTP and HTTPS are supported. URLs containing credentials are rejected.</p></div>}
        <div>
          <Label htmlFor={`${value.provider}-secret`}>{copy.secretLabel}</Label>
          <Input id={`${value.provider}-secret`} className="mt-2" type="password" value={input.secret} onChange={(event) => setInput({ ...input, secret: event.target.value, removeSecret: false })} autoComplete="new-password" placeholder={value.secretConfigured ? "Saved — enter a replacement" : "Not configured"} />
          <p className="mt-1 text-xs text-muted-foreground">Saved values are encrypted and never displayed again.</p>
          {value.secretConfigured && <label className="mt-2 flex items-center gap-2 text-xs text-muted-foreground"><input type="checkbox" checked={input.removeSecret} onChange={(event) => setInput({ ...input, removeSecret: event.target.checked, secret: event.target.checked ? "" : input.secret })} />Remove saved credential when saving</label>}
        </div>
        {value.provider === "tvdb" && <div><Label htmlFor="tvdb-pin">Subscriber PIN <span className="font-normal text-muted-foreground">(optional)</span></Label><Input id="tvdb-pin" className="mt-2" type="password" value={input.pin} onChange={(event) => setInput({ ...input, pin: event.target.value, removePin: false })} autoComplete="new-password" placeholder={value.pinConfigured ? "Saved — enter a replacement" : "Only for a user-supported key"} />{value.pinConfigured && <label className="mt-2 flex items-center gap-2 text-xs text-muted-foreground"><input type="checkbox" checked={input.removePin} onChange={(event) => setInput({ ...input, removePin: event.target.checked, pin: event.target.checked ? "" : input.pin })} />Remove saved PIN when saving</label>}</div>}

        {value.provider === "jellyfin" && (
          <div className="space-y-3 rounded-md border p-4">
            <div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="text-sm font-medium">Library mapping</h3><p className="text-xs text-muted-foreground">Ignored libraries are not imported. No title or year matching is used.</p></div><Button type="button" size="sm" variant="outline" disabled={discovering || !value.configured} onClick={() => void discover()}>{discovering ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}{discovering ? "Discovering…" : "Discover libraries"}</Button></div>
            {availableLibraries.map((library) => {
              const domain = input.libraryMappings.find((mapping) => mapping.libraryId === library.id)?.domain ?? ""
              return <label key={library.id} className="grid gap-2 border-t pt-3 text-sm sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center"><span><span className="font-medium">{library.name}</span>{library.collectionType && <span className="ml-2 text-xs text-muted-foreground">{library.collectionType}</span>}</span><select aria-label={`Map ${library.name}`} className="h-9 rounded-md border bg-background px-3" value={domain} onChange={(event) => mapLibrary(library.id, event.target.value as "" | "movies" | "tv")}><option value="">Ignore</option><option value="movies">Movies</option><option value="tv">TV Shows</option></select></label>
            })}
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" disabled={saving}>{saving && <LoaderCircle className="animate-spin" />}{saving ? "Saving…" : "Save configuration"}</Button>
          <Button type="button" variant="outline" disabled={testing || !value.configured} onClick={() => void test()}>{testing && <LoaderCircle className="animate-spin" />}{testing ? "Testing…" : "Test connection"}</Button>
          {value.provider === "jellyfin" && <Button type="button" variant="outline" disabled={syncing || !value.enabled || !value.configured || input.libraryMappings.length === 0} onClick={() => void importNow()}>{syncing ? <LoaderCircle className="animate-spin" /> : <Download />}{syncing ? "Importing…" : syncResult ? "Sync now" : "Import now"}</Button>}
          {message && <span role={failed ? "alert" : "status"} className={`inline-flex items-center gap-1 text-sm ${failed ? "text-destructive" : "text-muted-foreground"}`}>{failed ? <CircleAlert className="size-4" /> : <CircleCheck className="size-4" />}{message}</span>}
        </div>
        {syncResult && <div className="rounded-md bg-muted p-3 text-xs text-muted-foreground"><p>{syncResult.alreadyPresent} already tracked · {syncResult.skipped} skipped · {syncResult.failed} failed</p>{syncResult.issues.length > 0 && <ul className="mt-2 list-disc space-y-1 pl-5">{syncResult.issues.slice(0, 20).map((issue, index) => <li key={`${issue.libraryId ?? "item"}-${issue.title ?? "unknown"}-${index}`}>{issue.title ? `${issue.title}: ` : ""}{issue.reason}</li>)}</ul>}</div>}
        {value.lastTest && <p className="text-xs text-muted-foreground">Last tested {new Date(value.lastTest.testedAt).toLocaleString()} · {value.lastTest.message}</p>}
      </form>
    </section>
  )
}
