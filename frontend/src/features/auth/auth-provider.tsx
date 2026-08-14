import type { ReactNode } from "react"

import { AuthContext, type AuthContextValue } from "@/features/auth/auth-context"

export function AuthProvider({
  value,
  children,
}: {
  value: AuthContextValue
  children: ReactNode
}) {
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
