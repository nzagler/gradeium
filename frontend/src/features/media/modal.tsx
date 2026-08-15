import { useEffect, useRef, type ReactNode } from "react"
import { X } from "lucide-react"

import { Button } from "@/components/ui/button"

export function Modal({
  title,
  description,
  close,
  children,
  wide = false,
}: {
  title: string
  description?: string
  close: () => void
  children: ReactNode
  wide?: boolean
}) {
  const panel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    panel.current?.focus()
    function keydown(event: KeyboardEvent) {
      if (event.key === "Escape") close()
      if (event.key !== "Tab" || !panel.current) return
      const focusable = Array.from(
        panel.current.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((element) => !element.hasAttribute("hidden"))
      if (focusable.length === 0) {
        event.preventDefault()
        panel.current.focus()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.current)) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener("keydown", keydown)
    return () => {
      document.removeEventListener("keydown", keydown)
      document.body.style.overflow = previousOverflow
      previous?.focus()
    }
  }, [close])

  return (
    <div
      className="fixed inset-0 z-50 grid items-end bg-black/55 p-0 sm:place-items-center sm:p-5"
      onMouseDown={(event) => event.target === event.currentTarget && close()}
    >
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        tabIndex={-1}
        className={`max-h-[92svh] w-full overflow-y-auto rounded-t-xl border bg-background shadow-xl outline-none sm:rounded-xl ${wide ? "sm:max-w-5xl" : "sm:max-w-lg"}`}
      >
        <div className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b bg-background px-5 py-4">
          <div>
            <h2 id="modal-title" className="font-semibold">{title}</h2>
            {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={close} aria-label="Close dialog">
            <X aria-hidden="true" />
          </Button>
        </div>
        <div className="p-5 sm:p-6">{children}</div>
      </div>
    </div>
  )
}
