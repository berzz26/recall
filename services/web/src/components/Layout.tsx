import { Link, NavLink, useLocation } from 'react-router-dom'

function IconVideos({active}:{active:boolean}){
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={active?2:1.7} strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="3" width="18" height="14" rx="2"/><path d="M7 8l4 3-4 3z"/><path d="M16 8h2v6h-2z" opacity={active?1:0.7}/></svg>
}
function IconSearch(){
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><circle cx="11" cy="11" r="6"/><path d="M20 20L15.3 15.3"/></svg>
}
function IconSettings(){
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><circle cx="12" cy="12" r="3"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
}

export function Layout({ children }: { children: React.ReactNode }) {
  const loc = useLocation()
  const isVideos = loc.pathname.startsWith('/videos') || loc.pathname === '/'
  const isSearch = loc.pathname.startsWith('/search')
  const isSettings = loc.pathname.startsWith('/local-sources') || loc.pathname.startsWith('/settings')
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link to="/videos" className="sidebar-brand">
          <div className="brand-icon">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.4"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 12l10 5 10-5"/><path d="M2 17l10 5 10-5"/></svg>
          </div>
          <span>ReCall</span>
        </Link>
        <nav className="sidebar-nav">
          <NavLink to="/videos" className={() => `nav-item ${isVideos ? 'active' : ''}`}>
            <IconVideos active={isVideos}/><span>Videos</span>
          </NavLink>
          <NavLink to="/search" className={() => `nav-item ${isSearch ? 'active' : ''}`}>
            <IconSearch/><span>Search</span>
          </NavLink>
          <NavLink to="/local-sources" className={() => `nav-item ${isSettings ? 'active' : ''}`}>
            <IconSettings/><span>Settings</span>
          </NavLink>
        </nav>
      </aside>
      <div className="main-content">{children}</div>
    </div>
  )
}
