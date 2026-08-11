import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { api } from '../../services/api/client'

type Provider = { id: string; name: string; login: string }

export function LoginPage() {
  const { user, ready, continueBypass } = useAuth()
  const [providers, setProviders] = useState<Provider[]>([])
  const [bypass, setBypass] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .authProviders()
      .then((res) => {
        setProviders(res.providers ?? [])
        setBypass(!!res.bypass)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load SSO providers'))
  }, [])

  if (ready && user) return <Navigate to="/" replace />

  return (
    <div className="login-scene">
      <div className="login-glow" aria-hidden />
      <section className="login-panel">
        <p className="eyebrow">Private document intelligence</p>
        <h1 className="brand-hero">DocForge</h1>
        <p className="lede">Sign in with your organization account — Google or Microsoft SSO.</p>

        <div className="sso-stack">
          {providers.map((p) => (
            <a key={p.id} className={`sso-btn sso-${p.id}`} href={p.login}>
              Continue with {p.name}
            </a>
          ))}
          {providers.length === 0 && !bypass ? (
            <p className="muted">No SSO providers configured. Set Google/Microsoft OAuth client env vars on the API.</p>
          ) : null}
          {bypass ? (
            <button type="button" className="sso-btn sso-bypass" onClick={() => continueBypass()}>
              Continue in local bypass mode
            </button>
          ) : null}
        </div>
        {error ? <p className="error">{error}</p> : null}
      </section>
    </div>
  )
}
