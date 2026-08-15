import { useCallback, useEffect, useState } from "react"
import { Film, Gamepad2, Star, Tv } from "lucide-react"
import { Link, useSearchParams } from "react-router-dom"

import {
  getDashboard,
  type DashboardItem,
  type DashboardResponse,
  type DashboardScope,
  type MediaDomain,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { statusLabels } from "@/features/media/format"
import { formatPersonalRating, ratingDistributionLabel } from "@/features/media/rating-scale"
import { useRatingScale } from "@/features/media/rating-scale-context"
import { cn } from "@/lib/utils"

const scopes: [DashboardScope, string][] = [
  ["all", "All"],
  ["games", "Games"],
  ["movies", "Movies"],
  ["tv", "TV"],
]

const domainLabels: Record<MediaDomain, string> = {
  games: "Games",
  movies: "Movies",
  tv: "TV Shows",
}

export function DashboardPage() {
  const [params, setParams] = useSearchParams()
  const requested = params.get("scope")
  const scope = scopes.some(([value]) => value === requested)
    ? (requested as DashboardScope)
    : "all"
  const [value, setValue] = useState<DashboardResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setValue(await getDashboard(scope))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The Dashboard could not be loaded.")
    } finally {
      setLoading(false)
    }
  }, [scope])

  useEffect(() => {
    let active = true
    void getDashboard(scope)
      .then((response) => { if (active) setValue(response) })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : "The Dashboard could not be loaded.") })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [scope])

  function changeScope(nextScope: DashboardScope) {
    setLoading(true)
    setError(null)
    const next = new URLSearchParams(params)
    if (nextScope === "all") next.delete("scope")
    else next.set("scope", nextScope)
    setParams(next, { replace: true })
  }

  return (
    <section className="space-y-7" aria-labelledby="dashboard-title">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 id="dashboard-title" className="text-2xl font-semibold tracking-tight">
            Dashboard
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            A current-state view of your persisted Gradeium library.
          </p>
        </div>
        <div className="inline-flex w-fit rounded-md border bg-card p-1" aria-label="Dashboard scope">
          {scopes.map(([key, label]) => (
            <button
              key={key}
              type="button"
              aria-pressed={scope === key}
              onClick={() => changeScope(key)}
              className={cn(
                "min-h-9 rounded px-3 text-sm font-medium text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                scope === key && "bg-primary text-primary-foreground",
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </header>

      {loading && <DashboardLoading />}
      {error && (
        <div className="rounded-lg border bg-card p-6">
          <h2 className="font-semibold">Dashboard unavailable</h2>
          <p className="mt-1 text-sm text-muted-foreground">{error}</p>
          <Button className="mt-4" type="button" variant="outline" onClick={() => void load()}>
            Try again
          </Button>
        </div>
      )}
      {!loading && !error && value && <DashboardContent value={value} />}
    </section>
  )
}

function DashboardContent({ value }: { value: DashboardResponse }) {
  const ratingScale = useRatingScale()
  const visibleDomains = (Object.keys(value.totals) as MediaDomain[]).filter(
    (domain) => value.scope === "all" || value.scope === domain,
  )
  const tracked = visibleDomains.reduce((sum, domain) => sum + value.totals[domain].tracked, 0)
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {visibleDomains.map((domain) => (
          <TotalCard key={domain} domain={domain} totals={value.totals[domain]} />
        ))}
        <div className="rounded-lg border bg-card p-5 shadow-xs">
          <p className="text-sm font-medium text-muted-foreground">Average personal rating</p>
          <p className="mt-2 text-3xl font-semibold tabular-nums">
            {value.averageRating === undefined ? "—" : formatPersonalRating(value.averageRating, ratingScale)}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">Rated Library items only</p>
        </div>
      </div>

      {tracked === 0 ? (
        <div className="rounded-lg border bg-card px-6 py-14 text-center">
          <h2 className="font-semibold">Nothing tracked in this scope yet.</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Add a Game, Movie, or TV Show to make the Dashboard useful.
          </p>
        </div>
      ) : (
        <>
          <MediaSection title="Currently In Progress" items={value.inProgress} empty="Nothing is currently In Progress." />
          <div className="grid gap-5 xl:grid-cols-2">
            <DistributionCard title="Personal rating distribution" values={value.ratingDistribution} personalRating />
            <DistributionCard title="Status distribution" values={value.statusDistribution} />
          </div>
          <MediaSection title="Highest Rated" items={value.highestRated} empty="No rated Library items in this scope." showRating />
          {value.tvProgress.length > 0 && (
            <section className="rounded-lg border bg-card p-5 shadow-xs" aria-labelledby="tv-progress-title">
              <h2 id="tv-progress-title" className="font-semibold">TV progress</h2>
              <p className="mt-1 text-sm text-muted-foreground">Regular episodes only; Specials are excluded.</p>
              <div className="mt-5 grid gap-4 sm:grid-cols-2">
                {value.tvProgress.map((item) => (
                  <Link key={item.id} to={`/tv/${item.id}`} className="rounded-md border p-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                    <div className="flex items-center justify-between gap-3">
                      <span className="truncate font-medium">{item.title}</span>
                      <span className="text-sm tabular-nums text-muted-foreground">{item.watched} / {item.total}</span>
                    </div>
                    <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={`${item.title} regular episode progress`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={item.percent ?? 0}>
                      <div className="h-full bg-primary" style={{ width: `${item.percent ?? 0}%` }} />
                    </div>
                    {item.nextEpisode && <p className="mt-2 truncate text-xs text-muted-foreground">Next: {item.nextEpisode}</p>}
                  </Link>
                ))}
              </div>
            </section>
          )}
        </>
      )}
    </>
  )
}

function TotalCard({ domain, totals }: { domain: MediaDomain; totals: { tracked: number; library: number; backlog: number } }) {
  const Icon = domain === "games" ? Gamepad2 : domain === "movies" ? Film : Tv
  return (
    <div className="rounded-lg border bg-card p-5 shadow-xs">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium text-muted-foreground">{domainLabels[domain]}</p>
        <Icon aria-hidden="true" className="size-4 text-muted-foreground" />
      </div>
      <p className="mt-2 text-3xl font-semibold tabular-nums">{totals.tracked}</p>
      <p className="mt-1 text-xs text-muted-foreground">{totals.library} Library · {totals.backlog} Backlog</p>
    </div>
  )
}

function MediaSection({ title, items, empty, showRating = false }: { title: string; items: DashboardItem[]; empty: string; showRating?: boolean }) {
  const ratingScale = useRatingScale()
  return (
    <section aria-label={title}>
      <div className="mb-4 flex items-end justify-between gap-3">
        <h2 className="font-semibold">{title}</h2>
        <span className="text-xs text-muted-foreground">{items.length} {items.length === 1 ? "item" : "items"}</span>
      </div>
      {items.length === 0 ? <p className="rounded-lg border bg-card p-5 text-sm text-muted-foreground">{empty}</p> : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
          {items.map((item) => (
            <Link key={`${item.domain}:${item.id}`} to={`/${item.domain}/${item.id}`} className="group min-w-0 rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              <div className="aspect-[2/3] overflow-hidden rounded-lg border bg-muted shadow-xs">
                {item.artworkUrl ? <img src={item.artworkUrl} alt="" className="size-full object-cover transition-transform group-hover:scale-[1.02]" /> : <div className="grid size-full place-items-center text-xs text-muted-foreground">No artwork</div>}
              </div>
              <p className="mt-2 line-clamp-2 text-sm font-medium leading-5">{item.title}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {showRating && item.rating !== undefined ? <span className="inline-flex items-center gap-1"><Star aria-hidden="true" className="size-3" />{formatPersonalRating(item.rating, ratingScale)}</span> : statusLabels[item.status]}
              </p>
            </Link>
          ))}
        </div>
      )}
    </section>
  )
}

function DistributionCard({ title, values, personalRating = false }: { title: string; values: { key: string; label: string; count: number }[]; personalRating?: boolean }) {
  const ratingScale = useRatingScale()
  const maximum = Math.max(1, ...values.map((value) => value.count))
  return (
    <section className="rounded-lg border bg-card p-5 shadow-xs" aria-label={title}>
      <h2 className="font-semibold">{title}</h2>
      {values.length === 0 ? <p className="mt-4 text-sm text-muted-foreground">No data in this scope.</p> : (
        <dl className="mt-5 space-y-3">
          {values.map((value) => (
            <div key={value.key} className="grid grid-cols-[6rem_minmax(0,1fr)_2rem] items-center gap-3">
              <dt className="truncate text-xs text-muted-foreground">{personalRating ? ratingDistributionLabel(value.key, ratingScale) : value.label}</dt>
              <dd className="h-2 overflow-hidden rounded-full bg-muted">
                <div className="h-full rounded-full bg-primary" style={{ width: `${Math.max(4, value.count / maximum * 100)}%` }} />
              </dd>
              <dd className="text-right text-xs tabular-nums text-muted-foreground">{value.count}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  )
}

function DashboardLoading() {
  return <div aria-label="Loading Dashboard" className="space-y-6"><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 4 }, (_, index) => <div key={index} className="h-32 animate-pulse rounded-lg bg-muted" />)}</div><div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">{Array.from({ length: 6 }, (_, index) => <div key={index} className="aspect-[2/3] animate-pulse rounded-lg bg-muted" />)}</div></div>
}
