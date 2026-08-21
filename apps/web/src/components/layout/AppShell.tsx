import { Navigate, Outlet, NavLink, useLocation } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'

const guestRoutePrefixes = ['/upload', '/processing/', '/documents/', '/editor/']

function isGuestRoute(pathname: string) {
  return guestRoutePrefixes.some((prefix) => pathname === prefix.replace(/\/$/, '') || pathname.startsWith(prefix))
}

export function AppShell() {
  const { user, ready, logout } = useAuth()
  const location = useLocation()
  const guestMode = !user && isGuestRoute(location.pathname)

  if (!ready) return <div className="boot">Loading DocForge…</div>
  if (!user && !guestMode) return <Navigate to="/login" replace />

  return (
    <div className="shell">
      <header className="topbar">
        <NavLink to={user ? '/' : '/upload'} className="brand">
          DocForge
        </NavLink>
        <nav className="nav">
          {user ? <NavLink to="/">Dashboard</NavLink> : null}
          <NavLink to="/upload">Upload</NavLink>
          {user ? <NavLink to="/settings">Settings</NavLink> : null}
        </nav>
        <div className="userbox">
          {user ? (
            <>
              <span>{user.name}</span>
              <button type="button" className="ghost" onClick={logout}>
                Sign out
              </button>
            </>
          ) : (
            <NavLink to="/login" className="ghost">
              Sign in
            </NavLink>
          )}
        </div>
      </header>
      {!user ? (
        <p className="guest-banner">
          Guest mode — up to 3 free uploads. <NavLink to="/login">Sign in</NavLink> for up to 10.
        </p>
      ) : null}
      <main className="main">
        <Outlet />
      </main>
    </div>
  )
}
