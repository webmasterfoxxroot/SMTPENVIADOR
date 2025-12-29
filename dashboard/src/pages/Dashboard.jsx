import { useState, useEffect } from 'react'
import {
  Send,
  CheckCircle,
  XCircle,
  Eye,
  MousePointer,
  Server,
  Mail,
  Zap,
  TrendingUp
} from 'lucide-react'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import api from '../services/api'

function StatCard({ icon: Icon, label, value, color, subValue }) {
  return (
    <div className="card">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-sm text-gray-500">{label}</p>
          <p className="text-3xl font-bold mt-1">{value}</p>
          {subValue && (
            <p className="text-sm text-gray-400 mt-1">{subValue}</p>
          )}
        </div>
        <div className={`p-3 rounded-lg ${color}`}>
          <Icon className="w-6 h-6 text-white" />
        </div>
      </div>
    </div>
  )
}

function Dashboard() {
  const [stats, setStats] = useState({
    today_sent: 0,
    today_failed: 0,
    today_opened: 0,
    today_clicked: 0,
    queue_size: 0,
    sending_rate: 0,
    active_smtps: 0,
    active_campaigns: 0,
    total_smtps: 0,
    total_lists: 0,
    total_emails: 0,
    hourly: []
  })
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchStats()
    const interval = setInterval(fetchStats, 5000)
    return () => clearInterval(interval)
  }, [])

  const fetchStats = async () => {
    try {
      const response = await api.get('/stats')
      setStats(response.data)
    } catch (error) {
      console.error('Failed to fetch stats:', error)
    } finally {
      setLoading(false)
    }
  }

  const formatNumber = (num) => {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
    return num?.toString() || '0'
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>
        <div className="flex items-center gap-2 text-sm text-gray-500">
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
          Atualizando em tempo real
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard
          icon={Send}
          label="Enviados Hoje"
          value={formatNumber(stats.today_sent)}
          color="bg-blue-500"
          subValue={`${stats.sending_rate}/seg`}
        />
        <StatCard
          icon={CheckCircle}
          label="Taxa de Sucesso"
          value={stats.today_sent > 0
            ? Math.round((stats.today_sent / (stats.today_sent + stats.today_failed)) * 100) + '%'
            : '0%'
          }
          color="bg-green-500"
        />
        <StatCard
          icon={Eye}
          label="Aberturas"
          value={formatNumber(stats.today_opened)}
          color="bg-purple-500"
          subValue={stats.today_sent > 0
            ? `${Math.round((stats.today_opened / stats.today_sent) * 100)}% taxa`
            : '0% taxa'
          }
        />
        <StatCard
          icon={MousePointer}
          label="Cliques"
          value={formatNumber(stats.today_clicked)}
          color="bg-orange-500"
          subValue={stats.today_opened > 0
            ? `${Math.round((stats.today_clicked / stats.today_opened) * 100)}% CTR`
            : '0% CTR'
          }
        />
      </div>

      {/* Second Row */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard
          icon={Zap}
          label="Na Fila"
          value={formatNumber(stats.queue_size)}
          color="bg-yellow-500"
        />
        <StatCard
          icon={Server}
          label="SMTPs Ativos"
          value={`${stats.active_smtps}/${stats.total_smtps}`}
          color="bg-gray-500"
        />
        <StatCard
          icon={TrendingUp}
          label="Campanhas Ativas"
          value={stats.active_campaigns}
          color="bg-indigo-500"
        />
        <StatCard
          icon={Mail}
          label="Total de Emails"
          value={formatNumber(stats.total_emails)}
          color="bg-teal-500"
          subValue={`${stats.total_lists} listas`}
        />
      </div>

      {/* Chart */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-4">Envios por Hora (Hoje)</h2>
        <div className="h-80">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={stats.hourly || []}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="hour" tickFormatter={(h) => `${h}h`} />
              <YAxis />
              <Tooltip />
              <Line
                type="monotone"
                dataKey="sent"
                stroke="#3b82f6"
                strokeWidth={2}
                name="Enviados"
              />
              <Line
                type="monotone"
                dataKey="opened"
                stroke="#8b5cf6"
                strokeWidth={2}
                name="Abertos"
              />
              <Line
                type="monotone"
                dataKey="clicked"
                stroke="#f97316"
                strokeWidth={2}
                name="Cliques"
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

export default Dashboard
