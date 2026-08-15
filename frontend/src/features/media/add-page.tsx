import { useCallback, useEffect, useRef, useState } from "react"
import { ArrowLeft, Check, LoaderCircle, Search, X } from "lucide-react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"

import { addMedia, searchProvider, type MediaDomain, type MediaStatus, type ProviderSearchResult } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/features/media/modal"
import { statuses } from "@/features/media/format"

export function AddPage({ domain, title }: { domain: MediaDomain; title: string }) {
  const navigate = useNavigate()
  const [urlParams] = useSearchParams()
  const [query, setQuery] = useState(urlParams.get("q") ?? "")
  const [results, setResults] = useState<ProviderSearchResult[]>([])
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<ProviderSearchResult | null>(null)
  const [added, setAdded] = useState<{ id: string; title: string } | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const searchGeneration = useRef(0)

  const search = useCallback(async (value: string, nextPage = 1) => {
    const generation = ++searchGeneration.current
    setLoading(true); setError(null)
    try {
      const response = await searchProvider(domain, value, nextPage)
      if (generation !== searchGeneration.current) return
      setResults((current) => nextPage === 1 ? response.results : [...current, ...response.results])
      setPage(response.page); setHasMore(response.hasMore)
    } catch (cause) {
      if (generation !== searchGeneration.current) return
      setError(cause instanceof Error ? cause.message : "The provider search failed.")
      if (nextPage === 1) setResults([])
    } finally {
      if (generation === searchGeneration.current) setLoading(false)
    }
  }, [domain])
  useEffect(() => {
    const value = query.trim()
    if (value.length < 2) return
    const timer = window.setTimeout(() => void search(value), 350)
    return () => window.clearTimeout(timer)
  }, [query, search])

  function clearSearch() {
    searchGeneration.current++
    setQuery("")
    setResults([])
    setError(null)
    setHasMore(false)
    setPage(1)
    setLoading(false)
    inputRef.current?.focus()
  }

  return (
    <section className="mx-auto max-w-4xl space-y-6">
      <header><Button asChild variant="ghost" className="-ml-3"><Link to={`/${domain}`}><ArrowLeft />Back to {title}</Link></Button><h1 className="mt-3 text-2xl font-semibold">Add {title === "TV Shows" ? "a TV show" : title === "Movies" ? "a movie" : "a game"}</h1><p className="mt-1 text-sm text-muted-foreground">Search {domain === "games" ? "IGDB" : domain === "movies" ? "TMDB" : "TVDB"}. Nothing is saved until you confirm an initial status.</p></header>
      <label className="relative block"><span className="sr-only">Search provider</span><Search className="pointer-events-none absolute left-3 top-3 size-5 text-muted-foreground" /><Input ref={inputRef} className="h-11 px-11 text-base" autoFocus value={query} onChange={(event)=>{const value=event.target.value;setQuery(value);if(value.trim().length<2){setResults([]);setError(null);setHasMore(false)}}} placeholder={`Search for ${title.toLowerCase()}…`} />{query!==""&&<button type="button" aria-label="Clear search" className="absolute right-1.5 top-1.5 grid size-8 place-items-center rounded-md text-muted-foreground outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring" onClick={clearSearch}><X aria-hidden="true" className="size-4" /></button>}</label>
      {query.trim().length<2&&<p className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">Enter at least two characters to search.</p>}
      {loading&&page===1&&<div className="space-y-3">{Array.from({length:5},(_,index)=><div key={index} className="h-24 animate-pulse rounded-lg bg-muted" />)}</div>}
      {error&&<div role="alert" className="rounded-lg border bg-card p-5"><p className="text-sm">{error}</p><Button className="mt-3" type="button" variant="outline" onClick={()=>void search(query.trim())}>Try again</Button></div>}
      {!loading&&!error&&query.trim().length>=2&&results.length===0&&<div className="rounded-lg border bg-card p-8 text-center"><h2 className="font-medium">No results</h2><p className="mt-1 text-sm text-muted-foreground">Try a more specific title or different spelling.</p></div>}
      <div className="space-y-3">{results.map((result)=><SearchRow key={result.providerId} result={result} domain={domain} choose={()=>setSelected(result)} />)}</div>
      {hasMore&&<div className="text-center"><Button type="button" variant="outline" disabled={loading} onClick={()=>void search(query.trim(),page+1)}>{loading&&<LoaderCircle className="animate-spin" />}{loading?"Loading…":"Load more"}</Button></div>}
      {selected&&<InitialStatusDialog domain={domain} result={selected} close={()=>setSelected(null)} added={(id)=>{setSelected(null);setAdded({id,title:selected.title})}} />}
      {added&&<div role="status" className="fixed bottom-5 right-5 z-40 flex max-w-sm items-center gap-3 rounded-lg border bg-background p-4 shadow-lg"><span className="grid size-8 place-items-center rounded-full bg-emerald-100 text-emerald-700"><Check className="size-4" /></span><div className="min-w-0"><p className="truncate text-sm font-medium">Added {added.title}</p><button type="button" className="text-sm text-muted-foreground underline" onClick={()=>navigate(`/${domain}/${added.id}`)}>View details</button></div><Button type="button" size="sm" variant="ghost" onClick={()=>setAdded(null)}>Dismiss</Button></div>}
    </section>
  )
}

function SearchRow({result,domain,choose}:{result:ProviderSearchResult;domain:MediaDomain;choose:()=>void}){
  const metadata=[result.year,result.developer??result.director??result.network,result.gameType].filter(Boolean).join(" · ")
  return <article className="grid grid-cols-[4rem_minmax(0,1fr)_auto] items-center gap-4 rounded-lg border bg-card p-3 shadow-xs"><div className="h-20 overflow-hidden rounded bg-muted">{result.artworkUrl&&<img src={result.artworkUrl} alt="" className="size-full object-cover" />}</div><div className="min-w-0"><h2 className="truncate font-medium">{result.title}</h2><p className="mt-1 truncate text-sm text-muted-foreground">{metadata||"No additional details"}</p></div>{result.localId?<Button asChild variant="outline" size="sm"><Link to={`/${domain}/${result.localId}`}>{result.localState}</Link></Button>:<Button type="button" size="sm" onClick={choose}>Add</Button>}</article>
}

function InitialStatusDialog({domain,result,close,added}:{domain:MediaDomain;result:ProviderSearchResult;close:()=>void;added:(id:string)=>void}){
  const [status,setStatus]=useState<MediaStatus>("backlog");const[saving,setSaving]=useState(false);const[error,setError]=useState<string|null>(null)
  async function confirm(){setSaving(true);setError(null);try{const item=await addMedia(domain,result.providerId,status);added(item.id)}catch(cause){setError(cause instanceof Error?cause.message:"The item could not be added.");setSaving(false)}}
  return <Modal title={`Add ${result.title}`} description="Choose the initial personal status. You can change it later." close={close}><div className="space-y-5"><div><label htmlFor="initial-status" className="text-sm font-medium">Initial status</label><select id="initial-status" className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={status} onChange={(event)=>setStatus(event.target.value as MediaStatus)}>{statuses.map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></div>{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}<div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={close}>Cancel</Button><Button type="button" disabled={saving} onClick={()=>void confirm()}>{saving&&<LoaderCircle className="animate-spin" />}{saving?"Adding…":"Add to Gradeium"}</Button></div></div></Modal>
}
