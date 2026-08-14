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
  type: "string" | "boolean"
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

export type MediaDomain = "games" | "movies" | "tv"
export type MediaStatus =
  | "backlog"
  | "in_progress"
  | "on_hold"
  | "abandoned"
  | "completed"

export type PersonalState = {
  status: MediaStatus
  rating?: number
  ratingReason?: string
  dateAdded: string
}

export type TVProgress = {
  watched: number
  total: number
  percent: number
  specialsWatched: number
  specialsTotal: number
  nextEpisode?: TVEpisode
}

export type MediaItem = {
  id: string
  providerId: number
  title: string
  year?: number
  releaseDate?: string
  firstAired?: string
  developer?: string
  director?: string
  network?: string
  gameType?: string
  runtimeMinutes?: number
  genres: string[]
  communityRating?: number
  artworkUrl?: string
  state: PersonalState
  progress?: TVProgress
}

export type ProviderSearchResult = {
  providerId: number
  title: string
  year?: number
  developer?: string
  director?: string
  network?: string
  gameType?: string
  artworkUrl?: string
  localId?: string
  localState?: string
}

export type ProviderSearchPage = {
  results: ProviderSearchResult[]
  page: number
  hasMore: boolean
}

export type Artwork = {
  providerImageId: string
  kind: "poster" | "cover" | "backdrop" | "logo"
  language?: string
  imageUrl: string
  thumbnailUrl: string
  preferred: boolean
  available: boolean
}

export type Person = { name: string; role: string; profileUrl?: string; imageUrl?: string }
export type ExternalLink = { label: string; url: string }
export type RelatedMedia = {
  providerId: number
  title: string
  relationship?: string
  type?: string
  year?: number
  coverUrl?: string
  posterUrl?: string
  localId?: string
  localStatus?: MediaStatus
  localRating?: number
  releaseDate?: string
}
export type TVEpisode = {
  id: string
  providerId: number
  seasonNumber: number
  episodeNumber: number
  title: string
  overview?: string
  airDate?: string
  runtimeMinutes?: number
  stillUrl?: string
  special: boolean
  watched: boolean
}
export type TVSeason = {
  id: string
  providerId: number
  number: number
  name?: string
  special: boolean
  airDate?: string
  posterUrl?: string
  watched: number
  total: number
  episodes: TVEpisode[]
}

export type MediaDetail = MediaItem & {
  originalTitle?: string
  summary?: string
  overview?: string
  publisher?: string
  providerStatus?: string
  gameModes?: string[]
  platforms?: string[]
  franchise?: string
  productionCompanies?: string[]
  communityRatingCount?: number
  screenshots?: string[]
  additionalContent?: RelatedMedia[]
  relatedReleases?: RelatedMedia[]
  externalLinks?: ExternalLink[]
  cast?: Person[]
  crew?: Person[]
  keyPeople?: Person[]
  trailerKey?: string
  imdbId?: string
  homepage?: string
  collectionId?: number
  collectionName?: string
  collection?: RelatedMedia[]
  verifiedTmdbId?: number
  seasons?: TVSeason[]
  artworks: Artwork[]
  artworkPins: Record<string, string>
  unavailablePins: string[]
  metadataRefreshedAt: string
}

export type IntegrationView = {
  provider: "igdb" | "tmdb" | "tvdb"
  enabled: boolean
  configured: boolean
  state: "not_configured" | "disabled" | "configured" | "connected" | "error"
  clientId?: string
  secretConfigured: boolean
  pinConfigured?: boolean
  lastTest?: { provider: string; status: string; message: string; testedAt: string }
}

export type IntegrationConfiguration = {
  enabled: boolean
  clientId: string
  secret: string
  removeSecret: boolean
  pin: string
  removePin: boolean
}

