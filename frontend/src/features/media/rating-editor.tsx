import { useState, type FormEvent } from "react"
import { LoaderCircle, Minus, Plus, Star } from "lucide-react"

import type { MediaDomain, PersonalState } from "@/api/client"
import { updateMediaState } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Modal } from "@/features/media/modal"
import { formatRating } from "@/features/media/format"

export function RatingButton({
  domain,
  id,
  title,
  state,
  saved,
  className,
}: {
  domain: MediaDomain
  id: string
  title: string
  state: PersonalState
  saved: (state: PersonalState) => void
  className?: string
}) {
  const [open, setOpen] = useState(false)
  if (state.status === "backlog") return null
  return (
    <>
      <Button className={className} type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
        <Star aria-hidden="true" className={state.rating ? "fill-current" : ""} />
        {state.rating ? formatRating(state.rating) : "Rate"}
      </Button>
      {open && (
        <RatingEditor
          domain={domain}
          id={id}
          title={title}
          state={state}
          close={() => setOpen(false)}
          saved={(value) => { saved(value); setOpen(false) }}
        />
      )}
    </>
  )
}

function RatingEditor({ domain, id, title, state, close, saved }: {
  domain: MediaDomain; id: string; title: string; state: PersonalState
  close: () => void; saved: (state: PersonalState) => void
}) {
  const [rating, setRating] = useState(state.rating ?? 50)
  const [reason, setReason] = useState(state.ratingReason ?? "")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  async function submit(event: FormEvent) {
    event.preventDefault(); setSaving(true); setError(null)
    try {
      const value = await updateMediaState(domain, id, {
        status: state.status,
        rating,
        ratingReason: reason.trim() || undefined,
      })
      saved({ ...state, ...value })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The rating could not be saved.")
      setSaving(false)
    }
  }
  async function clearRating() {
    setSaving(true); setError(null)
    try {
      const value = await updateMediaState(domain, id, { status: state.status })
      saved({ ...state, ...value, rating: undefined, ratingReason: undefined })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The rating could not be cleared.")
      setSaving(false)
    }
  }
  return (
    <Modal title={`Rate ${title}`} description="Your rating is private and uses exact 0.1 increments." close={close}>
      <form className="space-y-5" onSubmit={(event) => void submit(event)}>
        <div>
          <label htmlFor="rating-value" className="text-sm font-medium">My rating</label>
          <div className="mt-2 flex items-center gap-3">
            <Button type="button" variant="outline" size="icon" aria-label="Decrease rating" onClick={() => setRating((value) => Math.max(10, value - 1))}><Minus /></Button>
            <input id="rating-value" className="h-10 w-24 rounded-md border bg-background px-3 text-center text-lg font-semibold" type="number" min="1" max="10" step="0.1" value={formatRating(rating)} onChange={(event) => { const value=Number(event.target.value); if(Number.isFinite(value)) setRating(Math.max(10, Math.min(100, Math.round(value * 10)))) }} />
            <Button type="button" variant="outline" size="icon" aria-label="Increase rating" onClick={() => setRating((value) => Math.min(100, value + 1))}><Plus /></Button>
            <span className="text-sm text-muted-foreground">out of 10</span>
          </div>
        </div>
        <div>
          <label htmlFor="rating-reason" className="text-sm font-medium">Reason <span className="font-normal text-muted-foreground">(optional)</span></label>
          <textarea id="rating-reason" maxLength={4000} rows={4} value={reason} onChange={(event) => setReason(event.target.value)} className="mt-2 w-full resize-y rounded-md border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" />
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <div className="flex flex-wrap justify-end gap-2">{state.rating&&<Button type="button" variant="ghost" disabled={saving} onClick={()=>void clearRating()}>Clear rating</Button>}<Button type="button" variant="outline" onClick={close}>Cancel</Button><Button type="submit" disabled={saving}>{saving && <LoaderCircle className="animate-spin" />}{saving ? "Saving…" : "Save rating"}</Button></div>
      </form>
    </Modal>
  )
}
