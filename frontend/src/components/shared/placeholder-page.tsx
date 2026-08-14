import type { LucideIcon } from "lucide-react"

type PlaceholderPageProps = {
  title: string
  description: string
  icon: LucideIcon
}

export function PlaceholderPage({
  title,
  description,
  icon: Icon,
}: PlaceholderPageProps) {
  return (
    <section aria-labelledby="page-title">
      <div className="flex items-start gap-4">
        <span className="grid size-11 shrink-0 place-items-center rounded-lg border bg-card text-card-foreground shadow-xs">
          <Icon aria-hidden="true" className="size-5" />
        </span>
        <div className="max-w-2xl">
          <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
            {title}
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground sm:text-base">
            {description}
          </p>
        </div>
      </div>
    </section>
  )
}
