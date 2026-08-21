import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { api } from '../../services/api/client'
import type { JobItem } from '../../types/api'

export function ProcessingPage() {
  const { documentId = '' } = useParams()
  const [liveJob, setLiveJob] = useState<JobItem | null>(null)
  const [sseActive, setSseActive] = useState(false)

  const docQ = useQuery({
    queryKey: ['document', documentId],
    queryFn: () => api.getDocument(documentId),
    enabled: !!documentId,
  })
  const jobQ = useQuery({
    queryKey: ['job-latest', documentId],
    queryFn: () => api.latestJob(documentId),
    enabled: !!documentId && !sseActive,
    refetchInterval: (q) => {
      const s = q.state.data?.status
      return s === 'queued' || s === 'processing' ? 1500 : false
    },
  })

  useEffect(() => {
    if (!documentId) return
    const ac = new AbortController()
    let stopped = false
    ;(async () => {
      try {
        const latest = await api.latestJob(documentId)
        setLiveJob(latest)
        if (latest.status !== 'queued' && latest.status !== 'processing') return
        setSseActive(true)
        await api.subscribeJobEvents(
          latest.job_id,
          (ev) => {
            if (ev.job) setLiveJob(ev.job)
            if (ev.type === 'done') setSseActive(false)
          },
          ac.signal,
        )
      } catch {
        if (!stopped) setSseActive(false)
      } finally {
        if (!stopped) setSseActive(false)
      }
    })()
    return () => {
      stopped = true
      ac.abort()
      setSseActive(false)
    }
  }, [documentId])

  const cancel = useMutation({
    mutationFn: () => api.cancelJob((liveJob ?? jobQ.data)!.job_id),
    onSuccess: (job) => {
      setLiveJob(job)
      void jobQ.refetch()
    },
  })

  const job = liveJob ?? jobQ.data
  const started = job?.created_at ? new Date(job.created_at).getTime() : Date.now()
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (!job || (job.status !== 'queued' && job.status !== 'processing')) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [job?.status, job?.job_id])
  const elapsedSec = Math.max(0, Math.round((now - started) / 1000))

  return (
    <section className="page narrow">
      <div className="page-head">
        <div>
          <h1>Processing</h1>
          <p>{docQ.data?.filename ?? documentId}</p>
        </div>
        <Link to={`/documents/${documentId}`}>Open document</Link>
      </div>

      {jobQ.isLoading && !job ? <p>Loading job…</p> : null}
      {jobQ.error && !job ? <p className="error">{(jobQ.error as Error).message}</p> : null}

      {job ? (
        <div className="progress-panel">
          <div className="progress-meta">
            <span className={`status status-${job.status}`}>{job.status}</span>
            <span>Stage: {job.stage}</span>
            <span>Elapsed: {elapsedSec}s</span>
            <span>Attempts: {job.attempts ?? 0}</span>
            <span className="muted">{sseActive ? 'live SSE' : 'polling'}</span>
          </div>
          <div className="bar large">
            <span style={{ width: `${job.progress}%` }} />
          </div>
          <p className="pct">{job.progress}%</p>
          {job.stage === 'ocr' && job.status === 'processing' ? (
            <p className="muted">
              OCR scan pages — large PDFs can take several minutes. Progress updates as each page finishes.
            </p>
          ) : null}
          {job.error ? (
            <p className="error">
              {job.error.code}: {job.error.message}
            </p>
          ) : null}
          {(job.status === 'queued' || job.status === 'processing') && (
            <button type="button" className="ghost" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
              Cancel job
            </button>
          )}
          {job.status === 'completed' && (
            <div className="actions">
              <Link className="btn" to={`/documents/${documentId}`}>
                View results
              </Link>
              <Link className="btn secondary" to={`/editor/${documentId}`}>
                Markdown editor
              </Link>
            </div>
          )}
        </div>
      ) : null}
    </section>
  )
}
