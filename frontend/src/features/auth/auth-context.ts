import { createContext, useContext } from "react"

import type { AuthUser } from "@/api/client"

export type AuthContextValue = {
  user: AuthUser
  signOut: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) {
    throw new Error("useAuth must be used inside the authenticated Gradeium shell")
  }
  return value
}
