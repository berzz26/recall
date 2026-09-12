import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import Dashboard from './pages/Dashboard'
import Videos from './pages/Videos'
import VideoDetail from './pages/VideoDetail'
import LocalSources from './pages/LocalSources'
import Search from './pages/Search'

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/videos" element={<Videos />} />
          <Route path="/videos/:id" element={<VideoDetail />} />
          <Route path="/search" element={<Search />} />
          <Route path="/local-sources" element={<LocalSources />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  )
}
