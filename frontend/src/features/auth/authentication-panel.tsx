import { useEffect, useState, type FormEvent, type ReactNode } from "react"
import {
  CheckCircle2,
  KeyRound,
  LoaderCircle,
  RotateCcw,
  ShieldCheck,
} from "lucide-react"

import {
  activateAuthentication,
  getAuthenticationConfiguration,
  saveAuthenticationConfiguration,
  testAuthenticationConfiguration,
  type AuthenticationConfiguration,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Props = {
  bootstrap: boolean
  onActivated?: () => void
}

type FormState = {
  issuerUrl: string
  clientId: string
  clientSecret: string
  publicUrl: string
  removeClientSecret: boolean
}

const emptyForm: FormState = {
  issuerUrl: "",
  clientId: "",
  clientSecret: "",
  publicUrl: "",
  removeClientSecret: false,
}

export function AuthenticationPanel({ bootstrap, onActivated }: Props) {
  const [configuration, setConfiguration] =
    useState<AuthenticationConfiguration | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [loading, setLoading] = useState(true)
  const [working, setWorking] = useState<"save" | "test" | "activate" | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const dirty =
    configuration !== null &&
    (form.issuerUrl !== configuration.issuerUrl ||
      form.clientId !== configuration.clientId ||
      form.publicUrl !== configuration.publicUrl ||
      form.clientSecret !== "" ||
      form.removeClientSecret)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const current = await getAuthenticationConfiguration()
      setConfiguration(current)
      setForm({
        issuerUrl: current.issuerUrl ?? "",
        clientId: current.clientId ?? "",
        clientSecret: "",
        publicUrl: current.publicUrl ?? "",
        removeClientSecret: false,
      })
    } catch (loadError) {
      setError(
        loadError instanceof Error
          ? loadError.message
          : "Authentication configuration could not be loaded.",
      )
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let active = true
    void getAuthenticationConfiguration()
      .then((current) => {
        if (!active) return
        setConfiguration(current)
        setForm({
          issuerUrl: current.issuerUrl ?? "",
          clientId: current.clientId ?? "",
          clientSecret: "",
          publicUrl: current.publicUrl ?? "",
          removeClientSecret: false,
        })
      })
      .catch((loadError: unknown) => {
        if (active) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : "Authentication configuration could not be loaded.",
          )
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setWorking("save")
    setMessage(null)
    setError(null)
    try {
      const saved = await saveAuthenticationConfiguration(form)
      setConfiguration(saved)
      setForm((current) => ({
        ...current,
        issuerUrl: saved.issuerUrl,
        clientId: saved.clientId,
        publicUrl: saved.publicUrl,
        clientSecret: "",
        removeClientSecret: false,
      }))
      setMessage(
        saved.activated
          ? "Validated configuration applied. The previous provider remained active until this update succeeded."
          : "Configuration saved. Test it before enabling authentication.",
      )
    } catch (saveError) {
      setError(
        saveError instanceof Error
          ? saveError.message
          : "Authentication configuration could not be saved.",
      )
    } finally {
      setWorking(null)
    }
  }

  async function testConfiguration() {
    setWorking("test")
    setMessage(null)
    setError(null)
    try {
      const result = await testAuthenticationConfiguration()
      setConfiguration((current) =>
        current
          ? {
              ...current,
              validated: result.validated,
              revision: result.revision,
              redirectUri: result.redirectUri,
            }
          : current,
      )
      setMessage("Discovery and endpoint validation succeeded.")
    } catch (testError) {
      setError(
        testError instanceof Error
          ? testError.message
          : "OIDC configuration validation failed.",
      )
    } finally {
      setWorking(null)
    }
  }

  async function activate() {
    setWorking("activate")
    setMessage(null)
    setError(null)
    try {
      await activateAuthentication()
      setMessage("Authentication enabled. Sign in to bind the initial administrator.")
      onActivated?.()
    } catch (activationError) {
      setError(
        activationError instanceof Error
          ? activationError.message
          : "Authentication could not be enabled.",
      )
    } finally {
      setWorking(null)
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border bg-card p-6" aria-label="Loading authentication settings">
        <div className="h-5 w-40 animate-pulse rounded bg-muted" />
        <div className="mt-5 h-9 w-full animate-pulse rounded bg-muted" />
        <div className="mt-3 h-9 w-full animate-pulse rounded bg-muted" />
      </div>
    )
  }

  if (!configuration && error) {
    return (
      <div className="rounded-lg border bg-card p-6">
        <h2 className="font-semibold">Authentication settings unavailable</h2>
        <p className="mt-2 text-sm text-muted-foreground">{error}</p>
        <Button className="mt-5" type="button" variant="outline" onClick={() => void load()}>
          Try again
        </Button>
      </div>
    )
  }

  return (
    <div className="rounded-lg border bg-card shadow-xs">
      <div className="border-b px-5 py-4 sm:px-6">
        <div className="flex items-center gap-3">
          <span className="grid size-9 place-items-center rounded-md border bg-background">
            <KeyRound aria-hidden="true" className="size-4" />
          </span>
          <div>
            <h2 className="font-semibold">Generic OpenID Connect</h2>
            <p className="mt-1 text-sm leading-5 text-muted-foreground">
              Pocket ID is supported through its standards-based OIDC interface.
            </p>
          </div>
        </div>
      </div>

      <form className="space-y-5 p-5 sm:p-6" onSubmit={(event) => void save(event)}>
        {bootstrap && (
          <div className="flex gap-3 rounded-md border bg-muted/40 p-4 text-sm leading-5">
            <ShieldCheck aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <p>
              Configure this new installation only from a trusted network. The
              unauthenticated bootstrap closes permanently when authentication is enabled.
            </p>
          </div>
        )}

        <Field label="Issuer URL" description="The exact issuer shown by your OIDC provider.">
          <Input
            id="oidc-issuer"
            type="url"
            required
            autoComplete="url"
            value={form.issuerUrl}
            onChange={(event) =>
              setForm((current) => ({ ...current, issuerUrl: event.target.value }))
            }
          />
        </Field>

        <Field label="Client ID" description="The client identifier registered for Gradeium.">
          <Input
            id="oidc-client-id"
            required
            autoComplete="off"
            value={form.clientId}
            onChange={(event) =>
              setForm((current) => ({ ...current, clientId: event.target.value }))
            }
          />
        </Field>

        <Field
          label="Client secret"
          description={
            configuration?.clientSecretConfigured
              ? "A secret is configured. Leave blank to keep it, enter a value to replace it, or mark it for removal."
              : "Not configured. Saved values are encrypted with Gradeium's persistent master key and never displayed again."
          }
        >
          <Input
            id="oidc-client-secret"
            type="password"
            autoComplete="new-password"
            disabled={form.removeClientSecret}
            value={form.clientSecret}
            placeholder={configuration?.clientSecretConfigured ? "Configured — leave blank to keep" : ""}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                clientSecret: event.target.value,
                removeClientSecret: false,
              }))
            }
          />
          {configuration?.clientSecretConfigured && (
            <Button
              className="mt-2"
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                setForm((current) => ({
                  ...current,
                  clientSecret: "",
                  removeClientSecret: !current.removeClientSecret,
                }))
              }
            >
              <RotateCcw aria-hidden="true" />
              {form.removeClientSecret ? "Keep configured secret" : "Remove on save"}
            </Button>
          )}
        </Field>

        <Field
          label="Public Gradeium URL"
          description="The external origin users open through your reverse proxy, without a path. HTTPS is required except on loopback development URLs."
        >
          <Input
            id="oidc-public-url"
            type="url"
            required
            autoComplete="url"
            value={form.publicUrl}
            onChange={(event) =>
              setForm((current) => ({ ...current, publicUrl: event.target.value }))
            }
          />
        </Field>

        {configuration?.redirectUri && (
          <div className="rounded-md border bg-muted/40 p-4">
            <p className="text-sm font-medium">Callback URI</p>
            <code className="mt-2 block break-all text-xs text-muted-foreground">
              {configuration.redirectUri}
            </code>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-3 border-t pt-5">
          <Button type="submit" disabled={working !== null}>
            {working === "save" && <LoaderCircle aria-hidden="true" className="animate-spin" />}
            {configuration?.activated ? "Save and apply validated changes" : "Save configuration"}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={working !== null || !configuration || dirty}
            onClick={() => void testConfiguration()}
          >
            {working === "test" && <LoaderCircle aria-hidden="true" className="animate-spin" />}
            Test configuration
          </Button>
          {bootstrap && (
            <Button
              type="button"
              disabled={working !== null || !configuration?.validated || dirty}
              onClick={() => void activate()}
            >
              {working === "activate" && <LoaderCircle aria-hidden="true" className="animate-spin" />}
              Enable authentication
            </Button>
          )}
        </div>

        {message && (
          <p className="flex items-start gap-2 text-sm text-emerald-700 dark:text-emerald-300" aria-live="polite">
            <CheckCircle2 aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            {message}
          </p>
        )}
        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
      </form>
    </div>
  )
}

function Field({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: ReactNode
}) {
  const id =
    label === "Issuer URL"
      ? "oidc-issuer"
      : label === "Client ID"
        ? "oidc-client-id"
        : label === "Client secret"
          ? "oidc-client-secret"
          : "oidc-public-url"
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      <p className="text-sm leading-5 text-muted-foreground">{description}</p>
    </div>
  )
}
