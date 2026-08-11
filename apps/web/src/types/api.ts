export type DocumentStatus = 'queued' | 'processing' | 'completed' | 'failed' | 'cancelled'
export type JobStatus = DocumentStatus

export interface DocumentItem {
  document_id: string
  filename: string
  content_type: string
  size_bytes: number
  page_count: number
  status: DocumentStatus
  output_formats: string[]
  created_at: string
  updated_at: string
}

export interface JobItem {
  job_id: string
  document_id: string
  status: JobStatus
  stage: string
  progress: number
  attempts?: number
  error?: { code: string; message: string } | null
  created_at?: string
  updated_at?: string
}

export interface ArtifactItem {
  artifact_id: string
  document_id: string
  job_id: string
  kind: string
  format: string
  filename: string
  size_bytes: number
  download_url: string
  created_at: string
}

export interface ApiError {
  code: string
  message: string
  retryable?: boolean
  request_id?: string
}

export class ApiClientError extends Error {
  code: string
  status: number
  retryable: boolean
  constructor(status: number, err: ApiError) {
    super(err.message)
    this.status = status
    this.code = err.code
    this.retryable = !!err.retryable
  }
}
