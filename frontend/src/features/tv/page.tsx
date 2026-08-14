import { useState } from "react"
import { Check, ChevronDown, Circle } from "lucide-react"

import {
  setAllRegularWatched,
  setEpisodeWatched,
  setSeasonWatched,
  setThroughEpisode,
  type MediaDetail,
  type TVEpisode,
  type TVSeason,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { AddPage } from "@/features/media/add-page"
import { BackLink, DetailError, DetailHero, DetailLoading, InfoGrid, PeopleRow, Section } from "@/features/media/detail-shared"
import { formatDate, formatRuntime } from "@/features/media/format"
import { LibraryPage } from "@/features/media/library-page"
import { useMediaDetail } from "@/features/media/use-media-detail"

export function TVPage({ backlog = false }: { backlog?: boolean }) {
  return <LibraryPage domain="tv" title="TV Shows" backlog={backlog} />
}

export function AddTVPage() {
  return <AddPage domain="tv" title="TV Shows" />
}

export function TVDetailPage() {
  const { detail, setDetail, loading, error, retry } = useMediaDetail("tv")
  const [working, setWorking] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  if (loading) return <DetailLoading />
  if (error || !detail) return <DetailError message={error ?? "TV show not found."} retry={retry} />

  const showID = detail.id
  const changed = (value: MediaDetail) => setDetail(value)
  async function all(watched: boolean) {
    setWorking(true)
    setActionError(null)
    try {
      changed(await setAllRegularWatched(showID, watched))
    } catch (cause) {
      setActionError(cause instanceof Error ? cause.message : "Episode progress could not be changed.")
    } finally {
      setWorking(false)
    }
  }

  return (
    <div className="space-y-6">
      <BackLink domain="tv" label="TV Shows" />
      <DetailHero
        domain="tv"
        detail={detail}
        changed={changed}
        subtitle={[detail.year, detail.network, detail.genres.slice(0, 3).join(" · ")].filter(Boolean).join(" · ")}
      />
      <Section title="Progress">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0 flex-1">
            <div className="flex justify-between text-sm">
              <span>{detail.progress?.watched ?? 0} of {detail.progress?.total ?? 0} regular episodes</span>
              <span>{detail.progress?.percent ?? 0}%</span>
            </div>
            <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
              <div className="h-full bg-primary" style={{ width: `${detail.progress?.percent ?? 0}%` }} />
            </div>
            {detail.state.status === "in_progress" && detail.progress?.nextEpisode && (
              <p className="mt-2 text-sm text-muted-foreground">
                Next: S{detail.progress.nextEpisode.seasonNumber} E{detail.progress.nextEpisode.episodeNumber} · {detail.progress.nextEpisode.title}
              </p>
            )}
          </div>
          <div className="flex gap-2">
            <Button type="button" size="sm" variant="outline" disabled={working} onClick={() => void all(true)}>Mark all watched</Button>
            <Button type="button" size="sm" variant="outline" disabled={working} onClick={() => void all(false)}>Mark all unwatched</Button>
          </div>
        </div>
        {actionError && <p role="alert" className="mt-3 text-sm text-destructive">{actionError}</p>}
      </Section>
      <Section title="Overview">
        <p className="max-w-4xl text-sm leading-7 text-muted-foreground">{detail.overview || "No overview is available."}</p>
      </Section>
      <Section title="Show information">
        <InfoGrid items={[
          ["First aired", formatDate(detail.firstAired)],
          ["Series status", detail.providerStatus],
          ["Network", detail.network],
          ["Genres", detail.genres.join(", ")],
          ["Regular seasons", detail.seasons?.filter((season) => !season.special).length],
          ["Regular episodes", detail.progress?.total],
        ]} />
      </Section>
      <Section title="Seasons and episodes">
        <div className="space-y-3">
          {detail.seasons?.map((season) => <SeasonPanel key={season.id} showID={detail.id} season={season} changed={changed} />)}
        </div>
      </Section>
      {!!detail.cast?.length && <Section title="Cast"><PeopleRow people={detail.cast} /></Section>}
      {!!detail.keyPeople?.length && <Section title="Key people"><PeopleRow people={detail.keyPeople} /></Section>}
    </div>
  )
}

function SeasonPanel({ showID, season, changed }: { showID: string; season: TVSeason; changed: (value: MediaDetail) => void }) {
  const [open, setOpen] = useState(false)
  const [working, setWorking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const percent = season.total ? Math.round(season.watched * 100 / season.total) : 0

  async function seasonAction(watched: boolean) {
    setWorking(true)
    setError(null)
    try { changed(await setSeasonWatched(showID, season.number, watched)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Season progress could not be changed.") }
    finally { setWorking(false) }
  }
  async function episodeAction(episode: TVEpisode, watched: boolean) {
    setWorking(true)
    setError(null)
    try { changed(await setEpisodeWatched(showID, episode.id, watched)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Episode progress could not be changed.") }
    finally { setWorking(false) }
  }
  async function through(episode: TVEpisode) {
    setWorking(true)
    setError(null)
    try { changed(await setThroughEpisode(showID, episode.id)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Episode progress could not be changed.") }
    finally { setWorking(false) }
  }

  return (
    <div className="overflow-hidden rounded-lg border">
      <button type="button" className="flex w-full items-center gap-4 p-4 text-left" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <ChevronDown className={`size-4 transition ${open ? "rotate-180" : ""}`} />
        <div className="min-w-0 flex-1">
          <div className="flex justify-between gap-3">
            <span className="font-medium">{season.special ? "Specials" : season.name || `Season ${season.number}`}{season.airDate ? ` · ${season.airDate.slice(0, 4)}` : ""}</span>
            <span className="text-sm text-muted-foreground">{season.watched}/{season.total}</span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${percent}%` }} /></div>
          {season.special && <p className="mt-1 text-xs text-muted-foreground">Specials are tracked separately and do not affect overall progress.</p>}
        </div>
      </button>
      {open && (
        <div className="border-t">
          <div className="flex flex-wrap gap-2 border-b p-3">
            <Button type="button" size="sm" variant="outline" disabled={working} onClick={() => void seasonAction(true)}>Mark season watched</Button>
            <Button type="button" size="sm" variant="outline" disabled={working} onClick={() => void seasonAction(false)}>Mark season unwatched</Button>
          </div>
          {error && <p role="alert" className="border-b px-3 py-2 text-sm text-destructive">{error}</p>}
          <div className="divide-y">
            {season.episodes.map((episode) => (
              <article key={episode.id} className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-3">
                <button
                  type="button"
                  disabled={working}
                  aria-label={`${episode.watched ? "Mark unwatched" : "Mark watched"}: ${episode.title}`}
                  className="grid size-9 place-items-center rounded-md border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  onClick={() => void episodeAction(episode, !episode.watched)}
                >
                  {episode.watched ? <Check className="size-4" /> : <Circle className="size-4" />}
                </button>
                <div className="min-w-0">
                  <p className="text-sm font-medium">{season.special ? `Special ${episode.episodeNumber}` : `E${episode.episodeNumber}`} · {episode.title}</p>
                  <p className="text-xs text-muted-foreground">{[formatRuntime(episode.runtimeMinutes), formatDate(episode.airDate)].filter(Boolean).join(" · ")}</p>
                  {episode.overview && <details className="mt-1"><summary className="cursor-pointer text-xs text-muted-foreground underline">Overview</summary><p className="mt-2 text-xs leading-5 text-muted-foreground">{episode.overview}</p></details>}
                </div>
                {!season.special && <Button type="button" size="sm" variant="ghost" disabled={working} onClick={() => void through(episode)}>Mark through</Button>}
              </article>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
