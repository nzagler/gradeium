import { useCallback, useEffect, useMemo, useState } from "react"
import { Grid2X2, List, Plus, Search, SlidersHorizontal, Star } from "lucide-react"
import { Link, useLocation, useSearchParams } from "react-router-dom"

import { getLibraryPreferences, getMediaItems, type MediaDomain, type MediaItem, type PersonalState } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/features/media/modal"
import { RatingButton } from "@/features/media/rating-editor"
import { formatRuntime, sortItems, sortOptions, statusLabels, statuses } from "@/features/media/format"
import { formatPersonalRating } from "@/features/media/rating-scale"
import { useRatingScale } from "@/features/media/rating-scale-context"

export function LibraryPage({ domain, title, backlog }: { domain: MediaDomain; title: string; backlog: boolean }) {
  const [params, setParams] = useSearchParams()
  const location = useLocation()
  const [items, setItems] = useState<MediaItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const search = params.get("q") ?? ""
  const status = params.get("status") ?? ""
  const rated = params.get("rated") ?? ""
  const year = params.get("year") ?? ""
  const genre = params.get("genre") ?? ""
  const sort = params.get("sort") ?? (backlog ? "added_desc" : "rating_desc")
  const view = params.get("view") ?? "grid"

  const load = useCallback(async () => {
    setLoading(true); setError(null)
    try { setItems((await getMediaItems(domain, backlog)).items) }
    catch (cause) { setError(cause instanceof Error ? cause.message : `The ${title.toLowerCase()} could not be loaded.`) }
    finally { setLoading(false) }
  }, [backlog, domain, title])
  useEffect(() => {
    let cancelled = false
    getMediaItems(domain, backlog)
      .then((response) => { if (!cancelled) setItems(response.items) })
      .catch((cause: unknown) => { if (!cancelled) setError(cause instanceof Error ? cause.message : `The ${title.toLowerCase()} could not be loaded.`) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [domain, backlog, title])
  useEffect(() => {
    if (params.has("sort") || params.has("view")) return
    void getLibraryPreferences().then((preferences) => {
      const next = new URLSearchParams(params)
      if (!backlog) next.set("sort", preferences.defaultLibrarySort)
      next.set("view", preferences.preferredView)
      setParams(next, { replace: true })
    }).catch(() => undefined)
  }, [backlog, params, setParams])
  useEffect(() => {
    const saved = sessionStorage.getItem(`gradeium:scroll:${location.pathname}${location.search}`)
    if (saved && !loading) { requestAnimationFrame(() => window.scrollTo(0, Number(saved))); sessionStorage.removeItem(`gradeium:scroll:${location.pathname}${location.search}`) }
  }, [loading, location.pathname, location.search])

  function updateParam(key: string, value: string) {
    const next = new URLSearchParams(params)
    if (value) next.set(key, value); else next.delete(key)
    setParams(next, { replace: true })
  }
  function rememberPosition() {
    const returnTo = `${location.pathname}${location.search}`
    sessionStorage.setItem(`gradeium:return:${domain}`, returnTo)
    sessionStorage.setItem(`gradeium:scroll:${returnTo}`, String(window.scrollY))
  }
  const genres = useMemo(() => [...new Set(items.flatMap((item) => item.genres))].sort(), [items])
  const years = useMemo(() => [...new Set(items.map((item) => item.year).filter((value): value is number => value !== undefined))].sort((a,b) => b-a), [items])
  const visible = useMemo(() => sortItems(items.filter((item) => {
    if (search && !item.title.toLowerCase().includes(search.toLowerCase())) return false
    if (status && item.state.status !== status) return false
    if (rated === "rated" && item.state.rating === undefined) return false
    if (rated === "unrated" && item.state.rating !== undefined) return false
    if (year && item.year !== Number(year)) return false
    if (genre && !item.genres.includes(genre)) return false
    return true
  }), sort), [items, search, status, rated, year, genre, sort])
  function stateSaved(id: string, state: PersonalState) { setItems((current) => current.map((item) => item.id === id ? { ...item, state } : item)) }
  const itemLabel = title.toLowerCase()
  return (
    <section className="space-y-6">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div><p className="text-sm font-medium text-muted-foreground">{title}</p><h1 className="text-2xl font-semibold tracking-tight">{backlog ? "Backlog" : "Library"}</h1><p className="mt-1 text-sm text-muted-foreground">{backlog ? `Items you plan to start later.` : `${items.length} tracked ${items.length === 1 ? "item" : "items"}.`}</p></div>
        <Button asChild><Link to={`/${domain}/add`}><Plus />Add {title === "TV Shows" ? "show" : title.slice(0,-1)}</Link></Button>
      </header>
      <div className="rounded-lg border bg-card p-4 shadow-xs">
        <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] gap-3 md:grid-cols-[minmax(12rem,1fr)_repeat(4,minmax(8rem,auto))_auto]">
          <label className="relative"><span className="sr-only">Search local {itemLabel}</span><Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input className="pl-9" value={search} onChange={(event) => updateParam("q",event.target.value)} placeholder={`Search ${backlog ? "backlog" : "library"}`} /></label>
          {!backlog && <select aria-label="Filter by status" className="hidden h-9 rounded-md border bg-background px-3 text-sm md:block" value={status} onChange={(event)=>updateParam("status",event.target.value)}><option value="">All statuses</option>{statuses.filter(([value])=>value!=="backlog").map(([value,label])=><option key={value} value={value}>{label}</option>)}</select>}
          {!backlog && <select aria-label="Filter by rating" className="hidden h-9 rounded-md border bg-background px-3 text-sm md:block" value={rated} onChange={(event)=>updateParam("rated",event.target.value)}><option value="">Rated and unrated</option><option value="rated">Rated</option><option value="unrated">Unrated</option></select>}
          <select aria-label="Filter by year" className="hidden h-9 rounded-md border bg-background px-3 text-sm md:block" value={year} onChange={(event)=>updateParam("year",event.target.value)}><option value="">All years</option>{years.map((value)=><option key={value}>{value}</option>)}</select>
          <select aria-label="Filter by genre" className="hidden h-9 rounded-md border bg-background px-3 text-sm md:block" value={genre} onChange={(event)=>updateParam("genre",event.target.value)}><option value="">All genres</option>{genres.map((value)=><option key={value}>{value}</option>)}</select>
          <Button type="button" variant="outline" size="icon" className="md:hidden" aria-label="Open filters" onClick={()=>setFiltersOpen(true)}><SlidersHorizontal /></Button>
          <div className="flex gap-1"><Button type="button" variant={view==="grid"?"default":"outline"} size="icon" aria-label="Grid view" onClick={()=>updateParam("view","grid")}><Grid2X2 /></Button><Button type="button" variant={view==="list"?"default":"outline"} size="icon" aria-label="List view" onClick={()=>updateParam("view","list")}><List /></Button></div>
        </div>
        <div className="mt-3 hidden items-center gap-2 md:flex"><SlidersHorizontal className="size-4 text-muted-foreground" /><label className="text-sm text-muted-foreground" htmlFor="library-sort">Sort</label><select id="library-sort" className="h-8 rounded-md border bg-background px-2 text-sm" value={sort} onChange={(event)=>updateParam("sort",event.target.value)}>{sortOptions.filter(([value])=>!backlog||!value.startsWith("rating")&&!value.startsWith("community")).map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></div>
      </div>
      {filtersOpen&&<Modal title="Filter and sort" description={`Refine this ${backlog?"Backlog":"Library"} view.`} close={()=>setFiltersOpen(false)}><div className="space-y-4">{!backlog&&<label className="block text-sm font-medium">Status<select className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={status} onChange={(event)=>updateParam("status",event.target.value)}><option value="">All statuses</option>{statuses.filter(([value])=>value!=="backlog").map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></label>}{!backlog&&<label className="block text-sm font-medium">Rating<select className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={rated} onChange={(event)=>updateParam("rated",event.target.value)}><option value="">Rated and unrated</option><option value="rated">Rated</option><option value="unrated">Unrated</option></select></label>}<label className="block text-sm font-medium">Year<select className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={year} onChange={(event)=>updateParam("year",event.target.value)}><option value="">All years</option>{years.map((value)=><option key={value}>{value}</option>)}</select></label><label className="block text-sm font-medium">Genre<select className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={genre} onChange={(event)=>updateParam("genre",event.target.value)}><option value="">All genres</option>{genres.map((value)=><option key={value}>{value}</option>)}</select></label><label className="block text-sm font-medium">Sort<select className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={sort} onChange={(event)=>updateParam("sort",event.target.value)}>{sortOptions.filter(([value])=>!backlog||!value.startsWith("rating")&&!value.startsWith("community")).map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></label><Button type="button" className="w-full" onClick={()=>setFiltersOpen(false)}>Show results</Button></div></Modal>}
      {loading && <div aria-label="Loading library" className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">{Array.from({length:10},(_,index)=><div key={index} className="aspect-[2/3] animate-pulse rounded-lg bg-muted" />)}</div>}
      {error && <div className="rounded-lg border bg-card p-6"><h2 className="font-semibold">Library unavailable</h2><p className="mt-1 text-sm text-muted-foreground">{error}</p><Button className="mt-4" type="button" variant="outline" onClick={()=>void load()}>Try again</Button></div>}
      {!loading&&!error&&visible.length===0&&<div className="rounded-lg border bg-card px-6 py-14 text-center"><h2 className="font-semibold">{items.length ? "No matches" : backlog ? `Your ${itemLabel} backlog is empty.` : `No ${itemLabel} yet.`}</h2><p className="mt-1 text-sm text-muted-foreground">{items.length ? "Try changing the current filters." : `Add your first ${title === "TV Shows" ? "show" : title.slice(0,-1).toLowerCase()} to get started.`}</p>{!items.length&&<Button className="mt-5" asChild><Link to={`/${domain}/add`}>Add {title === "TV Shows" ? "show" : title.slice(0,-1)}</Link></Button>}</div>}
      {!loading&&!error&&visible.length>0&&(view==="list"?<div className="overflow-hidden rounded-lg border bg-card">{visible.map((item)=><MediaListRow key={item.id} domain={domain} item={item} backlog={backlog} saveScroll={rememberPosition} saved={(state)=>stateSaved(item.id,state)} />)}</div>:<div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">{visible.map((item)=><MediaCard key={item.id} domain={domain} item={item} backlog={backlog} saveScroll={rememberPosition} saved={(state)=>stateSaved(item.id,state)} />)}</div>)}
    </section>
  )
}

function MediaCard({domain,item,backlog,saveScroll,saved}:{domain:MediaDomain;item:MediaItem;backlog:boolean;saveScroll:()=>void;saved:(state:PersonalState)=>void}){
  return <article className="group min-w-0"><Link to={`/${domain}/${item.id}`} onClick={saveScroll} className="block overflow-hidden rounded-lg border bg-muted shadow-xs outline-none transition hover:-translate-y-0.5 hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring"><div className="aspect-[2/3] bg-muted">{item.artworkUrl?<img src={item.artworkUrl} alt="" loading="lazy" className="size-full object-cover" />:<div className="grid size-full place-items-center text-xs text-muted-foreground">No artwork</div>}</div></Link><div className="pt-3"><Link to={`/${domain}/${item.id}`} onClick={saveScroll} className="line-clamp-2 font-medium leading-5 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{item.title}</Link><p className="mt-1 text-xs text-muted-foreground">{[item.year,domain==="movies"?formatRuntime(item.runtimeMinutes):undefined].filter(Boolean).join(" · ")}</p>{!backlog&&item.progress&&item.state.status==="in_progress"&&<div className="mt-2"><div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{width:`${item.progress.percent}%`}} /></div><p className="mt-1 text-xs text-muted-foreground">{item.progress.watched} of {item.progress.total} episodes</p></div>}<div className="mt-2 flex items-center justify-between gap-2"><span className="truncate text-xs text-muted-foreground">{backlog?"Backlog":statusLabels[item.state.status]}</span>{!backlog&&<RatingButton domain={domain} id={item.id} title={item.title} state={item.state} saved={saved} />}</div></div></article>
}
function MediaListRow({domain,item,backlog,saveScroll,saved}:{domain:MediaDomain;item:MediaItem;backlog:boolean;saveScroll:()=>void;saved:(state:PersonalState)=>void}){
  return <article className="grid grid-cols-[3rem_minmax(0,1fr)_auto] items-center gap-3 border-b p-3 last:border-b-0 sm:grid-cols-[3rem_minmax(0,1fr)_7rem_7rem_auto]"><Link to={`/${domain}/${item.id}`} onClick={saveScroll} className="h-16 overflow-hidden rounded border bg-muted">{item.artworkUrl&&<img src={item.artworkUrl} alt="" className="size-full object-cover" />}</Link><div className="min-w-0"><Link to={`/${domain}/${item.id}`} onClick={saveScroll} className="truncate font-medium hover:underline">{item.title}</Link><p className="text-xs text-muted-foreground sm:hidden">{item.year}</p></div><span className="hidden text-sm text-muted-foreground sm:block">{item.year??"—"}</span><span className="hidden text-sm text-muted-foreground sm:block">{statusLabels[item.state.status]}</span>{backlog?<span />:<RatingButton domain={domain} id={item.id} title={item.title} state={item.state} saved={saved} />}</article>
}

export function RatingSummary({value,label}:{value?:number;label:string}){const scale=useRatingScale();return <span className="inline-flex items-center gap-1 text-sm text-muted-foreground"><Star className="size-4" />{value!==undefined?`${formatPersonalRating(value,scale)} ${label}`:`No ${label} rating`}</span>}
