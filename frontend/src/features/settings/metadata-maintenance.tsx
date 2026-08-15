import { useCallback, useEffect, useState } from "react"
import { LoaderCircle, RefreshCw } from "lucide-react"

import { getMediaDetail, refreshMedia, type MediaDomain } from "@/api/client"
import { Button } from "@/components/ui/button"

export type MetadataRefreshResult = {
  total: number
  refreshed: number
  failed: number
  issues: { title: string; reason: string }[]
}

export type MetadataRefreshJob = {
  state: "idle" | "running" | "completed" | "failed"
  result?: MetadataRefreshResult
  message?: string
}

type RefreshDomain = { domain: MediaDomain; label: string; provider: string }

const domains: RefreshDomain[] = [
  { domain: "games", label: "Games", provider: "IGDB" },
  { domain: "movies", label: "Movies", provider: "TMDB" },
  { domain: "tv", label: "TV Shows", provider: "TVDB" },
]

const idleJobs: Record<MediaDomain, MetadataRefreshJob> = {
  games: { state: "idle" },
  movies: { state: "idle" },
  tv: { state: "idle" },
}

function status(domain: MediaDomain) {
  return getMediaDetail(domain, "all") as unknown as Promise<MetadataRefreshJob>
}

function start(domain: MediaDomain) {
  return refreshMedia(domain, "all") as unknown as Promise<MetadataRefreshJob>
}

export function MetadataMaintenance() {
  const [jobs, setJobs] = useState<Record<MediaDomain, MetadataRefreshJob>>(idleJobs)
  const [errors, setErrors] = useState<Partial<Record<MediaDomain, string>>>({})

  const refreshStatuses = useCallback(async () => {
    const settled = await Promise.allSettled(domains.map(({ domain }) => status(domain)))
    setJobs((current) => {
      const next = { ...current }
      settled.forEach((result, index) => {
        if (result.status === "fulfilled") next[domains[index].domain] = result.value
      })
      return next
    })
  }, [])

  useEffect(() => { void refreshStatuses() }, [refreshStatuses])
  useEffect(() => {
    if (!domains.some(({ domain }) => jobs[domain].state === "running")) return
    const timer = window.setInterval(() => void refreshStatuses(), 2000)
    return () => window.clearInterval(timer)
  }, [jobs, refreshStatuses])

  async function run(domain: MediaDomain) {
    setErrors((current) => ({ ...current, [domain]: undefined }))
    try {
      const job = await start(domain)
      setJobs((current) => ({ ...current, [domain]: job }))
    } catch (cause) {
      setErrors((current) => ({ ...current, [domain]: cause instanceof Error ? cause.message : "Metadata refresh could not be started." }))
    }
  }

  return (
    <section className="rounded-lg border bg-card shadow-xs">
      <div className="border-b px-5 py-4">
        <h2 className="font-semibold">Metadata maintenance</h2>
        <p className="mt-1 text-sm text-muted-foreground">Refresh provider metadata for every tracked item, including Backlog. These jobs continue on the server if you leave this page and do not change your ratings, statuses, or progress.</p>
      </div>
      <div className="divide-y">
        {domains.map(({ domain, label, provider }) => {
          const job = jobs[domain]
          const running = job.state === "running"
          return (
            <div key={domain} className="grid gap-3 p-5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <div>
                <div className="flex items-center gap-2"><h3 className="text-sm font-medium">{label}</h3><span className="text-xs text-muted-foreground">{provider}</span></div>
                {job.state === "idle" && <p className="mt-1 text-xs text-muted-foreground">No refresh has been run since Gradeium started.</p>}
                {running && <p className="mt-1 text-xs text-muted-foreground">Refreshing metadata in the background…</p>}
                {job.state === "completed" && job.result && <p className="mt-1 text-xs text-muted-foreground">Refreshed {job.result.refreshed} of {job.result.total}{job.result.failed ? ` · ${job.result.failed} failed` : ""}.</p>}
                {job.state === "failed" && <p className="mt-1 text-xs text-destructive">{job.message ?? "Metadata refresh failed."}</p>}
                {errors[domain] && <p role="alert" className="mt-1 text-xs text-destructive">{errors[domain]}</p>}
                {job.result && job.result.issues.length > 0 && <details className="mt-2 text-xs text-muted-foreground"><summary className="cursor-pointer">Show failed items</summary><ul className="mt-2 space-y-1">{job.result.issues.map((issue, index) => <li key={`${issue.title}-${index}`}>{issue.title}: {issue.reason}</li>)}</ul></details>}
              </div>
              <Button type="button" variant="outline" disabled={running} onClick={() => void run(domain)}>
                {running ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}
                {running ? "Refreshing…" : `Refresh all ${label.toLowerCase()}`}
              </Button>
            </div>
          )
        })}
      </div>
    </section>
  )
}
