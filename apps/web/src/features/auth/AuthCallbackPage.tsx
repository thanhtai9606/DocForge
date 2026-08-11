import { useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { setToken } from '../../services/api/client'
import { useAuth } from '../../app/providers/AuthProvider'

export function AuthCallbackPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { refresh } = useAuth()

  useEffect(() => {
    const token = params.get('token')
    if (!token) {
      navigate('/login', { replace: true })
      return
    }
    setToken(token)
    void refresh().then(() => navigate('/', { replace: true }))
  }, [params, navigate, refresh])

  return <div className="boot">Completing sign-in…</div>
}
