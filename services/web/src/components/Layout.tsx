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
          <svg width="22" height="22" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
            <rect width="32" height="32" rx="7" fill="#0f172a"/>
            <rect width="32" height="32" rx="7" fill="none" stroke="white" strokeOpacity="0.06" strokeWidth="1"/>
            <circle cx="15.5" cy="15.2" r="6.2" fill="none" stroke="white" strokeWidth="1.7" strokeLinecap="round"/>
            <path d="M13.2 13.1 L13.2 17.8 L17.9 15.45 Z" fill="#10a37f" stroke="#10a37f" strokeLinejoin="round" strokeWidth="1.1"/>
            <path d="M19.7 19.4 L24.2 23.9" stroke="white" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"/>
            <circle cx="15.5" cy="15.2" r="8.2" fill="none" stroke="#10a37f" strokeWidth="1.35" strokeLinecap="round" strokeLinejoin="round" stroke-dasharray="46.9 4.6" stroke-dashoffset="0" transform="rotate(-32 15.5 15.2)" opacity="0.98"/>
            <path d="M 20.95 8.75 L 22.48 10.92 L 20.15 12.15" fill="none" stroke="#10a37f" strokeWidth="1.35" strokeLinecap="round" strokeLinejoin="round" opacity="0.98"/>
          </svg>
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
