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
  Settings,
  Flame,
  Sun,
  Moon,
  Users,
  User
} from 'lucide-react'
import { removeToken, getUser } from '../services/auth'
import { useTheme } from '../contexts/ThemeContext'

function Layout({ onLogout }) {
  const navigate = useNavigate()
  const user = getUser()
  const { theme, toggleTheme } = useTheme()

  const handleLogout = () => {
    removeToken()
    onLogout()
    navigate('/login')
  }

  const menuItems = [
    { path: '/', icon: LayoutDashboard, label: 'Dashboard' },
    { path: '/smtp', icon: Server, label: 'Servidores SMTP' },
    { path: '/warmup', icon: Flame, label: 'Warmup' },
    { path: '/lists', icon: Mail, label: 'Listas de Emails' },
    { path: '/campaigns', icon: Send, label: 'Campanhas' },
    { path: '/templates', icon: FileText, label: 'Templates' },
    { path: '/blacklist', icon: Ban, label: 'Blacklist' },
    { path: '/settings', icon: Settings, label: 'Configuracoes' },
    ...(user?.role === 'admin' ? [{ path: '/users', icon: Users, label: 'Usuários' }] : []),
  ]

  return (
    <div className="flex min-h-screen bg-gray-100 dark:bg-gray-900 transition-colors duration-200">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-800 dark:bg-gray-950 text-white flex flex-col">
        <div className="p-4 border-b border-gray-700 dark:border-gray-800">
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

        {/* Theme Toggle */}
        <div className="px-4 py-3 border-t border-gray-700 dark:border-gray-800">
          <button
            onClick={toggleTheme}
            className="flex items-center gap-3 w-full px-4 py-3 text-gray-300 hover:bg-gray-700 hover:text-white rounded-lg transition-colors"
          >
            {theme === 'light' ? (
              <>
                <Moon className="w-5 h-5" />
                <span>Modo Escuro</span>
              </>
            ) : (
              <>
                <Sun className="w-5 h-5" />
                <span>Modo Claro</span>
              </>
            )}
          </button>
        </div>

        <div className="p-4 border-t border-gray-700 dark:border-gray-800 space-y-2">
          <NavLink
            to="/profile"
            className={({ isActive }) =>
              `flex items-center gap-3 px-4 py-2 rounded-lg transition-colors ${
                isActive
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-300 hover:bg-gray-700 hover:text-white'
              }`
            }
          >
            <User className="w-5 h-5" />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium truncate">{user?.name || 'Usuário'}</p>
              <p className="text-xs text-gray-400 truncate">{user?.email || 'admin@admin.com'}</p>
            </div>
          </NavLink>
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
