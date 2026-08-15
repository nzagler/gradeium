import type { RatingScale } from "@/api/client"

export const ratingScaleOptions: { value: RatingScale; label: string; example: string }[] = [
  { value: "1_10", label: "1 to 10", example: "8.7" },
  { value: "0_5", label: "0 to 5", example: "4.35" },
  { value: "minus5_plus5", label: "-5 to +5", example: "+3.7" },
  { value: "0_100", label: "0 to 100", example: "87" },
]

export function formatPersonalRating(value: number, scale: RatingScale) {
  switch (scale) {
    case "0_5":
      return trimDecimal(value / 20, 2)
    case "minus5_plus5": {
      const display = value / 10 - 5
      return `${display >= 0 ? "+" : ""}${display.toFixed(1)}`
    }
    case "0_100":
      return Math.round(value).toString()
    default:
      return (value / 10).toFixed(1)
  }
}

export function ratingScaleLabel(scale: RatingScale) {
  return ratingScaleOptions.find((option) => option.value === scale)?.label ?? "1 to 10"
}

export function ratingStepLabel(scale: RatingScale) {
  if (scale === "0_5") return "0.05"
  if (scale === "0_100") return "1"
  return "0.1"
}

export function minimumSelectableCanonicalRating(scale: RatingScale) {
  return scale === "1_10" ? 10 : 0
}

export function canonicalFromDisplay(value: number, scale: RatingScale) {
  const canonical = scale === "0_5"
    ? value * 20
    : scale === "minus5_plus5"
      ? (value + 5) * 10
      : scale === "0_100"
        ? value
        : value * 10
  return Math.max(0, Math.min(100, Math.round(canonical)))
}

export function parseDisplayRating(value: string, scale: RatingScale) {
  const parsed = Number(value.trim())
  if (!Number.isFinite(parsed) || parsed < displayMinimum(scale) || parsed > displayMaximum(scale)) {
    return undefined
  }
  return canonicalFromDisplay(parsed, scale)
}

export function displayFromCanonical(value: number, scale: RatingScale) {
  if (scale === "0_5") return value / 20
  if (scale === "minus5_plus5") return value / 10 - 5
  if (scale === "0_100") return value
  return value / 10
}

export function rollRatingDigit(current: number, digit: string, scale: RatingScale) {
  const precision = scale === "0_100" ? 0 : scale === "0_5" ? 2 : 1
  const negative = scale === "minus5_plus5" && displayFromCanonical(current, scale) < 0
  let digits = ratingDigits(current, scale, precision) + digit
  while (digits.length > 1) {
    const display = Number(digits) / 10 ** precision * (negative ? -1 : 1)
    if (display >= displayMinimum(scale) && display <= displayMaximum(scale)) {
      return canonicalFromDisplay(display, scale)
    }
    digits = digits.slice(1)
  }
  const display = Number(digits) / 10 ** precision * (negative ? -1 : 1)
  return canonicalFromDisplay(Math.max(displayMinimum(scale), Math.min(displayMaximum(scale), display)), scale)
}

export function eraseRatingDigit(current: number, scale: RatingScale) {
  const precision = scale === "0_100" ? 0 : scale === "0_5" ? 2 : 1
  const negative = scale === "minus5_plus5" && displayFromCanonical(current, scale) < 0
  const digits = ratingDigits(current, scale, precision).slice(0, -1) || "0"
  return canonicalFromDisplay(Number(digits) / 10 ** precision * (negative ? -1 : 1), scale)
}

function ratingDigits(current: number, scale: RatingScale, precision: number) {
  return Math.round(Math.abs(displayFromCanonical(current, scale)) * 10 ** precision)
    .toString()
    .padStart(precision + 1, "0")
}

export function ratingDistributionLabel(key: string, scale: RatingScale) {
  const bucket = Number(key)
  if (!Number.isFinite(bucket)) return key
  const start = bucket * 10
  const end = Math.min(100, start + 9)
  return start === end
    ? formatPersonalRating(start, scale)
    : `${formatPersonalRating(start, scale)}–${formatPersonalRating(end, scale)}`
}

function displayMinimum(scale: RatingScale) {
  if (scale === "1_10") return 1
  return scale === "minus5_plus5" ? -5 : 0
}

function displayMaximum(scale: RatingScale) {
  if (scale === "0_5" || scale === "minus5_plus5") return 5
  if (scale === "0_100") return 100
  return 10
}

function trimDecimal(value: number, precision: number) {
  return Number(value.toFixed(precision)).toString()
}
