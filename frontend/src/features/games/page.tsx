import { useState } from "react"
import { Link } from "react-router-dom"

import type { MediaDetail } from "@/api/client"
import { Button } from "@/components/ui/button"
import { AddPage } from "@/features/media/add-page"
import { BackLink, DetailError, DetailHero, DetailLoading, InfoGrid, Section } from "@/features/media/detail-shared"
import { useMediaDetail } from "@/features/media/use-media-detail"
import { formatDate, formatRating } from "@/features/media/format"
import { LibraryPage } from "@/features/media/library-page"
import { Modal } from "@/features/media/modal"

export function GamesPage({ backlog = false }: { backlog?: boolean }) { return <LibraryPage domain="games" title="Games" backlog={backlog} /> }
export function AddGamePage() { return <AddPage domain="games" title="Games" /> }

export function GameDetailPage() {
  const { detail, setDetail, loading, error, retry } = useMediaDetail("games")
  const [screenshot, setScreenshot] = useState<string | null>(null)
  if (loading) return <DetailLoading />
  if (error || !detail) return <DetailError message={error ?? "Game not found."} retry={retry} />
  const changed = (value: MediaDetail) => setDetail(value)
  const badge = detail.gameType && ["Remake", "Remaster"].includes(detail.gameType) ? [detail.gameType] : []
  return <div className="space-y-6"><BackLink domain="games" label="Games" /><DetailHero domain="games" detail={detail} changed={changed} badges={badge} subtitle={[detail.year,detail.developer,detail.genres.slice(0,3).join(" · ")].filter(Boolean).join(" · ")} />
    <Section title="Overview"><p className="max-w-4xl whitespace-pre-line text-sm leading-7 text-muted-foreground">{detail.summary || "No overview is available."}</p></Section>
    <Section title="Game information"><InfoGrid items={[["Release date",formatDate(detail.releaseDate)],["Developer",detail.developer],["Publisher",detail.publisher],["Genres",detail.genres.join(", ")],["Game modes",detail.gameModes?.join(", ")],["Platforms",detail.platforms?.join(", ")],["Franchise",detail.franchise]]} /></Section>
    {!!detail.screenshots?.length&&<Section title="Screenshots"><div className="flex snap-x gap-3 overflow-x-auto pb-2">{detail.screenshots.map((image)=><button key={image} type="button" className="w-72 shrink-0 snap-start overflow-hidden rounded-lg border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-[22rem]" onClick={()=>setScreenshot(image)}><img src={image} alt="Open screenshot" loading="lazy" className="aspect-video size-full object-cover" /></button>)}</div></Section>}
    {!!detail.additionalContent?.length&&<Section title="Additional content"><div className="divide-y">{detail.additionalContent.map((item)=><article key={item.providerId} className="flex items-center gap-4 py-3"><div className="h-16 w-12 overflow-hidden rounded bg-muted">{item.coverUrl&&<img src={item.coverUrl} alt="" className="size-full object-cover" />}</div><div><h3 className="text-sm font-medium">{item.title}</h3><p className="text-xs text-muted-foreground">{[item.type,item.year].filter(Boolean).join(" · ")} · Requires the base game</p></div></article>)}</div></Section>}
    {!!detail.relatedReleases?.length&&<Section title="Related releases"><RelatedGames detail={detail} /></Section>}
    {detail.franchise&&<Section title="Franchise"><p className="text-sm text-muted-foreground">{detail.franchise}</p></Section>}
    {!!detail.externalLinks?.length&&<Section title="External links"><div className="flex flex-wrap gap-2">{detail.externalLinks.map((link)=><Button key={link.url} asChild variant="outline" size="sm"><a href={link.url} target="_blank" rel="noreferrer">{link.label}</a></Button>)}</div></Section>}
    {screenshot&&<Modal title="Screenshot" close={()=>setScreenshot(null)} wide><img src={screenshot} alt="Game screenshot" className="w-full rounded-lg" /></Modal>}
  </div>
}

function RelatedGames({detail}:{detail:MediaDetail}){return <div className="grid gap-3 sm:grid-cols-2">{detail.relatedReleases?.map((item)=><article key={`${item.providerId}-${item.relationship}`} className="flex gap-3 rounded-md border p-3"><div className="h-20 w-14 overflow-hidden rounded bg-muted">{item.coverUrl&&<img src={item.coverUrl} alt="" className="size-full object-cover" />}</div><div className="min-w-0"><p className="text-xs capitalize text-muted-foreground">{item.relationship}</p><h3 className="truncate text-sm font-medium">{item.title}</h3><p className="text-xs text-muted-foreground">{item.year}</p>{item.localId?<Link className="mt-2 inline-block text-xs underline" to={`/games/${item.localId}`}>{item.localStatus}{item.localRating?` · ${formatRating(item.localRating)}`:""}</Link>:<Link className="mt-2 inline-block text-xs underline" to={`/games/add?q=${encodeURIComponent(item.title)}`}>Not in Library</Link>}</div></article>)}</div>}
