export type SetupStatus = {
  complete: boolean
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

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set("Accept", "application/json")
  if (init?.body !== undefined) {
    headers.set("Content-Type", "application/json")
  }

  const response = await fetch(`/api${path}`, { ...init, headers })
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
  return apiRequest<SetupStatus>("/setup/complete", {
    method: "POST",
  })
}

export function getSettings(): Promise<SettingsResponse> {
  return apiRequest<SettingsResponse>("/admin/settings")
}

export function updateSetting(key: string, value: unknown) {
  return apiRequest<{ key: string; configured: boolean; value: unknown }>(
    `/admin/settings/${encodeURIComponent(key)}`,
    {
      method: "PUT",
      body: JSON.stringify({ value }),
    },
  )
}

export function getSystemStatus(): Promise<SystemStatus> {
  return apiRequest<SystemStatus>("/admin/system/status")
}
