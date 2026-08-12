import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../services/api/client'

function escapeHtml(s: string) {
  return s
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

function renderMarkdownSafe(md: string) {
  // Minimal safe renderer — never injects raw HTML from source.
  const lines = md.split('\n')
  const html: string[] = []
  let inCode = false
  let listOpen = false
  const closeList = () => {
    if (listOpen) {
      html.push('</ul>')
      listOpen = false
    }
  }
  for (const raw of lines) {
    const line = raw.replace(/\r$/, '')
    if (line.startsWith('```')) {
      closeList()
      if (inCode) {
        html.push('</code></pre>')
        inCode = false
      } else {
        html.push('<pre><code>')
        inCode = true
      }
      continue
    }
    if (inCode) {
      html.push(`${escapeHtml(line)}\n`)
      continue
    }
    if (/^\s*[-*]\s+/.test(line)) {
      if (!listOpen) {
        html.push('<ul>')
        listOpen = true
      }
      html.push(`<li>${escapeHtml(line.replace(/^\s*[-*]\s+/, ''))}</li>`)
      continue
    }
    closeList()
    if (/^###\s+/.test(line)) html.push(`<h3>${escapeHtml(line.slice(4))}</h3>`)
    else if (/^##\s+/.test(line)) html.push(`<h2>${escapeHtml(line.slice(3))}</h2>`)
    else if (/^#\s+/.test(line)) html.push(`<h1>${escapeHtml(line.slice(2))}</h1>`)
    else if (line.trim() === '') html.push('<br/>')
    else html.push(`<p>${escapeHtml(line)}</p>`)
  }
  closeList()
  if (inCode) html.push('</code></pre>')
  return html.join('')
}

export function MarkdownEditorPage() {
  const { documentId = '' } = useParams()
  const qc = useQueryClient()
  const artsQ = useQuery({
    queryKey: ['artifacts', documentId],
    queryFn: () => api.listArtifacts(documentId, '?kind=export&format=markdown'),
    enabled: !!documentId,
  })
  const mdArt = artsQ.data?.artifacts.find((a) => a.format === 'markdown')
  const [source, setSource] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!mdArt) return
    api
      .fetchText(api.artifactDownloadUrl(mdArt.artifact_id))
      .then((t) => {
        setSource(t)
        setLoaded(true)
        setDirty(false)
      })
      .catch(() => setLoaded(true))
  }, [mdArt?.artifact_id])

  const preview = useMemo(() => renderMarkdownSafe(source), [source])

  const save = useMutation({
    mutationFn: () => api.saveMarkdown(documentId, source),
    onSuccess: () => {
      setDirty(false)
      setMessage('Saved to artifact store.')
      void qc.invalidateQueries({ queryKey: ['artifacts', documentId] })
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : 'Save failed'),
  })
  const regen = useMutation({
    mutationFn: () => api.regenerateMarkdown(documentId),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['artifacts', documentId] })
      if (mdArt) {
        const t = await api.fetchText(api.artifactDownloadUrl(mdArt.artifact_id))
        setSource(t)
        setDirty(false)
      }
      setMessage('Regenerated markdown from CDOM.')
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : 'Regenerate failed'),
  })

  function copy() {
    void navigator.clipboard.writeText(source)
  }

  function download() {
    const blob = new Blob([source], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = mdArt?.filename ?? 'document.md'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <section className="page">
      <div className="page-head">
        <div>
          <h1>Markdown editor</h1>
          <p>Source on the left, safe preview on the right.</p>
        </div>
        <div className="actions">
          <button type="button" className="ghost" onClick={() => save.mutate()} disabled={!mdArt || save.isPending}>
            {save.isPending ? 'Saving…' : dirty ? 'Save*' : 'Save'}
          </button>
          <button type="button" className="ghost" onClick={() => regen.mutate()} disabled={!mdArt || regen.isPending}>
            {regen.isPending ? 'Regenerating…' : 'Regenerate'}
          </button>
          <button type="button" className="ghost" onClick={copy} disabled={!source}>
            Copy
          </button>
          <button type="button" className="ghost" onClick={download} disabled={!source}>
            Download
          </button>
          <Link to={`/documents/${documentId}`}>Viewer</Link>
        </div>
      </div>
      {message ? <p className="muted">{message}</p> : null}
      {!loaded && <p>Loading markdown…</p>}
      <div className="split editor">
        <textarea
          className="editor-source"
          value={source}
          onChange={(e) => {
            setSource(e.target.value)
            setDirty(true)
          }}
          spellCheck={false}
          placeholder="Markdown will appear here after export completes."
        />
        <div className="editor-preview" dangerouslySetInnerHTML={{ __html: preview }} />
      </div>
    </section>
  )
}
