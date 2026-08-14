import { useState, type ReactNode } from "react"
import { KeyRound, LoaderCircle, ShieldCheck } from "lucide-react"
import { useLocation } from "react-router-dom"

import { startOIDCLogin } from "@/api/client"
import { Button } from "@/components/ui/button"

export function LoginPage() {
  const location = useLocation()
  const [working, setWorking] = useState(false)
  const [error, setError] = useState<string | null>(
    new URLSearchParams(location.search).has("error")
      ? "Sign in could not be completed. Start a new OIDC sign-in request."
      : null,
  )

  async function signIn() {
    setWorking(true)
    setError(null)
    try {
      const result = await startOIDCLogin("/")
      window.location.assign(result.authorizationUrl)
    } catch (loginError) {
      setError(
        loginError instanceof Error
          ? loginError.message
          : "OIDC sign in could not be started.",
      )
      setWorking(false)
    }
  }

  return (
    <AuthFrame>
      <div className="grid size-11 place-items-center rounded-lg border bg-card shadow-xs">
        <KeyRound aria-hidden="true" className="size-5" />
      </div>
      <p className="mt-6 text-sm font-medium text-muted-foreground">Gradeium</p>
      <h1 className="mt-2 text-2xl font-semibold tracking-tight">Sign in</h1>
      <p className="mt-3 max-w-md text-sm leading-6 text-muted-foreground">
        Continue with the OpenID Connect provider configured by your administrator.
      </p>
      <Button className="mt-7 min-w-44" type="button" disabled={working} onClick={() => void signIn()}>
        {working && <LoaderCircle aria-hidden="true" className="animate-spin" />}
        {working ? "Opening provider…" : "Sign in with OIDC"}
      </Button>
      {error && (
        <p className="mt-5 max-w-md text-sm text-destructive" role="alert">
          {error}
        </p>
      )}
      <p className="mt-8 flex items-center gap-2 text-xs text-muted-foreground">
        <ShieldCheck aria-hidden="true" className="size-3.5" />
        Gradeium does not provide a local-password bypass.
      </p>
    </AuthFrame>
  )
}

export function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <main className="grid min-h-svh place-items-center bg-background px-5 py-12 text-foreground">
      <section className="flex w-full max-w-2xl flex-col items-center text-center">
        {children}
      </section>
    </main>
  )
}
