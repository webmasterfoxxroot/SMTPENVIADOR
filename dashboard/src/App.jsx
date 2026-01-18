import { Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import SMTPServers from './pages/SMTPServers'
import Warmup from './pages/Warmup'
import EmailLists from './pages/EmailLists'
import Campaigns from './pages/Campaigns'
import CampaignEdit from './pages/CampaignEdit'
import CampaignDetails from './pages/CampaignDetails'
import Templates from './pages/Templates'
import Blacklist from './pages/Blacklist'
import Settings from './pages/Settings'
import Users from './pages/Users'
import Profile from './pages/Profile'
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
        <Route path="warmup" element={<Warmup />} />
        <Route path="lists" element={<EmailLists />} />
        <Route path="campaigns" element={<Campaigns />} />
        <Route path="campaigns/new" element={<CampaignEdit />} />
        <Route path="campaigns/:id" element={<CampaignEdit />} />
        <Route path="campaigns/:id/details" element={<CampaignDetails />} />
        <Route path="templates" element={<Templates />} />
        <Route path="blacklist" element={<Blacklist />} />
        <Route path="settings" element={<Settings />} />
        <Route path="users" element={<Users />} />
        <Route path="profile" element={<Profile />} />
      </Route>
    </Routes>
  )
}

export default App
