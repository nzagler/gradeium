import { useCallback, useEffect, useState, type FormEvent } from "react"
import { Archive, Download, FileDown, LoaderCircle, RotateCcw, Trash2, Upload } from "lucide-react"

import {
  createBackup,
  deleteBackup,
  downloadBackup,
  downloadRatingsCSV,
  getBackups,
  getBackupSettings,
  restoreBackup,
  restoreBackupUpload,
  updateBackupSettings,
  type BackupMetadata,
  type BackupSettings,
} from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Modal } from "@/features/media/modal"

type Confirmation =
  | { kind: "restore"; backup: BackupMetadata }
  | { kind: "delete"; backup: BackupMetadata }
  | { kind: "upload"; file: File }

export function BackupsPanel() {
  const [settings, setSettings] = useState<BackupSettings | null>(null)
  const [items, setItems] = useState<BackupMetadata[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null)
  const [confirmationText, setConfirmationText] = useState("")
  const [upload, setUpload] = useState<File | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [inventory, schedule] = await Promise.all([getBackups(), getBackupSettings()])
      setItems(inventory.backups)
      setSettings(schedule)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Backups could not be loaded.")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let active = true
    void Promise.all([getBackups(), getBackupSettings()])
      .then(([inventory, schedule]) => {
        if (active) {
          setItems(inventory.backups)
          setSettings(schedule)
        }
      })
      .catch((cause: unknown) => {
        if (active) setError(cause instanceof Error ? cause.message : "Backups could not be loaded.")
      })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  async function saveSchedule(event: FormEvent) {
    event.preventDefault()
    if (!settings) return
    setBusy("settings")
    setMessage(null)
    setError(null)
    try {
      setSettings(await updateBackupSettings(settings))
      setMessage("Automatic backup settings saved.")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Backup settings could not be saved.")
    } finally {
      setBusy(null)
    }
  }

  async function createNow() {
    setBusy("create")
    setMessage(null)
    setError(null)
    try {
      const created = await createBackup()
      setItems((current) => [created, ...current])
      setMessage("Portable backup created and validated.")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The backup could not be created.")
    } finally {
      setBusy(null)
    }
  }

  async function confirmAction() {
    if (!confirmation) return
    const required = confirmation.kind === "delete" ? "DELETE" : "RESTORE"
    if (confirmationText !== required) return
    setBusy(confirmation.kind)
    setError(null)
    setMessage(null)
    try {
      if (confirmation.kind === "delete") {
        await deleteBackup(confirmation.backup.id)
        setItems((current) => current.filter((item) => item.id !== confirmation.backup.id))
        setMessage("Backup deleted.")
      } else if (confirmation.kind === "restore") {
        const result = await restoreBackup(confirmation.backup.id)
        setMessage(`Restore completed. Safety backup ${result.safetyBackup.filename} was created first.`)
        await load()
      } else {
        const result = await restoreBackupUpload(confirmation.file)
        setMessage(`Uploaded backup restored. Safety backup ${result.safetyBackup.filename} was created first.`)
        setUpload(null)
        await load()
      }
      setConfirmation(null)
      setConfirmationText("")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The backup operation failed safely.")
    } finally {
      setBusy(null)
    }
  }

  if (loading && !settings) {
    return <div aria-label="Loading backups" className="h-72 animate-pulse rounded-lg bg-muted" />
  }

  return (
    <div className="space-y-6">
      {error && (
        <div role="alert" className="rounded-lg border border-destructive/40 bg-card p-4 text-sm text-destructive">
          {error}
          {!settings && <Button className="ml-3" type="button" variant="outline" size="sm" onClick={() => void load()}>Try again</Button>}
        </div>
      )}
      {message && <p role="status" className="rounded-md border bg-card p-3 text-sm text-muted-foreground">{message}</p>}

      {settings && (
        <section className="rounded-lg border bg-card shadow-xs">
          <div className="border-b px-5 py-4">
            <h2 className="font-semibold">Automatic backups</h2>
            <p className="mt-1 text-sm text-muted-foreground">Persistent-state scheduling continues across restarts and creates one overdue backup after downtime.</p>
          </div>
          <form className="grid gap-5 p-5 md:grid-cols-3" onSubmit={(event) => void saveSchedule(event)}>
            <label className="flex min-h-10 items-center gap-3 text-sm font-medium">
              <input type="checkbox" className="size-4" checked={settings.enabled} onChange={(event) => setSettings({ ...settings, enabled: event.target.checked })} />
              Enable automatic backups
            </label>
            <div>
              <Label htmlFor="backup-interval">Interval</Label>
              <select id="backup-interval" className="mt-2 h-10 w-full rounded-md border bg-background px-3 text-sm" value={settings.intervalDays} onChange={(event) => setSettings({ ...settings, intervalDays: Number(event.target.value) })}>
                <option value={1}>Daily</option><option value={3}>Every 3 days</option><option value={7}>Weekly</option><option value={14}>Every 2 weeks</option><option value={30}>Monthly</option>
              </select>
            </div>
            <div>
              <Label htmlFor="backup-retention">Automatic backups to retain</Label>
              <Input id="backup-retention" className="mt-2" type="number" min={1} max={365} value={settings.retentionCount} onChange={(event) => setSettings({ ...settings, retentionCount: Number(event.target.value) })} />
            </div>
            <div className="md:col-span-3 flex flex-wrap items-center gap-3">
              <Button type="submit" disabled={busy !== null}>{busy === "settings" && <LoaderCircle className="animate-spin" />}Save schedule</Button>
              <p className="text-xs text-muted-foreground">
                {settings.nextDueAt ? `Next expected ${formatDateTime(settings.nextDueAt)}.` : "Automatic backups are disabled."}
                {settings.lastSuccessfulAutomaticAt && ` Last successful ${formatDateTime(settings.lastSuccessfulAutomaticAt)}.`}
              </p>
            </div>
            {settings.lastError && <p role="alert" className="md:col-span-3 text-sm text-destructive">Last scheduler error: {settings.lastError}</p>}
          </form>
        </section>
      )}

      <section className="rounded-lg border bg-card shadow-xs">
        <div className="flex flex-col gap-3 border-b px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div><h2 className="font-semibold">Portable backups</h2><p className="mt-1 text-sm text-muted-foreground">Versioned JSON.gz files in the persistent backup mount. Secrets and session material are never included.</p></div>
          <Button type="button" onClick={() => void createNow()} disabled={busy !== null}><Archive />{busy === "create" ? "Creating…" : "Create now"}</Button>
        </div>
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">No completed backups yet.</p> : (
          <div className="divide-y">
            {items.map((item) => (
              <article key={item.id} className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{item.filename}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{kindLabel(item.kind)} · {formatDateTime(item.createdAt)} · {formatBytes(item.sizeBytes)} · Format v{item.formatVersion}</p>
                  <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={item.sha256}>SHA-256 {item.sha256}</p>
                  {!item.valid && <p className="mt-1 text-xs text-destructive">File missing or failed inventory validation.</p>}
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" disabled={!item.valid || busy !== null} onClick={() => void downloadBackup(item.id).catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "Download failed."))}><Download />Download</Button>
                  <Button type="button" variant="outline" size="sm" disabled={!item.valid || busy !== null} onClick={() => { setConfirmation({ kind: "restore", backup: item }); setConfirmationText("") }}><RotateCcw />Restore</Button>
                  <Button type="button" variant="outline" size="sm" disabled={busy !== null} onClick={() => { setConfirmation({ kind: "delete", backup: item }); setConfirmationText("") }} aria-label={`Delete ${item.filename}`}><Trash2 /></Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="grid gap-6 lg:grid-cols-2">
        <div className="rounded-lg border bg-card p-5 shadow-xs">
          <h2 className="font-semibold">Restore a downloaded backup</h2>
          <p className="mt-1 text-sm text-muted-foreground">Upload a Gradeium JSON.gz backup. It is fully validated before a safety backup and transactional restore.</p>
          <Input className="mt-4" type="file" accept=".gz,application/gzip,application/octet-stream" onChange={(event) => setUpload(event.target.files?.[0] ?? null)} aria-label="Choose Gradeium backup file" />
          <Button className="mt-3" type="button" variant="outline" disabled={!upload || busy !== null} onClick={() => upload && setConfirmation({ kind: "upload", file: upload })}><Upload />Validate and restore</Button>
        </div>
        <div className="rounded-lg border bg-card p-5 shadow-xs">
          <h2 className="font-semibold">Ratings CSV</h2>
          <p className="mt-1 text-sm text-muted-foreground">Download a UTF-8 CSV of current rated Library items, including rating reasons.</p>
          <Button className="mt-4" type="button" variant="outline" onClick={() => void downloadRatingsCSV().catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "CSV export failed."))}><FileDown />Download ratings CSV</Button>
        </div>
      </section>

      {confirmation && (
        <Modal title={confirmation.kind === "delete" ? "Delete backup" : "Restore portable data"} description={confirmation.kind === "delete" ? "This permanently removes the selected backup file." : "Current portable media state will be replaced transactionally. Gradeium creates a safety backup first and does not change authentication or secrets."} close={() => { if (!busy) { setConfirmation(null); setConfirmationText("") } }}>
          <div className="space-y-4">
            <p className="break-all rounded-md bg-muted p-3 text-xs">{confirmation.kind === "upload" ? confirmation.file.name : confirmation.backup.filename}</p>
            <div><Label htmlFor="backup-confirmation">Type {confirmation.kind === "delete" ? "DELETE" : "RESTORE"} to continue</Label><Input id="backup-confirmation" className="mt-2" autoComplete="off" value={confirmationText} onChange={(event) => setConfirmationText(event.target.value)} /></div>
            <div className="flex justify-end gap-2"><Button type="button" variant="outline" disabled={busy !== null} onClick={() => setConfirmation(null)}>Cancel</Button><Button type="button" variant={confirmation.kind === "delete" ? "destructive" : "default"} disabled={busy !== null || confirmationText !== (confirmation.kind === "delete" ? "DELETE" : "RESTORE")} onClick={() => void confirmAction()}>{busy && <LoaderCircle className="animate-spin" />}{confirmation.kind === "delete" ? "Delete backup" : "Create safety backup and restore"}</Button></div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function formatDateTime(value: string) { return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) }
function formatBytes(value: number) { if (value < 1024) return `${value} B`; if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`; return `${(value / 1024 / 1024).toFixed(1)} MB` }
function kindLabel(value: BackupMetadata["kind"]) { return value === "pre_restore" ? "Pre-restore safety" : value[0].toUpperCase() + value.slice(1) }
