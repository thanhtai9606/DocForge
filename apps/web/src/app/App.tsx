import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from './providers/AuthProvider'
import { AppShell } from '../components/layout/AppShell'
import { LoginPage } from '../features/auth/LoginPage'
import { AuthCallbackPage } from '../features/auth/AuthCallbackPage'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { UploadPage } from '../features/upload/UploadPage'
import { ProcessingPage } from '../features/processing/ProcessingPage'
import { DocumentViewerPage } from '../features/documents/DocumentViewerPage'
import { MarkdownEditorPage } from '../features/editor/MarkdownEditorPage'
import { SettingsPage } from '../features/settings/SettingsPage'

const qc = new QueryClient()

export function App() {
  return (
    <QueryClientProvider client={qc}>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/auth/callback" element={<AuthCallbackPage />} />
            <Route element={<AppShell />}>
              <Route index element={<DashboardPage />} />
              <Route path="upload" element={<UploadPage />} />
              <Route path="processing/:documentId" element={<ProcessingPage />} />
              <Route path="documents/:documentId" element={<DocumentViewerPage />} />
              <Route path="editor/:documentId" element={<MarkdownEditorPage />} />
              <Route path="settings" element={<SettingsPage />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}
