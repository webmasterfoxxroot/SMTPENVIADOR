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
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, BarChart, Bar, Legend } from 'recharts'
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
    hourly: [],
    by_provider: [],
    by_domain: []
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

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
        {/* Hourly Chart */}
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Envios por Hora (Hoje)</h2>
          <div className="h-72">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={stats.hourly || []}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="hour" tickFormatter={(h) => `${h}h`} />
                <YAxis />
                <Tooltip />
                <Legend />
                <Line
                  type="monotone"
                  dataKey="sent"
                  stroke="#3b82f6"
                  strokeWidth={2}
                  name="Enviados"
                />
                <Line
                  type="monotone"
                  dataKey="failed"
                  stroke="#ef4444"
                  strokeWidth={2}
                  name="Falhos"
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

        {/* Provider Stats Chart */}
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Envios por Provedor (Hoje)</h2>
          <div className="h-72">
            {(stats.by_provider || []).length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={stats.by_provider || []} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" />
                  <YAxis dataKey="provider" type="category" width={120} tick={{ fontSize: 12 }} />
                  <Tooltip />
                  <Legend />
                  <Bar dataKey="sent" fill="#3b82f6" name="Enviados" />
                  <Bar dataKey="failed" fill="#ef4444" name="Falhos" />
                  <Bar dataKey="opened" fill="#8b5cf6" name="Abertos" />
                  <Bar dataKey="clicked" fill="#f97316" name="Cliques" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-400">
                <p>Nenhum dado de provedor disponível</p>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Provider Stats Table */}
      {(stats.by_provider || []).length > 0 && (
        <div className="card mb-8">
          <h2 className="text-lg font-semibold mb-4">Detalhes por Servidor SMTP</h2>
          <table className="table">
            <thead>
              <tr>
                <th>Servidor SMTP</th>
                <th className="text-right">Enviados</th>
                <th className="text-right">Falhos</th>
                <th className="text-right">Abertos</th>
                <th className="text-right">Cliques</th>
                <th className="text-right">Taxa Sucesso</th>
                <th className="text-right">Taxa Abertura</th>
              </tr>
            </thead>
            <tbody>
              {(stats.by_provider || []).map((provider, index) => (
                <tr key={index}>
                  <td className="font-medium">{provider.provider}</td>
                  <td className="text-right text-blue-600">{provider.sent?.toLocaleString()}</td>
                  <td className="text-right text-red-600">{provider.failed?.toLocaleString()}</td>
                  <td className="text-right text-purple-600">{provider.opened?.toLocaleString()}</td>
                  <td className="text-right text-orange-600">{provider.clicked?.toLocaleString()}</td>
                  <td className="text-right">
                    {provider.sent + provider.failed > 0
                      ? Math.round((provider.sent / (provider.sent + provider.failed)) * 100) + '%'
                      : '-'}
                  </td>
                  <td className="text-right">
                    {provider.sent > 0
                      ? Math.round((provider.opened / provider.sent) * 100) + '%'
                      : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Email Domain Stats */}
      {(stats.by_domain || []).length > 0 && (
        <>
          {/* Domain Chart */}
          <div className="card mb-8">
            <h2 className="text-lg font-semibold mb-4">Envios por Domínio de Email (Hoje)</h2>
            <div className="h-80">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={stats.by_domain || []} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" />
                  <YAxis dataKey="domain" type="category" width={140} tick={{ fontSize: 11 }} />
                  <Tooltip />
                  <Legend />
                  <Bar dataKey="sent" fill="#3b82f6" name="Enviados" />
                  <Bar dataKey="opened" fill="#8b5cf6" name="Abertos" />
                  <Bar dataKey="clicked" fill="#f97316" name="Cliques" />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Domain Stats Table */}
          <div className="card overflow-x-auto">
            <h2 className="text-lg font-semibold mb-4">Detalhes por Domínio de Email</h2>
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200">
                  <th className="text-left py-3 px-2 font-semibold text-gray-600 w-48">DOMÍNIO</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-24">ENVIADOS</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-20">FALHOS</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-20">ABERTOS</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-20">CLIQUES</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-24">SUCESSO</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-24">ABERTURA</th>
                  <th className="text-center py-3 px-2 font-semibold text-gray-600 w-20">CTR</th>
                </tr>
              </thead>
              <tbody>
                {(stats.by_domain || []).map((domain, index) => (
                  <tr key={index} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="py-3 px-2 font-medium text-gray-800">{domain.domain}</td>
                    <td className="py-3 px-2 text-center">
                      <span className="inline-block min-w-[50px] text-blue-600 font-medium">
                        {domain.sent?.toLocaleString() || 0}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className="inline-block min-w-[50px] text-red-600 font-medium">
                        {domain.failed?.toLocaleString() || 0}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className="inline-block min-w-[50px] text-purple-600 font-medium">
                        {domain.opened?.toLocaleString() || 0}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className="inline-block min-w-[50px] text-orange-600 font-medium">
                        {domain.clicked?.toLocaleString() || 0}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className={`inline-block min-w-[50px] px-2 py-1 rounded text-xs font-medium ${
                        domain.sent + domain.failed > 0
                          ? (domain.sent / (domain.sent + domain.failed)) >= 0.9 ? 'bg-green-100 text-green-700' : 'bg-yellow-100 text-yellow-700'
                          : 'bg-gray-100 text-gray-500'
                      }`}>
                        {domain.sent + domain.failed > 0
                          ? Math.round((domain.sent / (domain.sent + domain.failed)) * 100) + '%'
                          : '-'}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className={`inline-block min-w-[50px] px-2 py-1 rounded text-xs font-medium ${
                        domain.sent > 0 && domain.opened > 0 ? 'bg-purple-100 text-purple-700' : 'bg-gray-100 text-gray-500'
                      }`}>
                        {domain.sent > 0
                          ? Math.round((domain.opened / domain.sent) * 100) + '%'
                          : '-'}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-center">
                      <span className={`inline-block min-w-[50px] px-2 py-1 rounded text-xs font-medium ${
                        domain.opened > 0 && domain.clicked > 0 ? 'bg-orange-100 text-orange-700' : 'bg-gray-100 text-gray-500'
                      }`}>
                        {domain.opened > 0
                          ? Math.round((domain.clicked / domain.opened) * 100) + '%'
                          : '-'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}

export default Dashboard
