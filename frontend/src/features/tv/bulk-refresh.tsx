import { useCallback, useEffect, useRef, useState } from "react"
import { LoaderCircle, RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { getTVBulkRefreshStatus, startTVBulkRefresh, type TVBulkRefreshJob } from "@/features/tv/api"

export function TVBulkRefreshControl({ completed }: { completed: () => void }) {
  const [job, setJob] = useState<TVBulkRefreshJob>({ state: "idle" })
  const [error, setError] = useState<string | null>(null)
  const notified = useRef(false)

  const apply = useCallback((next: TVBulkRefreshJob) => {
    setJob(next)
    if (next.state === "running") notified.current = false
    if (next.state === "completed" && !notified.current) {
      notified.current = true
      completed()
    }
  }, [completed])

  useEffect(() => {
    let active = true
    void getTVBulkRefreshStatus()
      .then((status) => { if (active) apply(status) })
      .catch(() => undefined)
    return () => { active = false }
  }, [apply])

  useEffect(() => {
    if (job.state !== "running") return
    let active = true
    const timer = window.setInterval(() => {
      void getTVBulkRefreshStatus()
        .then((status) => { if (active) apply(status) })
        .catch((cause: unknown) => {
          if (active) setError(cause instanceof Error ? cause.message : "Refresh status could not be loaded.")
        })
    }, 2000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [apply, job.state])

  async function start() {
    setError(null)
    try { apply(await startTVBulkRefresh()) }
    catch (cause) { setError(cause instanceof Error ? cause.message : "TV metadata refresh could not be started.") }
  }

  const running = job.state === "running"
  const result = job.result
  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      <Button type="button" variant="outline" disabled={running} onClick={() => void start()}>
        {running ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}
        {running ? "Refreshing TV metadata…" : "Refresh all TV metadata"}
      </Button>
      {job.state === "completed" && result && (
        <span role="status" className="text-xs text-muted-foreground">
          Refreshed {result.refreshed}/{result.total}{result.failed ? ` · ${result.failed} failed` : ""}.
        </span>
      )}
      {job.state === "failed" && <span role="alert" className="text-xs text-destructive">{job.message ?? "TV metadata refresh failed."}</span>}
      {error && <span role="alert" className="text-xs text-destructive">{error}</span>}
    </div>
  )
}
