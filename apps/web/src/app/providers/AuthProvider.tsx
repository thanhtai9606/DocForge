import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, getToken, setToken } from '../../services/api/client'

type User = { email: string; name: string; provider?: string }

type AuthState = {
  user: User | null
  ready: boolean
  refresh: () => Promise<void>
  continueBypass: () => void
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [ready, setReady] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const me = await api.me()
      setUser(me.user)
    } catch {
      setToken(null)
      setUser(null)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        await refresh()
      } finally {
        if (!cancelled) setReady(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [refresh])

  const value = useMemo<AuthState>(
    () => ({
      user,
      ready,
      refresh,
      continueBypass() {
        // Bypass mode: API /me works without token when AUTH_BYPASS=1.
        setToken(null)
        void refresh()
      },
      logout() {
        setToken(null)
        setUser(null)
        void api.logout().catch(() => undefined)
      },
    }),
    [user, ready, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('AuthProvider missing')
  return ctx
}

export function useHasSessionToken() {
  return !!getToken()
}
