import { useEffect, useState, type ReactNode } from "react"
import { AlertCircle, Database, KeyRound, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"

import { APIError, completeSetup, getSetupStatus } from "@/api/client"
import { Button } from "@/components/ui/button"

type SetupGateProps = {
  children: ReactNode
}

type GateState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "incomplete" }
  | { status: "complete" }

export function SetupGate({ children }: SetupGateProps) {
  const navigate = useNavigate()
  const [state, setState] = useState<GateState>({ status: "loading" })
  const [submitting, setSubmitting] = useState(false)

  async function loadStatus() {
    try {
      const result = await getSetupStatus()
      setState({ status: result.complete ? "complete" : "incomplete" })
    } catch (error) {
      setState({
        status: "error",
        message:
          error instanceof Error
            ? error.message
            : "Gradeium could not load setup status.",
      })
    }
  }

  useEffect(() => {
    let active = true
    void getSetupStatus()
      .then((result) => {
        if (active) {
          setState({ status: result.complete ? "complete" : "incomplete" })
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setState({
            status: "error",
            message:
              error instanceof Error
                ? error.message
                : "Gradeium could not load setup status.",
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
      setState({ status: "complete" })
      navigate("/settings", { replace: true })
    } catch (error) {
      if (error instanceof APIError && error.code === "setup_already_complete") {
        setState({ status: "complete" })
        navigate("/settings", { replace: true })
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

  if (state.status === "complete") {
    return children
  }

  if (state.status === "loading") {
    return (
      <SetupFrame>
        <LoaderCircle
          aria-hidden="true"
          className="size-6 animate-spin text-muted-foreground"
        />
        <p className="mt-4 text-sm text-muted-foreground">
          Checking installation state…
        </p>
      </SetupFrame>
    )
  }

  if (state.status === "error") {
    return (
      <SetupFrame>
        <AlertCircle aria-hidden="true" className="size-7 text-destructive" />
        <h1 className="mt-4 text-xl font-semibold tracking-tight">
          Setup could not continue
        </h1>
        <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
          {state.message}
        </p>
        <Button
          className="mt-6"
          type="button"
          onClick={() => {
            setState({ status: "loading" })
            void loadStatus()
          }}
        >
          Try again
        </Button>
      </SetupFrame>
    )
  }

  return (
    <SetupFrame>
      <div className="grid size-11 place-items-center rounded-lg border bg-card shadow-xs">
        <KeyRound aria-hidden="true" className="size-5" />
      </div>
      <p className="mt-6 text-sm font-medium text-muted-foreground">
        First-run setup
      </p>
      <h1 className="mt-2 text-2xl font-semibold tracking-tight">
        Initialize Gradeium
      </h1>
      <p className="mt-3 max-w-lg text-sm leading-6 text-muted-foreground">
        This creates Gradeium&apos;s one-time setup state and secures future
        settings with a persistent key in the config mount. No account,
        provider, or login is configured in this phase.
      </p>

      <div className="mt-7 w-full max-w-lg rounded-lg border bg-card p-4 text-left shadow-xs">
        <div className="flex gap-3">
          <Database
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0 text-muted-foreground"
          />
          <div>
            <p className="text-sm font-medium">Persistent installation state</p>
            <p className="mt-1 text-sm leading-5 text-muted-foreground">
              Setup can be completed only once. Authentication and integrations
              remain unavailable until their dedicated phases.
            </p>
          </div>
        </div>
      </div>

      <Button
        className="mt-7 min-w-40"
        type="button"
        disabled={submitting}
        onClick={() => void initialize()}
      >
        {submitting && <LoaderCircle aria-hidden="true" className="animate-spin" />}
        {submitting ? "Initializing…" : "Initialize Gradeium"}
      </Button>
    </SetupFrame>
  )
}

function SetupFrame({ children }: { children: ReactNode }) {
  return (
    <main className="grid min-h-svh place-items-center bg-background px-5 py-12 text-foreground">
      <section className="flex w-full max-w-2xl flex-col items-center text-center">
        {children}
      </section>
    </main>
  )
}
