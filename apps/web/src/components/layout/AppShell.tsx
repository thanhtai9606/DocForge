import { Navigate, Outlet, NavLink } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'

export function AppShell() {
  const { user, ready, logout } = useAuth()
  if (!ready) return <div className="boot">Loading DocForge…</div>
  if (!user) return <Navigate to="/login" replace />

  return (
    <div className="shell">
      <header className="topbar">
        <NavLink to="/" className="brand">
          DocForge
        </NavLink>
        <nav className="nav">
          <NavLink to="/">Dashboard</NavLink>
          <NavLink to="/upload">Upload</NavLink>
          <NavLink to="/settings">Settings</NavLink>
        </nav>
        <div className="userbox">
          <span>{user.name}</span>
          <button type="button" className="ghost" onClick={logout}>
            Sign out
          </button>
        </div>
      </header>
      <main className="main">
        <Outlet />
      </main>
    </div>
  )
}
