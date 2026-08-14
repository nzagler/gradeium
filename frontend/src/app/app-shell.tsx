import { useEffect, useState } from "react"
import {
  Film,
  Gamepad2,
  LayoutDashboard,
  LogOut,
  Menu,
  Settings,
  Tv,
  UserRound,
  X,
  type LucideIcon,
} from "lucide-react"
import { NavLink, Outlet } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { getSettings } from "@/api/client"
import { cn } from "@/lib/utils"
import { useAuth } from "@/features/auth/auth-context"

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

function Brand({ name }: { name: string }) {
  return (
    <NavLink
      to="/"
      className="flex items-center gap-3 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      aria-label="Gradeium dashboard"
    >
      <span className="grid size-9 place-items-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
        G
      </span>
      <span className="truncate text-base font-semibold tracking-tight">{name}</span>
    </NavLink>
  )
}

function Navigation({ onNavigate, isAdmin }: { onNavigate?: () => void; isAdmin: boolean }) {
  return (
    <nav aria-label="Primary navigation" className="space-y-1">
      {navigation.filter((item) => item.path !== "/settings" || isAdmin).map((item) => {
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
  const { user, signOut } = useAuth()
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false)
  const [instanceName, setInstanceName] = useState("Gradeium")
  const [signingOut, setSigningOut] = useState(false)
  const [signOutError, setSignOutError] = useState<string | null>(null)

  async function handleSignOut() {
    setSigningOut(true)
    setSignOutError(null)
    try {
      await signOut()
    } catch (error) {
      setSignOutError(error instanceof Error ? error.message : "Sign out failed.")
      setSigningOut(false)
    }
  }

  useEffect(() => {
    let active = true
    void getSettings()
      .then((response) => {
        const setting = response.settings.find(
          (candidate) => candidate.key === "general.instance_name",
        )
        if (active && typeof setting?.value === "string") {
          setInstanceName(setting.value)
        }
      })
      .catch(() => {
        // The shell remains usable with its safe default while settings retry.
      })

    function updateInstanceName(event: Event) {
      if (event instanceof CustomEvent && typeof event.detail === "string") {
        setInstanceName(event.detail)
      }
    }
    window.addEventListener("gradeium:instance-name", updateInstanceName)
    return () => {
      active = false
      window.removeEventListener("gradeium:instance-name", updateInstanceName)
    }
  }, [])

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 flex-col border-r bg-sidebar p-5 text-sidebar-foreground lg:flex">
        <Brand name={instanceName} />
        <div className="mt-8 flex-1">
          <Navigation isAdmin={user.isAdmin} />
        </div>
        <div className="space-y-3 border-t pt-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted">
              <UserRound aria-hidden="true" className="size-4" />
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">
                {user.displayName ?? user.email ?? "Signed in"}
              </p>
              <p className="text-xs text-muted-foreground">
                {user.isAdmin ? "Administrator" : "User"}
              </p>
            </div>
          </div>
          <Button
            className="w-full justify-start"
            type="button"
            variant="ghost"
            disabled={signingOut}
            onClick={() => void handleSignOut()}
          >
            <LogOut aria-hidden="true" />
            {signingOut ? "Signing out…" : "Sign out"}
          </Button>
          {signOutError && <p className="text-xs text-destructive">{signOutError}</p>}
        </div>
      </aside>

      <header className="sticky top-0 z-30 border-b bg-background lg:hidden">
        <div className="flex h-16 items-center justify-between px-4">
          <Brand name={instanceName} />
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
            <Navigation
              isAdmin={user.isAdmin}
              onNavigate={() => setMobileNavigationOpen(false)}
            />
            <div className="mt-3 border-t pt-3">
              <p className="px-3 text-sm font-medium">
                {user.displayName ?? user.email ?? "Signed in"}
              </p>
              <Button
                className="mt-2 w-full justify-start"
                type="button"
                variant="ghost"
                disabled={signingOut}
                onClick={() => void handleSignOut()}
              >
                <LogOut aria-hidden="true" />
                {signingOut ? "Signing out…" : "Sign out"}
              </Button>
            </div>
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
