import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { useEffect, useMemo, useState } from 'react'
import { api, getToken } from '../../services/api/client'

export function DocumentViewerPage() {
  const { documentId = '' } = useParams()
  const docQ = useQuery({
    queryKey: ['document', documentId],
    queryFn: () => api.getDocument(documentId),
    enabled: !!documentId,
  })
  const artsQ = useQuery({
    queryKey: ['artifacts', documentId],
    queryFn: () => api.listArtifacts(documentId),
    enabled: !!documentId,
  })

  const cdomArt = artsQ.data?.artifacts.find((a) => a.kind === 'cdom' || a.format === 'json')
  const [cdomText, setCdomText] = useState('')
  useEffect(() => {
    if (!cdomArt) return
    api.fetchText(api.artifactDownloadUrl(cdomArt.artifact_id)).then(setCdomText).catch(() => setCdomText(''))
  }, [cdomArt?.artifact_id])

  const pdfUrl = useMemo(() => {
    const url = api.originalUrl(documentId)
    const token = getToken()
    return token ? `${url}?auth=1` : url
  }, [documentId])

  // Fetch PDF as blob with auth header for iframe
  const [pdfBlobUrl, setPdfBlobUrl] = useState<string | null>(null)
  useEffect(() => {
    let revoke: string | null = null
    ;(async () => {
      try {
        const headers = new Headers()
        const token = getToken()
        if (token) headers.set('Authorization', `Bearer ${token}`)
        const res = await fetch(api.originalUrl(documentId), { headers })
        if (!res.ok) return
        const blob = await res.blob()
        revoke = URL.createObjectURL(blob)
        setPdfBlobUrl(revoke)
      } catch {
        setPdfBlobUrl(null)
      }
    })()
    return () => {
      if (revoke) URL.revokeObjectURL(revoke)
    }
  }, [documentId])

  const exports = artsQ.data?.artifacts.filter((a) => a.kind === 'export') ?? []

  return (
    <section className="page">
      <div className="page-head">
        <div>
          <h1>{docQ.data?.filename ?? 'Document'}</h1>
          <p>PDF preview and CDOM structure.</p>
        </div>
        <div className="actions">
          <Link to={`/processing/${documentId}`}>Progress</Link>
          <Link to={`/editor/${documentId}`}>Markdown</Link>
        </div>
      </div>

      <div className="split">
        <div className="pane">
          <h2>PDF</h2>
          {pdfBlobUrl ? (
            <iframe title="PDF preview" src={pdfBlobUrl} className="pdf-frame" />
          ) : (
            <p className="muted">PDF preview unavailable ({pdfUrl})</p>
          )}
        </div>
        <div className="pane">
          <h2>CDOM / Artifacts</h2>
          <ul className="artifact-list">
            {exports.map((a) => (
              <li key={a.artifact_id}>
                <a href={api.artifactDownloadUrl(a.artifact_id)} download={a.filename}>
                  Download {a.filename}
                </a>
              </li>
            ))}
          </ul>
          <pre className="codeblock">{cdomText || 'No CDOM artifact yet.'}</pre>
        </div>
      </div>
    </section>
  )
}
