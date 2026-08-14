import { useState } from "react"
import {
  Film,
  Gamepad2,
  LayoutDashboard,
  Menu,
  Settings,
  Tv,
  X,
  type LucideIcon,
} from "lucide-react"
import { NavLink, Outlet } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type NavigationItem = {
  label: string
  path: string
  icon: LucideIcon
}

const navigation: NavigationItem[] = [
  { label: "Dashboard", path: "/", icon: LayoutDashboard },
  { label: "Games", path: "/games", icon: Gamepad2 },
  { label: "Movies", path: "/movies", icon: Film },
  { label: "TV Shows", path: "/tv", icon: Tv },
  { label: "Settings", path: "/settings", icon: Settings },
]

function Brand() {
  return (
    <NavLink
      to="/"
      className="flex items-center gap-3 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      aria-label="Gradeium dashboard"
    >
      <span className="grid size-9 place-items-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
        G
      </span>
      <span className="text-base font-semibold tracking-tight">Gradeium</span>
    </NavLink>
  )
}

function Navigation({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <nav aria-label="Primary navigation" className="space-y-1">
      {navigation.map((item) => {
        const Icon = item.icon
        return (
          <NavLink
            key={item.path}
            to={item.path}
            end={item.path === "/"}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                "flex min-h-10 items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                isActive && "bg-accent text-accent-foreground",
              )
            }
          >
            <Icon aria-hidden="true" className="size-4" />
            {item.label}
          </NavLink>
        )
      })}
    </nav>
  )
}

export function AppShell() {
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false)

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 flex-col border-r bg-sidebar p-5 text-sidebar-foreground lg:flex">
        <Brand />
        <div className="mt-8 flex-1">
          <Navigation />
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">
          Runtime foundation
        </p>
      </aside>

      <header className="sticky top-0 z-30 border-b bg-background lg:hidden">
        <div className="flex h-16 items-center justify-between px-4">
          <Brand />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-controls="mobile-navigation"
            aria-expanded={mobileNavigationOpen}
            aria-label={mobileNavigationOpen ? "Close navigation" : "Open navigation"}
            onClick={() => setMobileNavigationOpen((open) => !open)}
          >
            {mobileNavigationOpen ? (
              <X aria-hidden="true" />
            ) : (
              <Menu aria-hidden="true" />
            )}
          </Button>
        </div>
        {mobileNavigationOpen && (
          <div id="mobile-navigation" className="border-t p-3">
            <Navigation onNavigate={() => setMobileNavigationOpen(false)} />
          </div>
        )}
      </header>

      <main className="lg:pl-64">
        <div className="mx-auto w-full max-w-7xl px-5 py-8 sm:px-8 lg:px-10 lg:py-10">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
