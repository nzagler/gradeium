import { useState, type FormEvent, type KeyboardEvent } from "react"
import { LoaderCircle, Minus, Plus, Star } from "lucide-react"

import type { MediaDomain, PersonalState } from "@/api/client"
import { updateMediaState } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Modal } from "@/features/media/modal"
import { useRatingScale } from "@/features/media/rating-scale-context"
import {
  canonicalFromDisplay,
  displayFromCanonical,
  eraseRatingDigit,
  formatPersonalRating,
  parseDisplayRating,
  ratingScaleLabel,
  ratingStepLabel,
  rollRatingDigit,
} from "@/features/media/rating-scale"

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
  const scale = useRatingScale()
  if (state.status === "backlog") return null
  const rated = state.rating !== undefined
  return (
    <>
      <Button className={className} type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
        <Star aria-hidden="true" className={rated ? "fill-current" : ""} />
        {rated ? formatPersonalRating(state.rating ?? 0, scale) : "Rate"}
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
  const scale = useRatingScale()
  const [rating, setRating] = useState(state.rating ?? 50)
  const [reason, setReason] = useState(state.ratingReason ?? "")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inputError, setInputError] = useState<string | null>(null)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (inputError) return
    setSaving(true)
    setError(null)
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
    setSaving(true)
    setError(null)
    try {
      const value = await updateMediaState(domain, id, { status: state.status })
      saved({ ...state, ...value, rating: undefined, ratingReason: undefined })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The rating could not be cleared.")
      setSaving(false)
    }
  }

  function changeRating(next: number) {
    setRating(Math.max(0, Math.min(100, next)))
    setInputError(null)
  }

  function keyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (/^[0-9]$/.test(event.key)) {
      event.preventDefault()
      changeRating(rollRatingDigit(rating, event.key, scale))
    } else if (scale === "minus5_plus5" && (event.key === "-" || event.key === "+")) {
      event.preventDefault()
      const display = Math.abs(displayFromCanonical(rating, scale)) * (event.key === "-" ? -1 : 1)
      changeRating(canonicalFromDisplay(display, scale))
    } else if (event.key === "Backspace" || event.key === "Delete") {
      event.preventDefault()
      changeRating(eraseRatingDigit(rating, scale))
    } else if (event.key === "ArrowDown") {
      event.preventDefault()
      changeRating(rating - 1)
    } else if (event.key === "ArrowUp") {
      event.preventDefault()
      changeRating(rating + 1)
    }
  }

  return (
    <Modal title={`Rate ${title}`} description={`Your private rating is displayed on the ${ratingScaleLabel(scale)} scale and stored canonically.`} close={close}>
      <form className="space-y-5" onSubmit={(event) => void submit(event)}>
        <div>
          <label htmlFor="rating-value" className="text-sm font-medium">My rating</label>
          <div className="mt-2 flex flex-wrap items-center gap-3">
            <Button type="button" variant="outline" size="icon" aria-label={`Decrease rating by ${ratingStepLabel(scale)}`} onClick={() => changeRating(rating - 1)}><Minus /></Button>
            <input
              id="rating-value"
              className="h-10 w-28 rounded-md border bg-background px-3 text-center text-lg font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              type="text"
              inputMode={scale === "0_100" ? "numeric" : "decimal"}
              value={formatPersonalRating(rating, scale)}
              aria-invalid={inputError ? true : undefined}
              aria-describedby="rating-guidance"
              onKeyDown={keyDown}
              onChange={(event) => {
                const raw = event.target.value
                const current = formatPersonalRating(rating, scale)
                const appended = raw.startsWith(current) ? raw.slice(current.length) : ""
                if (/^[0-9]$/.test(appended)) {
                  changeRating(rollRatingDigit(rating, appended, scale))
                  return
                }
                const parsed = parseDisplayRating(raw, scale)
                if (parsed === undefined) {
                  setInputError(`Enter a rating on the ${ratingScaleLabel(scale)} scale.`)
                } else {
                  changeRating(parsed)
                }
              }}
            />
            <Button type="button" variant="outline" size="icon" aria-label={`Increase rating by ${ratingStepLabel(scale)}`} onClick={() => changeRating(rating + 1)}><Plus /></Button>
            {scale === "minus5_plus5" && (
              <Button type="button" variant="outline" aria-label="Toggle rating sign" onClick={() => changeRating(100 - rating)}>+ / −</Button>
            )}
          </div>
          <p id="rating-guidance" className="mt-2 text-xs text-muted-foreground">Type digits to roll the value; on 0 to 10, 5.0 then 8 becomes 0.8, then 7 becomes 8.7. Arrow keys and the step buttons change it by {ratingStepLabel(scale)}.</p>
          {inputError && <p role="alert" className="mt-1 text-xs text-destructive">{inputError}</p>}
        </div>
        <div>
          <label htmlFor="rating-reason" className="text-sm font-medium">Reason <span className="font-normal text-muted-foreground">(optional)</span></label>
          <textarea id="rating-reason" maxLength={4000} rows={4} value={reason} onChange={(event) => setReason(event.target.value)} className="mt-2 w-full resize-y rounded-md border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" />
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <div className="flex flex-wrap justify-end gap-2">
          {state.rating !== undefined && <Button type="button" variant="ghost" disabled={saving} onClick={() => void clearRating()}>Clear rating</Button>}
          <Button type="button" variant="outline" onClick={close}>Cancel</Button>
          <Button type="submit" disabled={saving || Boolean(inputError)}>{saving && <LoaderCircle className="animate-spin" />}{saving ? "Saving…" : "Save rating"}</Button>
        </div>
      </form>
    </Modal>
  )
}
