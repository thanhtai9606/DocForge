import type { ArtifactItem, DocumentItem, JobItem } from '../../types/api'
import { ApiClientError, type ApiError } from '../../types/api'

const TOKEN_KEY = 'docforge_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null) {
  if (!token) localStorage.removeItem(TOKEN_KEY)
  else localStorage.setItem(TOKEN_KEY, token)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type') && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(path, { ...init, headers })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const contentType = res.headers.get('content-type') ?? ''
  let data: unknown = {}
  if (text) {
    if (contentType.includes('application/json') || text.trimStart().startsWith('{') || text.trimStart().startsWith('[')) {
      try {
        data = JSON.parse(text)
      } catch {
        throw new Error(`API returned invalid JSON (${res.status})`)
      }
    } else if (!res.ok) {
      throw new Error(`API unavailable (${res.status}). Check that docforge-api is running and /api is proxied correctly.`)
    }
  }
  if (!res.ok) {
    const err = ((data as { error?: ApiError }).error ?? { code: 'INTERNAL_ERROR', message: res.statusText }) as ApiError
    throw new ApiClientError(res.status, err)
  }
  return data as T
}

export const api = {
  authProviders() {
    return request<{ providers: { id: string; name: string; login: string }[]; bypass: boolean }>(
      '/api/v1/auth/providers',
    )
  },
  me() {
    return request<{ user: { email: string; name: string; provider?: string } }>('/api/v1/auth/me')
  },
  logout() {
    return request<{ ok: boolean }>('/api/v1/auth/logout', { method: 'POST' })
  },
  listDocuments(limit = 50) {
    return request<{ documents: DocumentItem[] }>(`/api/v1/documents?limit=${limit}`)
  },
  getDocument(id: string) {
    return request<DocumentItem>(`/api/v1/documents/${id}`)
  },
  deleteDocument(id: string) {
    return request<void>(`/api/v1/documents/${id}`, { method: 'DELETE' })
  },
  uploadDocument(file: File, formats: string[], onProgress?: (pct: number) => void) {
    return new Promise<{ document_id: string; job_id: string; status: string }>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', '/api/v1/documents')
      const token = getToken()
      if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
      }
      xhr.onload = () => {
        try {
          const data = JSON.parse(xhr.responseText || '{}')
          if (xhr.status >= 200 && xhr.status < 300) resolve(data)
          else reject(new ApiClientError(xhr.status, data.error ?? { code: 'UPLOAD_FAILED', message: 'upload failed' }))
        } catch (err) {
          reject(err)
        }
      }
      xhr.onerror = () => reject(new ApiClientError(0, { code: 'NETWORK', message: 'network error' }))
      const form = new FormData()
      form.append('file', file)
      form.append('output_formats', formats.join(','))
      xhr.send(form)
    })
  },
  getJob(id: string) {
    return request<JobItem>(`/api/v1/jobs/${id}`)
  },
  latestJob(documentId: string) {
    return request<JobItem>(`/api/v1/documents/${documentId}/job`)
  },
  cancelJob(id: string) {
    return request<JobItem>(`/api/v1/jobs/${id}/cancel`, { method: 'POST' })
  },
  retryJob(id: string) {
    return request<JobItem>(`/api/v1/jobs/${id}/retry`, { method: 'POST' })
  },
  listArtifacts(documentId: string, query = '') {
    return request<{ artifacts: ArtifactItem[] }>(`/api/v1/documents/${documentId}/artifacts${query}`)
  },
  originalUrl(documentId: string) {
    return `/api/v1/documents/${documentId}/original`
  },
  artifactDownloadUrl(artifactId: string) {
    return `/api/v1/artifacts/${artifactId}/download`
  },
  async fetchText(url: string) {
    const headers = new Headers()
    const token = getToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)
    const res = await fetch(url, { headers })
    if (!res.ok) throw new Error('failed to load content')
    return res.text()
  },
  saveMarkdown(documentId: string, markdown: string) {
    return request<{ artifact_id: string; size_bytes: number; format: string }>(
      `/api/v1/documents/${documentId}/markdown`,
      { method: 'PUT', body: JSON.stringify({ markdown }) },
    )
  },
  regenerateMarkdown(documentId: string) {
    return request<{ artifact_id: string; size_bytes: number; format: string }>(
      `/api/v1/documents/${documentId}/markdown/regenerate`,
      { method: 'POST' },
    )
  },
  async subscribeJobEvents(
    jobId: string,
    onEvent: (ev: { type: string; job?: JobItem; error?: string }) => void,
    signal?: AbortSignal,
  ) {
    const headers = new Headers()
    const token = getToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)
    const res = await fetch(`/api/v1/jobs/${jobId}/events`, { headers, signal })
    if (!res.ok || !res.body) {
      throw new Error(`sse failed (${res.status})`)
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      const parts = buf.split('\n\n')
      buf = parts.pop() ?? ''
      for (const block of parts) {
        let type = 'message'
        let data = ''
        for (const line of block.split('\n')) {
          if (line.startsWith('event:')) type = line.slice(6).trim()
          if (line.startsWith('data:')) data += line.slice(5).trim()
        }
        if (!data) continue
        try {
          const parsed = JSON.parse(data) as JobItem & { message?: string }
          if (type === 'error') onEvent({ type, error: parsed.message ?? data })
          else onEvent({ type, job: parsed })
        } catch {
          onEvent({ type, error: data })
        }
      }
    }
  },
}
