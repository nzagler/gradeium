import {
  type KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useRef,
  useState,
} from "react"
import { Film, Gamepad2, Plus, Tv } from "lucide-react"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"

const quickAddItems = [
  { label: "Games", path: "/games/add", icon: Gamepad2 },
  { label: "TV Shows", path: "/tv/add", icon: Tv },
  { label: "Movies", path: "/movies/add", icon: Film },
]

export function QuickAdd() {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const firstItemRef = useRef<HTMLAnchorElement>(null)

  useEffect(() => {
    if (!open) return

    firstItemRef.current?.focus()

    function closeFromOutside(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }

    function closeFromEscape(event: KeyboardEvent) {
      if (event.key !== "Escape") return
      event.preventDefault()
      setOpen(false)
      triggerRef.current?.focus()
    }

    document.addEventListener("pointerdown", closeFromOutside)
    document.addEventListener("keydown", closeFromEscape)
    return () => {
      document.removeEventListener("pointerdown", closeFromOutside)
      document.removeEventListener("keydown", closeFromEscape)
    }
  }, [open])

  function moveFocus(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return

    const items = Array.from(
      event.currentTarget.querySelectorAll<HTMLAnchorElement>("[role='menuitem']"),
    )
    if (items.length === 0) return

    event.preventDefault()
    const current = items.indexOf(document.activeElement as HTMLAnchorElement)
    if (event.key === "Home") items[0]?.focus()
    else if (event.key === "End") items.at(-1)?.focus()
    else if (event.key === "ArrowDown") items[(current + 1) % items.length]?.focus()
    else items[(current - 1 + items.length) % items.length]?.focus()
  }

  return (
    <div ref={rootRef} className="quick-add fixed bottom-4 right-4 z-40">
      {open && (
        <div
          id="quick-add-menu"
          role="menu"
          aria-label="Add media"
          className="absolute bottom-14 right-0 w-44 rounded-lg border bg-popover p-1.5 text-popover-foreground shadow-lg"
          onKeyDown={moveFocus}
        >
          {quickAddItems.map((item, index) => {
            const Icon = item.icon
            return (
              <Link
                key={item.path}
                ref={index === 0 ? firstItemRef : undefined}
                to={item.path}
                role="menuitem"
                className="flex min-h-10 items-center gap-3 rounded-md px-3 text-sm font-medium outline-none hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => setOpen(false)}
              >
                <Icon aria-hidden="true" className="size-4" />
                {item.label}
              </Link>
            )
          })}
        </div>
      )}
      <Button
        ref={triggerRef}
        type="button"
        size="icon"
        className="size-11 rounded-full shadow-md"
        aria-label={open ? "Close quick add" : "Quick add"}
        aria-controls="quick-add-menu"
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((current) => !current)}
      >
        <Plus aria-hidden="true" className="size-5" />
      </Button>
    </div>
  )
}
