import { useEffect, useState, type ReactNode } from "react"
import { AlertCircle, Database, KeyRound, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"

import {
  APIError,
  completeSetup,
  getAuthStatus,
  getSetupStatus,
  logout,
  setSessionCSRFToken,
  type AuthStatus,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { AuthProvider } from "@/features/auth/auth-provider"
import { AuthenticationPanel } from "@/features/auth/authentication-panel"
import { AuthFrame, LoginPage } from "@/features/auth/login-page"

type SetupGateProps = {
  children: ReactNode
}

type GateState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "setup" }
  | { status: "authentication-setup" }
  | { status: "login" }
  | { status: "ready"; auth: AuthStatus }

export function SetupGate({ children }: SetupGateProps) {
  const navigate = useNavigate()
  const [state, setState] = useState<GateState>({ status: "loading" })
  const [submitting, setSubmitting] = useState(false)

  function applyAuthStatus(auth: AuthStatus) {
    if (!auth.activated) {
      setSessionCSRFToken(null)
      setState({ status: "authentication-setup" })
      return
    }
    if (!auth.authenticated || !auth.user || !auth.csrfToken) {
      setSessionCSRFToken(null)
      setState({ status: "login" })
      return
    }
    setSessionCSRFToken(auth.csrfToken)
    setState({ status: "ready", auth })
  }

  async function loadGate() {
    try {
      const setup = await getSetupStatus()
      if (!setup.complete) {
        setSessionCSRFToken(null)
        setState({ status: "setup" })
        return
      }
      applyAuthStatus(await getAuthStatus())
    } catch (error) {
      setState({
        status: "error",
        message:
          error instanceof Error
            ? error.message
            : "Gradeium could not load installation state.",
      })
    }
  }

  useEffect(() => {
    let active = true
    void Promise.all([getSetupStatus(), getAuthStatus()])
      .then(([setup, auth]) => {
        if (!active) return
        if (!setup.complete) {
          setSessionCSRFToken(null)
          setState({ status: "setup" })
          return
        }
        if (!auth.activated) {
          setSessionCSRFToken(null)
          setState({ status: "authentication-setup" })
        } else if (!auth.authenticated || !auth.user || !auth.csrfToken) {
          setSessionCSRFToken(null)
          setState({ status: "login" })
        } else {
          setSessionCSRFToken(auth.csrfToken)
          setState({ status: "ready", auth })
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setState({
            status: "error",
            message:
              error instanceof Error
                ? error.message
                : "Gradeium could not load installation state.",
          })
        }
      })
    return () => {
      active = false
    }
  }, [])

  async function initialize() {
    setSubmitting(true)
    try {
      await completeSetup()
      navigate("/settings/authentication", { replace: true })
      applyAuthStatus(await getAuthStatus())
    } catch (error) {
      if (error instanceof APIError && error.code === "setup_already_complete") {
        navigate("/settings/authentication", { replace: true })
        applyAuthStatus(await getAuthStatus())
        return
      }
      setState({
        status: "error",
        message:
          error instanceof Error
            ? error.message
            : "Gradeium could not complete setup.",
      })
    } finally {
      setSubmitting(false)
    }
  }

  async function signOut() {
    await logout()
    setSessionCSRFToken(null)
    setState({ status: "login" })
    navigate("/", { replace: true })
  }

  if (state.status === "ready" && state.auth.user) {
    return (
      <AuthProvider value={{ user: state.auth.user, signOut }}>
        {children}
      </AuthProvider>
    )
  }

  if (state.status === "loading") {
    return (
      <SetupFrame>
        <LoaderCircle aria-hidden="true" className="size-6 animate-spin text-muted-foreground" />
        <p className="mt-4 text-sm text-muted-foreground">Checking installation state…</p>
      </SetupFrame>
    )
  }

  if (state.status === "error") {
    return (
      <SetupFrame>
        <AlertCircle aria-hidden="true" className="size-7 text-destructive" />
        <h1 className="mt-4 text-xl font-semibold tracking-tight">Gradeium could not continue</h1>
        <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{state.message}</p>
        <Button
          className="mt-6"
          type="button"
          onClick={() => {
            setState({ status: "loading" })
            void loadGate()
          }}
        >
          Try again
        </Button>
      </SetupFrame>
    )
  }

  if (state.status === "authentication-setup") {
    return (
      <AuthFrame>
        <p className="text-sm font-medium text-muted-foreground">Authentication setup</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">Protect this Gradeium instance</h1>
        <p className="mt-3 max-w-xl text-sm leading-6 text-muted-foreground">
          Configure generic OpenID Connect, validate discovery, and enable sign in. The first verified identity becomes the initial administrator.
        </p>
        <div className="mt-8 w-full text-left">
          <AuthenticationPanel
            bootstrap
            onActivated={() => {
              setSessionCSRFToken(null)
              setState({ status: "login" })
              navigate("/", { replace: true })
            }}
          />
        </div>
      </AuthFrame>
    )
  }

  if (state.status === "login") {
    return <LoginPage />
  }

  return (
    <SetupFrame>
      <div className="grid size-11 place-items-center rounded-lg border bg-card shadow-xs">
        <KeyRound aria-hidden="true" className="size-5" />
      </div>
      <p className="mt-6 text-sm font-medium text-muted-foreground">First-run setup</p>
      <h1 className="mt-2 text-2xl font-semibold tracking-tight">Initialize Gradeium</h1>
      <p className="mt-3 max-w-lg text-sm leading-6 text-muted-foreground">
        This creates Gradeium&apos;s one-time installation state and secures settings with a persistent key. Authentication is configured in the next step; no local password is created.
      </p>

      <div className="mt-7 w-full max-w-lg rounded-lg border bg-card p-4 text-left shadow-xs">
        <div className="flex gap-3">
          <Database aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div>
            <p className="text-sm font-medium">Persistent installation state</p>
            <p className="mt-1 text-sm leading-5 text-muted-foreground">
              Setup can be completed only once. Keep PostgreSQL and the config mount together for disaster recovery.
            </p>
          </div>
        </div>
      </div>

      <Button className="mt-7 min-w-40" type="button" disabled={submitting} onClick={() => void initialize()}>
        {submitting && <LoaderCircle aria-hidden="true" className="animate-spin" />}
        {submitting ? "Initializing…" : "Initialize Gradeium"}
      </Button>
    </SetupFrame>
  )
}

function SetupFrame({ children }: { children: ReactNode }) {
  return (
    <main className="grid min-h-svh place-items-center bg-background px-5 py-12 text-foreground">
      <section className="flex w-full max-w-2xl flex-col items-center text-center">{children}</section>
    </main>
  )
}
