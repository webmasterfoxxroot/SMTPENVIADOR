import { Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import SMTPServers from './pages/SMTPServers'
import EmailLists from './pages/EmailLists'
import Campaigns from './pages/Campaigns'
import CampaignEdit from './pages/CampaignEdit'
import Templates from './pages/Templates'
import Blacklist from './pages/Blacklist'
import { getToken } from './services/auth'

function PrivateRoute({ children }) {
  const token = getToken()
  return token ? children : <Navigate to="/login" />
}

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(!!getToken())

  useEffect(() => {
    const handleStorageChange = () => {
      setIsAuthenticated(!!getToken())
    }
    window.addEventListener('storage', handleStorageChange)
    return () => window.removeEventListener('storage', handleStorageChange)
  }, [])

  return (
    <Routes>
      <Route path="/login" element={<Login onLogin={() => setIsAuthenticated(true)} />} />
      <Route path="/" element={
        <PrivateRoute>
          <Layout onLogout={() => setIsAuthenticated(false)} />
        </PrivateRoute>
      }>
        <Route index element={<Dashboard />} />
        <Route path="smtp" element={<SMTPServers />} />
        <Route path="lists" element={<EmailLists />} />
        <Route path="campaigns" element={<Campaigns />} />
        <Route path="campaigns/new" element={<CampaignEdit />} />
        <Route path="campaigns/:id" element={<CampaignEdit />} />
        <Route path="templates" element={<Templates />} />
        <Route path="blacklist" element={<Blacklist />} />
      </Route>
    </Routes>
  )
}

export default App