export type LibraryPreferences = {
  defaultLibrarySort: string
  preferredView: "grid" | "list"
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

export function getIntegrations() {
  return apiRequest<{ integrations: IntegrationView[] }>("/admin/integrations")
}

export function configureIntegration(
  provider: string,
  configuration: IntegrationConfiguration,
) {
  return apiRequest<IntegrationView>(`/admin/integrations/${provider}`, {
    method: "PUT",
    body: JSON.stringify(configuration),
  })
}

export function testIntegration(provider: string) {
  return apiRequest<{ status: string; message: string; testedAt: string }>(
    `/admin/integrations/${provider}/test`,
    { method: "POST" },
  )
}

export function getLibraryPreferences() {
  return apiRequest<LibraryPreferences>("/preferences/library")
}

export function updateLibraryPreferences(value: LibraryPreferences) {
  return apiRequest<LibraryPreferences>("/preferences/library", {
    method: "PUT",
    body: JSON.stringify(value),
  })
}

export function getMediaItems(domain: MediaDomain, backlog: boolean) {
  return apiRequest<{ items: MediaItem[] }>(
    `/${domain}/?view=${backlog ? "backlog" : "library"}`,
  )
}

export function searchProvider(
  domain: MediaDomain,
  query: string,
  page: number,
) {
  return apiRequest<ProviderSearchPage>(
    `/${domain}/search?q=${encodeURIComponent(query)}&page=${page}`,
  )
}

export function addMedia(
  domain: MediaDomain,
  providerId: number,
  status: MediaStatus,
) {
  return apiRequest<MediaDetail>(`/${domain}/`, {
    method: "POST",
    body: JSON.stringify({ providerId, status }),
  })
}

export function getMediaDetail(domain: MediaDomain, id: string) {
  return apiRequest<MediaDetail>(`/${domain}/${encodeURIComponent(id)}`)
}

export function updateMediaState(
  domain: MediaDomain,
  id: string,
  state: Pick<PersonalState, "status" | "rating" | "ratingReason">,
  confirmRatingClear = false,
) {
  return apiRequest<PersonalState>(
    `/${domain}/${encodeURIComponent(id)}/state`,
    {
      method: "PATCH",
      body: JSON.stringify({ ...state, confirmRatingClear }),
    },
  )
}

export function selectMediaArtwork(
  domain: MediaDomain,
  id: string,
  kind: string,
  providerImageId: string,
) {
  return apiRequest<MediaDetail>(
    `/${domain}/${encodeURIComponent(id)}/artwork/${encodeURIComponent(kind)}`,
    { method: "PUT", body: JSON.stringify({ providerImageId }) },
  )
}

export function refreshMedia(domain: MediaDomain, id: string) {
  return apiRequest<MediaDetail>(`/${domain}/${encodeURIComponent(id)}/refresh`, {
    method: "POST",
  })
}

export async function removeMedia(domain: MediaDomain, id: string) {
  const response = await fetch(`/api/${domain}/${encodeURIComponent(id)}`, {
    method: "DELETE",
    credentials: "same-origin",
    headers: sessionCSRFToken ? { "X-CSRF-Token": sessionCSRFToken } : {},
  })
  if (!response.ok) {
    let body: APIErrorResponse = {}
    try { body = (await response.json()) as APIErrorResponse } catch { /* ignored */ }
    throw new APIError(body.message ?? "The item could not be removed.", response.status, body.error)
  }
}

export function setEpisodeWatched(
  id: string,
  episodeId: string,
  watched: boolean,
) {
  return apiRequest<MediaDetail>(
    `/tv/${encodeURIComponent(id)}/episodes/${encodeURIComponent(episodeId)}`,
    { method: "PUT", body: JSON.stringify({ watched }) },
  )
}

export function setSeasonWatched(id: string, season: number, watched: boolean) {
  return apiRequest<MediaDetail>(
    `/tv/${encodeURIComponent(id)}/seasons/${season}`,
    { method: "PUT", body: JSON.stringify({ watched }) },
  )
}

export function setThroughEpisode(id: string, episodeId: string) {
  return apiRequest<MediaDetail>(
    `/tv/${encodeURIComponent(id)}/progress/through/${encodeURIComponent(episodeId)}`,
    { method: "POST" },
  )
}

export function setAllRegularWatched(id: string, watched: boolean) {
  return apiRequest<MediaDetail>(`/tv/${encodeURIComponent(id)}/progress/regular`, {
    method: "PUT",
    body: JSON.stringify({ watched }),
  })
}
