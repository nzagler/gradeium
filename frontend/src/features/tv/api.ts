import {
  APIError,
  getAuthSession,
  getMediaDetail,
  refreshMedia,
  type MediaDetail,
} from "@/api/client"

export type TVBulkRefreshResult = {
  total: number
  refreshed: number
  failed: number
  issues: { title: string; reason: string }[]
}

export type TVBulkRefreshJob = {
  state: "idle" | "running" | "completed" | "failed"
  result?: TVBulkRefreshResult
  message?: string
}

export async function startTVBulkRefresh() {
  return (await refreshMedia("tv", "all")) as unknown as TVBulkRefreshJob
}

export async function getTVBulkRefreshStatus() {
  return (await getMediaDetail("tv", "all")) as unknown as TVBulkRefreshJob
}

export async function rematchTV(id: string, providerId: number): Promise<MediaDetail> {
  const session = await getAuthSession()
  const response = await fetch(
    `/api/tv/${encodeURIComponent(id)}/refresh?providerId=${encodeURIComponent(String(providerId))}`,
    {
      method: "POST",
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        "X-CSRF-Token": session.csrfToken,
      },
    },
  )
  const body = await response.json().catch(() => ({})) as MediaDetail & { error?: string; message?: string }
  if (!response.ok) {
    throw new APIError(body.message ?? "The TVDB match could not be changed.", response.status, body.error)
  }
  return body
}
