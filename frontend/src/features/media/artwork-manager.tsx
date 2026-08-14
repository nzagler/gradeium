import { useMemo, useState } from "react"
import { Check, LoaderCircle } from "lucide-react"

import { selectMediaArtwork, type MediaDetail, type MediaDomain } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Modal } from "@/features/media/modal"

export function ArtworkManager({ domain, detail, close, changed }: { domain: MediaDomain; detail: MediaDetail; close: () => void; changed: (detail: MediaDetail) => void }) {
  const groups = useMemo(() => [...new Set(detail.artworks.filter((item) => item.available).map((item) => item.kind))], [detail.artworks])
  const [kind, setKind] = useState(groups[0] ?? "poster")
  const [saving, setSaving] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const options = detail.artworks.filter((item) => item.kind === kind && item.available)
  async function choose(providerImageId: string) {
    setSaving(providerImageId || "default"); setError(null)
    try { changed(await selectMediaArtwork(domain, detail.id, kind, providerImageId)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Artwork could not be selected.") }
    finally { setSaving(null) }
  }
  return <Modal title="Manage artwork" description="Choose only from images supplied by the metadata provider." close={close} wide><div className="space-y-5">{detail.unavailablePins.length>0&&<p className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">A previously pinned {detail.unavailablePins.join(", ")} image is no longer supplied. Gradeium is showing the provider default.</p>}<div role="tablist" aria-label="Artwork type" className="flex gap-1 border-b">{groups.map((value)=><button key={value} type="button" role="tab" aria-selected={kind===value} className={`border-b-2 px-4 py-2 text-sm font-medium capitalize ${kind===value?"border-foreground":"border-transparent text-muted-foreground"}`} onClick={()=>setKind(value)}>{value}</button>)}</div>{groups.length===0?<p className="text-sm text-muted-foreground">This provider did not supply customizable artwork.</p>:<><div className="flex justify-between gap-3"><div><h3 className="text-sm font-medium capitalize">{kind}</h3><p className="text-xs text-muted-foreground">{detail.artworkPins[kind]?"A provider image is pinned.":"Using the provider default."}</p></div>{detail.artworkPins[kind]&&<Button type="button" variant="outline" size="sm" disabled={saving!==null} onClick={()=>void choose("")}>Use provider default</Button>}</div><div className={`grid gap-3 ${kind==="backdrop"?"grid-cols-1 sm:grid-cols-2":"grid-cols-2 sm:grid-cols-3 md:grid-cols-4"}`}>{options.map((item)=>{const selected=detail.artworkPins[kind]===item.providerImageId||(!detail.artworkPins[kind]&&item.preferred);return <button key={item.providerImageId} type="button" disabled={saving!==null} className={`relative overflow-hidden rounded-lg border-2 bg-muted text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected?"border-foreground":"border-transparent"}`} onClick={()=>void choose(item.providerImageId)}><img src={item.thumbnailUrl} alt={`${kind} option`} loading="lazy" className={`w-full object-cover ${kind==="backdrop"?"aspect-video":"aspect-[2/3]"}`} />{selected&&<span className="absolute right-2 top-2 grid size-7 place-items-center rounded-full bg-background shadow"><Check className="size-4" /></span>}{saving===item.providerImageId&&<span className="absolute inset-0 grid place-items-center bg-black/50 text-white"><LoaderCircle className="animate-spin" /></span>}</button>})}</div></>}{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}</div></Modal>
}
