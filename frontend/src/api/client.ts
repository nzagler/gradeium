export type SetupStatus = {
  complete: boolean
}

export type AuthUser = {
  id: string
  displayName?: string | null
  email?: string | null
  isAdmin: boolean
}

export type AuthStatus = {
  setupComplete: boolean
  activated: boolean
  bootstrapAllowed: boolean
  authenticated: boolean
  user?: AuthUser
  csrfToken?: string
  sessionExpiresAt?: string
}

export type AuthenticationConfiguration = {
  issuerUrl: string
  clientId: string
  publicUrl: string
  revision: number
  activated: boolean
  validated: boolean
  validatedAt?: string
  clientSecretConfigured: boolean
  redirectUri?: string
}

export type AuthenticationConfigurationInput = {
  issuerUrl: string
  clientId: string
  clientSecret: string
  publicUrl: string
  removeClientSecret: boolean
}

export type AuthenticationValidation = {
  revision: number
  redirectUri: string
  issuerUrl: string
  validated: boolean
}

export type SettingDefinition = {
  key: string
  section: string
  label: string
  description: string
  type: "string"
  sensitivity: "public" | "secret"
  reserved: boolean
  configured: boolean
  value?: unknown
}

export type SettingsResponse = {
  settings: SettingDefinition[]
}

export type SystemStatus = {
  setupComplete: boolean
  masterKey: {
    available: boolean
    storage: string
  }
}

type APIErrorResponse = {
  error?: string
  message?: string
}

export class APIError extends Error {
  readonly status: number
  readonly code?: string

  constructor(message: string, status: number, code?: string) {
    super(message)
    this.name = "APIError"
    this.status = status
    this.code = code
  }
}

let sessionCSRFToken: string | null = null

export function setSessionCSRFToken(value: string | null) {
  sessionCSRFToken = value
}

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set("Accept", "application/json")
  if (init?.body !== undefined) {
    headers.set("Content-Type", "application/json")
  }
  const method = (init?.method ?? "GET").toUpperCase()
  if (!["GET", "HEAD", "OPTIONS"].includes(method) && sessionCSRFToken) {
    headers.set("X-CSRF-Token", sessionCSRFToken)
  }

  const response = await fetch(`/api${path}`, {
    ...init,
    headers,
    credentials: "same-origin",
  })
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new APIError("Gradeium returned an invalid response.", response.status)
  }
  if (!response.ok) {
    const error = body as APIErrorResponse
    throw new APIError(
      error.message ?? "The request could not be completed.",
      response.status,
      error.error,
    )
  }
  return body as T
}

export function getSetupStatus(): Promise<SetupStatus> {
  return apiRequest<SetupStatus>("/setup/status")
}

export function completeSetup(): Promise<SetupStatus> {
  return apiRequest<SetupStatus>("/setup/complete", { method: "POST" })
}

export function getAuthStatus(): Promise<AuthStatus> {
  return apiRequest<AuthStatus>("/auth/status")
}

export function getAuthSession() {
  return apiRequest<{ user: AuthUser; expiresAt: string; csrfToken: string }>(
    "/auth/session",
  )
}

export function getAuthenticationConfiguration() {
  return apiRequest<AuthenticationConfiguration>("/auth/configuration")
}

export function saveAuthenticationConfiguration(
  configuration: AuthenticationConfigurationInput,
) {
  return apiRequest<AuthenticationConfiguration>("/auth/configuration", {
    method: "PUT",
    body: JSON.stringify(configuration),
  })
}

export function testAuthenticationConfiguration() {
  return apiRequest<AuthenticationValidation>("/auth/configuration/test", {
    method: "POST",
  })
}

export function activateAuthentication() {
  return apiRequest<AuthenticationConfiguration>("/auth/activate", {
    method: "POST",
  })
}

export function startOIDCLogin(returnTo = "/") {
  return apiRequest<{ authorizationUrl: string }>(
    `/auth/login?returnTo=${encodeURIComponent(returnTo)}`,
  )
}

export function logout() {
  return apiRequest<{ authenticated: false }>("/auth/logout", { method: "POST" })
}

export function getSettings(): Promise<SettingsResponse> {
  return apiRequest<SettingsResponse>("/admin/settings")
}

export function updateSetting(key: string, value: unknown) {
  return apiRequest<{ key: string; configured: boolean; value: unknown }>(
    `/admin/settings/${encodeURIComponent(key)}`,
    { method: "PUT", body: JSON.stringify({ value }) },
  )
}

export function getSystemStatus(): Promise<SystemStatus> {
  return apiRequest<SystemStatus>("/admin/system/status")
}
