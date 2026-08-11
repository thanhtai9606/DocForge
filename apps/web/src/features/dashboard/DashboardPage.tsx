import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../../services/api/client'

export function DashboardPage() {
  const qc = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['documents'],
    queryFn: () => api.listDocuments(),
    refetchInterval: 5000,
  })
  const del = useMutation({
    mutationFn: (id: string) => api.deleteDocument(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents'] }),
  })
  const retry = useMutation({
    mutationFn: async (documentId: string) => {
      const job = await api.latestJob(documentId)
      return api.retryJob(job.job_id)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents'] }),
  })

  const docs = data?.documents ?? []

  return (
    <section className="page">
      <div className="page-head">
        <div>
          <h1>Dashboard</h1>
          <p>Recent documents and processing jobs.</p>
        </div>
        <Link className="btn" to="/upload">
          Upload PDF
        </Link>
      </div>

      {isLoading ? <p>Loading…</p> : null}
      {error ? <p className="error">{(error as Error).message}</p> : null}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>File</th>
              <th>Status</th>
              <th>Pages</th>
              <th>Formats</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {docs.map((d) => (
              <tr key={d.document_id}>
                <td>{d.filename}</td>
                <td>
                  <span className={`status status-${d.status}`}>{d.status}</span>
                </td>
                <td>{d.page_count || '—'}</td>
                <td>{d.output_formats?.join(', ')}</td>
                <td>{new Date(d.created_at).toLocaleString()}</td>
                <td className="actions">
                  <Link to={`/documents/${d.document_id}`}>View</Link>
                  <Link to={`/processing/${d.document_id}`}>Progress</Link>
                  <Link to={`/editor/${d.document_id}`}>Edit</Link>
                  {(d.status === 'failed' || d.status === 'cancelled') && (
                    <button type="button" className="linkish" onClick={() => retry.mutate(d.document_id)}>
                      Retry
                    </button>
                  )}
                  <button type="button" className="linkish danger" onClick={() => del.mutate(d.document_id)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
            {!isLoading && docs.length === 0 ? (
              <tr>
                <td colSpan={6}>No documents yet. Upload a PDF to start.</td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  )
}
