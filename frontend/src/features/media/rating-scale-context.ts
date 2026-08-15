import { createContext, useContext } from "react"

import type { RatingScale } from "@/api/client"

export const RatingScaleContext = createContext<RatingScale>("1_10")

export function useRatingScale() {
  return useContext(RatingScaleContext)
}
