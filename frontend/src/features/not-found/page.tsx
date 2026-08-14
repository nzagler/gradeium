import { CircleAlert } from "lucide-react"

import { PlaceholderPage } from "@/components/shared/placeholder-page"

export function NotFoundPage() {
  return (
    <PlaceholderPage
      title="Page not found"
      description="The requested Gradeium page does not exist. Use the navigation to continue."
      icon={CircleAlert}
    />
  )
}
