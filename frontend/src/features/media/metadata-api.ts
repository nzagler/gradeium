import { APIError, getAuthSession, type MediaDetail, type MediaDomain } from "@/api/client"

export async function rematchMedia(domain: MediaDomain, id: string, providerId: number): Promise<MediaDetail> {
  const session = await getAuthSession()
  const response = await fetch(
    `/api/${domain}/${encodeURIComponent(id)}/refresh?providerId=${encodeURIComponent(String(providerId))}`,
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
    throw new APIError(body.message ?? "The metadata match could not be changed.", response.status, body.error)
  }
  return body
}
