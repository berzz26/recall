import { Link, NavLink } from 'react-router-dom'

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="layout">
      <nav className="nav">
        <Link to="/" className="logo">ReCall</Link>
        <NavLink to="/" end>Dashboard</NavLink>
        <NavLink to="/videos">Videos</NavLink>
        <NavLink to="/local-sources">Local Sources</NavLink>
        <span className="spacer" />
        <a href="http://localhost:8080/health" target="_blank" rel="noreferrer">API</a>
      </nav>
      <main className="main">{children}</main>
    </div>
  )
}
