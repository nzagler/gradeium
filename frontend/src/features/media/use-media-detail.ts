import { useCallback, useEffect, useState } from "react"
import { useParams } from "react-router-dom"

import { getMediaDetail, type MediaDetail, type MediaDomain } from "@/api/client"

export function useMediaDetail(domain: MediaDomain) {
  const { id = "" } = useParams()
  const [detail, setDetail] = useState<MediaDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setDetail(await getMediaDetail(domain, id))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The detail page could not be loaded.")
    } finally {
      setLoading(false)
    }
  }, [domain, id])

  useEffect(() => {
    let cancelled = false
    getMediaDetail(domain, id)
      .then((value) => {
        if (!cancelled) setDetail(value)
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : "The detail page could not be loaded.")
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [domain, id])

  return { id, detail, setDetail, loading, error, retry: load }
}
