import {
  useEffect,
  useState,
  type FormEvent,
  type ReactNode,
} from "react"
import {
  Archive,
  Blocks,
  CircleCheck,
  KeyRound,
  LoaderCircle,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  type LucideIcon,
} from "lucide-react"
import { Navigate, NavLink, useParams } from "react-router-dom"

import {
  getSettings,
  getSystemStatus,
  updateSetting,
  type SettingDefinition,
  type SystemStatus,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { AuthenticationPanel } from "@/features/auth/authentication-panel"
import { useAuth } from "@/features/auth/auth-context"
import { IntegrationsPanel } from "@/features/settings/integrations-panel"
import { LibrarySettings } from "@/features/settings/library-settings"
import { BackupsPanel } from "@/features/settings/backups-panel"

const instanceNameKey = "general.instance_name"

type SectionSlug =
  | "general"
  | "authentication"
  | "library"
  | "integrations"
  | "backups"
  | "system"

type SettingsSection = {
  slug: SectionSlug
  label: string
  icon: LucideIcon
}

const sections: SettingsSection[] = [
  { slug: "general", label: "General", icon: SlidersHorizontal },
  { slug: "library", label: "Library", icon: SlidersHorizontal },
  { slug: "authentication", label: "Authentication", icon: KeyRound },
  { slug: "integrations", label: "Integrations", icon: Blocks },
  { slug: "backups", label: "Backups", icon: Archive },
  { slug: "system", label: "System", icon: ShieldCheck },
]

type PageState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | {
      status: "ready"
      settings: SettingDefinition[]
      system: SystemStatus
    }

export function SettingsPage() {
  const { user } = useAuth()
  const { section } = useParams<{ section?: string }>()
  const activeSection =
    user.isAdmin
      ? sections.find((candidate) => candidate.slug === section)?.slug ?? "general"
      : "library"
  const [state, setState] = useState<PageState>({ status: "loading" })

  async function loadSettings() {
    if (!user.isAdmin) return
    try {
      const [settingsResponse, system] = await Promise.all([
        getSettings(),
        getSystemStatus(),
      ])
      setState({ status: "ready", settings: settingsResponse.settings, system })
    } catch (error) {
      setState({
        status: "error",
        message:
          error instanceof Error ? error.message : "Settings could not be loaded.",
      })
    }
  }

  useEffect(() => {
    if (!user.isAdmin) return
    let active = true
    void Promise.all([getSettings(), getSystemStatus()])
      .then(([settingsResponse, system]) => {
        if (active) {
          setState({ status: "ready", settings: settingsResponse.settings, system })
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setState({
            status: "error",
            message:
              error instanceof Error ? error.message : "Settings could not be loaded.",
          })
        }
      })
    return () => {
      active = false
    }
  }, [user.isAdmin])

  if (!user.isAdmin && section !== undefined && section !== "library") {
    return <Navigate to="/settings/library" replace />
  }

  return (
    <section aria-labelledby="settings-title">
      <div className="flex items-start gap-4">
        <span className="grid size-11 shrink-0 place-items-center rounded-lg border bg-card shadow-xs">
          <Settings aria-hidden="true" className="size-5" />
        </span>
        <div>
          <h1 id="settings-title" className="text-2xl font-semibold tracking-tight">
            Settings
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            Manage Library preferences and, for administrators, installation settings.
          </p>
        </div>
      </div>

      <div className="mt-8 grid gap-7 lg:grid-cols-[13rem_minmax(0,1fr)]">
        <SettingsNavigation activeSection={activeSection} isAdmin={user.isAdmin} />
        <div className="min-w-0">
          {!user.isAdmin && (
            <SettingsContent section="library" settings={[]} />
          )}
          {user.isAdmin && state.status === "loading" && <SettingsLoading />}
          {user.isAdmin && state.status === "error" && (
            <SettingsError
              message={state.message}
              retry={() => {
                setState({ status: "loading" })
                return loadSettings()
              }}
            />
          )}
          {user.isAdmin && state.status === "ready" && (
            <SettingsContent
              section={activeSection}
              settings={state.settings}
              system={state.system}
            />
          )}
        </div>
      </div>
    </section>
  )
}

function SettingsNavigation({ activeSection, isAdmin }: { activeSection: SectionSlug; isAdmin: boolean }) {
  return (
    <nav
      aria-label="Settings sections"
      className="flex gap-1 overflow-x-auto pb-1 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0"
    >
      {sections.filter((section) => isAdmin || section.slug === "library").map((section) => {
        const Icon = section.icon
        return (
          <NavLink
            key={section.slug}
            to={section.slug === "general" ? "/settings" : `/settings/${section.slug}`}
            aria-current={activeSection === section.slug ? "page" : undefined}
            className={cn(
              "flex min-h-10 shrink-0 items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              activeSection === section.slug && "bg-accent text-accent-foreground",
            )}
          >
            <Icon aria-hidden="true" className="size-4" />
            {section.label}
          </NavLink>
        )
      })}
    </nav>
  )
}

function SettingsContent({
  section,
  settings,
  system,
}: {
  section: SectionSlug
  settings: SettingDefinition[]
  system?: SystemStatus
}) {
  if (section === "general") {
    return <GeneralSettings settings={settings} />
  }
  if (section === "system" && system) {
    return <SystemSettings status={system} />
  }
  if (section === "authentication") {
    return <AuthenticationPanel bootstrap={false} />
  }
  if (section === "library") {
    return <LibrarySettings />
  }
  if (section === "integrations") {
    return <IntegrationsPanel />
  }
  if (section === "backups") {
    return <BackupsPanel />
  }
  return (
    <SettingsCard
      title={sections.find((candidate) => candidate.slug === section)?.label ?? "Settings"}
      description="This settings section is not available."
    >
      <p className="text-sm text-muted-foreground">Available in a later phase.</p>
    </SettingsCard>
  )
}

function GeneralSettings({ settings }: { settings: SettingDefinition[] }) {
  const instanceSetting = settings.find((setting) => setting.key === instanceNameKey)
  const initialValue =
    typeof instanceSetting?.value === "string" ? instanceSetting.value : "Gradeium"
  const [instanceName, setInstanceName] = useState(initialValue)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<string | null>(null)

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setMessage(null)
    try {
      const result = await updateSetting(instanceNameKey, instanceName)
      const savedValue = typeof result.value === "string" ? result.value : instanceName
      setInstanceName(savedValue)
      window.dispatchEvent(
        new CustomEvent("gradeium:instance-name", { detail: savedValue }),
      )
      setMessage("General settings saved.")
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "The setting could not be saved.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsCard
      title="General"
      description="Basic presentation settings stored in Gradeium's database."
    >
      <form className="space-y-5" onSubmit={(event) => void save(event)}>
        <div className="space-y-2">
          <Label htmlFor="instance-name">Instance name</Label>
          <Input
            id="instance-name"
            value={instanceName}
            maxLength={80}
            autoComplete="off"
            onChange={(event) => setInstanceName(event.target.value)}
            aria-describedby="instance-name-description"
          />
          <p
            id="instance-name-description"
            className="text-sm leading-5 text-muted-foreground"
          >
            Shown in the application navigation. Between 1 and 80 characters.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" disabled={saving || instanceName.trim() === ""}>
            {saving && <LoaderCircle aria-hidden="true" className="animate-spin" />}
            {saving ? "Saving…" : "Save changes"}
          </Button>
          {message && (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {message}
            </p>
          )}
        </div>
      </form>
    </SettingsCard>
  )
}

function SystemSettings({ status }: { status: SystemStatus }) {
  return (
    <SettingsCard
      title="System"
      description="Safe installation status. Secret material is never displayed."
    >
      <dl className="divide-y rounded-md border">
        <StatusRow
          label="Initial setup"
          value={status.setupComplete ? "Complete" : "Required"}
          healthy={status.setupComplete}
        />
        <StatusRow
          label="Master key"
          value={status.masterKey.available ? "Available" : "Unavailable"}
          healthy={status.masterKey.available}
        />
        <StatusRow
          label="Authentication"
          value={status.authenticationActivated ? "Activated" : "Not activated"}
          healthy={status.authenticationActivated}
        />
        {status.backups && (
          <StatusRow
            label="Backup runtime"
            value={status.backups.available ? (status.backups.automaticEnabled ? "Available · automatic enabled" : "Available · automatic disabled") : "Unavailable"}
            healthy={status.backups.available}
          />
        )}
        <div className="grid gap-1 px-4 py-3 sm:grid-cols-[10rem_1fr] sm:items-center">
          <dt className="text-sm font-medium">Key storage</dt>
          <dd className="text-sm text-muted-foreground">{status.masterKey.storage}</dd>
        </div>
        <div className="grid gap-1 px-4 py-3 sm:grid-cols-[10rem_1fr] sm:items-center">
          <dt className="text-sm font-medium">Application</dt>
          <dd className="text-sm text-muted-foreground">{status.application.version} · {status.application.commit} · {status.application.goVersion}</dd>
        </div>
      </dl>
      <p className="mt-4 text-sm leading-6 text-muted-foreground">
        Keep the config mount and PostgreSQL data together when planning disaster
        recovery. Losing either side prevents encrypted settings from being used.
      </p>
    </SettingsCard>
  )
}

function StatusRow({
  label,
  value,
  healthy,
}: {
  label: string
  value: string
  healthy: boolean
}) {
  return (
    <div className="grid gap-1 px-4 py-3 sm:grid-cols-[10rem_1fr] sm:items-center">
      <dt className="text-sm font-medium">{label}</dt>
      <dd className="flex items-center gap-2 text-sm text-muted-foreground">
        <CircleCheck
          aria-hidden="true"
          className={cn("size-4", healthy ? "text-emerald-600" : "text-destructive")}
        />
        {value}
      </dd>
    </div>
  )
}

function SettingsCard({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="rounded-lg border bg-card shadow-xs">
      <div className="border-b px-5 py-4 sm:px-6">
        <h2 className="text-base font-semibold">{title}</h2>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">{description}</p>
      </div>
      <div className="p-5 sm:p-6">{children}</div>
    </div>
  )
}

function SettingsLoading() {
  return (
    <div aria-label="Loading settings" className="rounded-lg border bg-card p-6">
      <div className="h-5 w-28 animate-pulse rounded bg-muted" />
      <div className="mt-4 h-9 w-full max-w-md animate-pulse rounded bg-muted" />
      <div className="mt-3 h-4 w-64 max-w-full animate-pulse rounded bg-muted" />
    </div>
  )
}

function SettingsError({ message, retry }: { message: string; retry: () => Promise<void> }) {
  return (
    <SettingsCard title="Settings unavailable" description={message}>
      <Button type="button" variant="outline" onClick={() => void retry()}>
        Try again
      </Button>
    </SettingsCard>
  )
}
