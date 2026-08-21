import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../../services/api/client'
import { ApiClientError } from '../../types/api'

const ALL_FORMATS = ['markdown', 'docx', 'json'] as const

export function UploadPage() {
  const navigate = useNavigate()
  const [file, setFile] = useState<File | null>(null)
  const [formats, setFormats] = useState<string[]>(['markdown', 'docx', 'json'])
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [dragOver, setDragOver] = useState(false)
  const [quota, setQuota] = useState<{ tier: string; limit: number; remaining: number } | null>(null)

  useEffect(() => {
    api
      .uploadQuota()
      .then((q) => setQuota({ tier: q.tier, limit: q.limit, remaining: q.remaining }))
      .catch(() => undefined)
  }, [])

  function toggleFormat(f: string) {
    setFormats((prev) => (prev.includes(f) ? prev.filter((x) => x !== f) : [...prev, f]))
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!file) {
      setError('Choose a PDF file')
      return
    }
    if (!file.name.toLowerCase().endsWith('.pdf')) {
      setError('Only PDF uploads are accepted')
      return
    }
    if (formats.length === 0) {
      setError('Select at least one output format')
      return
    }
    setBusy(true)
    setError('')
    try {
      const res = await api.uploadDocument(file, formats, setProgress)
      if (typeof res.quota_remaining === 'number') {
        setQuota((prev) =>
          prev ? { ...prev, remaining: res.quota_remaining as number } : { tier: 'anonymous', limit: 3, remaining: res.quota_remaining as number },
        )
      }
      navigate(`/processing/${res.document_id}`)
    } catch (err) {
      if (err instanceof ApiClientError && err.code === 'QUOTA_EXCEEDED') {
        setError('Upload limit reached. Sign in to get up to 10 uploads.')
        setQuota((prev) => (prev ? { ...prev, remaining: 0 } : prev))
      } else {
        setError(err instanceof Error ? err.message : 'Upload failed')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="page narrow">
      <div className="page-head">
        <div>
          <h1>Upload</h1>
          <p>Drop a PDF, choose outputs, and start processing.</p>
          {quota ? (
            <p className="muted">
              {quota.remaining} of {quota.limit} uploads remaining
              {quota.tier === 'anonymous' ? (
                <>
                  {' '}
                  — <Link to="/login">Sign in</Link> for up to 10.
                </>
              ) : null}
            </p>
          ) : null}
        </div>
      </div>
      <form className="upload-form" onSubmit={onSubmit}>
        <div
          className={`dropzone ${dragOver ? 'active' : ''}`}
          onDragOver={(e) => {
            e.preventDefault()
            setDragOver(true)
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault()
            setDragOver(false)
            const f = e.dataTransfer.files?.[0]
            if (f) setFile(f)
          }}
        >
          <p>{file ? file.name : 'Drag & drop PDF here'}</p>
          <input
            type="file"
            accept="application/pdf,.pdf"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </div>
        <fieldset>
          <legend>Output formats</legend>
          {ALL_FORMATS.map((f) => (
            <label key={f} className="check">
              <input type="checkbox" checked={formats.includes(f)} onChange={() => toggleFormat(f)} />
              {f}
            </label>
          ))}
        </fieldset>
        {progress > 0 && busy ? <div className="bar"><span style={{ width: `${progress}%` }} /></div> : null}
        {error ? <p className="error">{error}</p> : null}
        <button type="submit" disabled={busy || quota?.remaining === 0}>
          {busy ? 'Uploading…' : quota?.remaining === 0 ? 'Upload limit reached' : 'Start processing'}
        </button>
      </form>
    </section>
  )
}
