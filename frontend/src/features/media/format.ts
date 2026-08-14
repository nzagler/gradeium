import type { MediaItem, MediaStatus } from "@/api/client"

export const statusLabels: Record<MediaStatus, string> = {
  backlog: "Backlog",
  in_progress: "In Progress",
  on_hold: "On Hold",
  abandoned: "Abandoned",
  completed: "Completed",
}

export const statuses = Object.entries(statusLabels) as [MediaStatus, string][]

export function formatRating(value?: number) {
  return value === undefined ? "" : (value / 10).toFixed(1)
}

export function formatRuntime(minutes?: number) {
  if (!minutes) return ""
  const hours = Math.floor(minutes / 60)
  const remainder = minutes % 60
  if (!hours) return `${remainder} min`
  return remainder ? `${hours}h ${remainder}m` : `${hours}h`
}

export function formatDate(value?: string) {
  if (!value) return ""
  const date = new Date(`${value.slice(0, 10)}T00:00:00Z`)
  if (Number.isNaN(date.valueOf())) return ""
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  }).format(date)
}

export const sortOptions = [
  ["rating_desc", "My Rating high to low"],
  ["rating_asc", "My Rating low to high"],
  ["community_desc", "Community Rating high to low"],
  ["title_asc", "Title A to Z"],
  ["title_desc", "Title Z to A"],
  ["release_desc", "Release date newest"],
  ["release_asc", "Release date oldest"],
  ["added_desc", "Date Added newest"],
  ["added_asc", "Date Added oldest"],
] as const

export function sortItems(items: MediaItem[], sort: string) {
  return [...items].sort((left, right) => {
    switch (sort) {
      case "rating_asc":
        return nullableNumber(left.state.rating, right.state.rating, false)
      case "community_desc":
        return nullableNumber(left.communityRating, right.communityRating, true)
      case "title_asc":
        return left.title.localeCompare(right.title)
      case "title_desc":
        return right.title.localeCompare(left.title)
      case "release_desc":
        return nullableNumber(releaseValue(left), releaseValue(right), true)
      case "release_asc":
        return nullableNumber(releaseValue(left), releaseValue(right), false)
      case "added_desc":
        return right.state.dateAdded.localeCompare(left.state.dateAdded)
      case "added_asc":
        return left.state.dateAdded.localeCompare(right.state.dateAdded)
      default:
        return nullableNumber(left.state.rating, right.state.rating, true)
    }
  })
}

function releaseValue(item: MediaItem) {
  const exact = item.releaseDate ?? item.firstAired
  if (exact) {
    const timestamp = Date.parse(exact)
    if (Number.isFinite(timestamp)) return timestamp
  }
  return item.year
}

function nullableNumber(left: number | undefined, right: number | undefined, desc: boolean) {
  if (left === undefined) return right === undefined ? 0 : 1
  if (right === undefined) return -1
  return desc ? right - left : left - right
}
