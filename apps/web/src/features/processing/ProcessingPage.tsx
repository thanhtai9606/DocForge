import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { api } from '../../services/api/client'

export function ProcessingPage() {
  const { documentId = '' } = useParams()
  const docQ = useQuery({
    queryKey: ['document', documentId],
    queryFn: () => api.getDocument(documentId),
    enabled: !!documentId,
  })
  const jobQ = useQuery({
    queryKey: ['job-latest', documentId],
    queryFn: () => api.latestJob(documentId),
    enabled: !!documentId,
    refetchInterval: (q) => {
      const s = q.state.data?.status
      return s === 'queued' || s === 'processing' ? 1500 : false
    },
  })
  const cancel = useMutation({
    mutationFn: () => api.cancelJob(jobQ.data!.job_id),
    onSuccess: () => jobQ.refetch(),
  })

  const job = jobQ.data
  const started = job?.created_at ? new Date(job.created_at).getTime() : Date.now()
  const elapsedSec = Math.max(0, Math.round((Date.now() - started) / 1000))

  return (
    <section className="page narrow">
      <div className="page-head">
        <div>
          <h1>Processing</h1>
          <p>{docQ.data?.filename ?? documentId}</p>
        </div>
        <Link to={`/documents/${documentId}`}>Open document</Link>
      </div>

      {jobQ.isLoading ? <p>Loading job…</p> : null}
      {jobQ.error ? <p className="error">{(jobQ.error as Error).message}</p> : null}

      {job ? (
        <div className="progress-panel">
          <div className="progress-meta">
            <span className={`status status-${job.status}`}>{job.status}</span>
            <span>Stage: {job.stage}</span>
            <span>Elapsed: {elapsedSec}s</span>
            <span>Attempts: {job.attempts ?? 0}</span>
          </div>
          <div className="bar large">
            <span style={{ width: `${job.progress}%` }} />
          </div>
          <p className="pct">{job.progress}%</p>
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
