import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  Server,
  Mail,
  Send,
  FileText,
  Ban,
  LogOut,
  Zap,
  Settings
} from 'lucide-react'
import { removeToken, getUser } from '../services/auth'

function Layout({ onLogout }) {
  const navigate = useNavigate()
  const user = getUser()

  const handleLogout = () => {
    removeToken()
    onLogout()
    navigate('/login')
  }

  const menuItems = [
    { path: '/', icon: LayoutDashboard, label: 'Dashboard' },
    { path: '/smtp', icon: Server, label: 'Servidores SMTP' },
    { path: '/lists', icon: Mail, label: 'Listas de Emails' },
    { path: '/campaigns', icon: Send, label: 'Campanhas' },
    { path: '/templates', icon: FileText, label: 'Templates' },
    { path: '/blacklist', icon: Ban, label: 'Blacklist' },
    { path: '/settings', icon: Settings, label: 'Configuracoes' },
  ]

  return (
    <div className="flex min-h-screen">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-800 text-white flex flex-col">
        <div className="p-4 border-b border-gray-700">
          <div className="flex items-center gap-2">
            <Zap className="w-8 h-8 text-blue-400" />
            <span className="text-xl font-bold">SMTPENVIADOR</span>
          </div>
        </div>

        <nav className="flex-1 p-4 space-y-1">
          {menuItems.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              end={item.path === '/'}
              className={({ isActive }) =>
                `sidebar-link ${isActive ? 'active' : ''}`
              }
            >
              <item.icon className="w-5 h-5" />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="p-4 border-t border-gray-700">
          <div className="text-sm text-gray-400 mb-2">
            {user?.email || 'admin@admin.com'}
          </div>
          <button
            onClick={handleLogout}
            className="sidebar-link w-full text-red-400 hover:bg-red-900 hover:text-red-300"
          >
            <LogOut className="w-5 h-5" />
            Sair
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 p-8 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}

export default Layout
