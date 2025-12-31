import { useState, useEffect } from 'react'
import {
  Send,
  CheckCircle,
  Eye,
  MousePointer,
  Server,
  Mail,
  Zap,
  TrendingUp,
  ArrowUp,
  ArrowDown,
  Activity,
  Globe
} from 'lucide-react'
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell
} from 'recharts'
import api from '../services/api'

// Circular Progress Component
function CircularProgress({ value, size = 120, strokeWidth = 8, color = '#3b82f6' }) {
  const radius = (size - strokeWidth) / 2
  const circumference = radius * 2 * Math.PI
  const offset = circumference - (value / 100) * circumference

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="transform -rotate-90">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="#e5e7eb"
          strokeWidth={strokeWidth}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          className="transition-all duration-500"
        />
      </svg>
      <div className="absolute inset-0 flex items-center justify-center">
        <span className="text-2xl font-bold text-gray-800">{value}%</span>
      </div>
    </div>
  )
}

// Main Stat Card with gradient
function MainStatCard({ icon: Icon, label, value, subValue, gradient, trend }) {
  return (
    <div className={`relative overflow-hidden rounded-2xl p-6 text-white shadow-lg ${gradient}`}>
      <div className="absolute top-0 right-0 -mt-4 -mr-4 h-24 w-24 rounded-full bg-white/10"></div>
      <div className="absolute bottom-0 left-0 -mb-4 -ml-4 h-16 w-16 rounded-full bg-white/10"></div>

      <div className="relative">
        <div className="flex items-center justify-between">
          <div className="rounded-xl bg-white/20 p-3">
            <Icon className="h-6 w-6" />
          </div>
          {trend !== undefined && (
            <div className={`flex items-center gap-1 text-sm ${trend >= 0 ? 'text-green-200' : 'text-red-200'}`}>
              {trend >= 0 ? <ArrowUp className="h-4 w-4" /> : <ArrowDown className="h-4 w-4" />}
              {Math.abs(trend)}%
            </div>
          )}
        </div>

        <div className="mt-4">
          <p className="text-3xl font-bold">{value}</p>
          <p className="mt-1 text-sm text-white/80">{label}</p>
          {subValue && (
            <p className="mt-2 text-xs text-white/60">{subValue}</p>
          )}
        </div>
      </div>
    </div>
  )
}

// Small Stat Card
function SmallStatCard({ icon: Icon, label, value, subValue, iconBg }) {
  return (
    <div className="bg-white rounded-xl p-5 shadow-sm border border-gray-100 hover:shadow-md transition-shadow">
      <div className="flex items-center gap-4">
        <div className={`rounded-xl p-3 ${iconBg}`}>
          <Icon className="h-5 w-5 text-white" />
        </div>
        <div>
          <p className="text-2xl font-bold text-gray-800">{value}</p>
          <p className="text-sm text-gray-500">{label}</p>
          {subValue && (
            <p className="text-xs text-gray-400 mt-1">{subValue}</p>
          )}
        </div>
      </div>
    </div>
  )
}

// Custom Tooltip for Charts
function CustomTooltip({ active, payload, label }) {
  if (active && payload && payload.length) {
    const isHour = typeof label === 'number' && label < 24
    return (
      <div className="bg-white p-4 rounded-lg shadow-lg border border-gray-100">
        <p className="text-sm font-semibold text-gray-800 mb-2">{isHour ? `${label}h` : label}</p>
        {payload.map((entry, index) => (
          <p key={index} className="text-sm" style={{ color: entry.color }}>
            {entry.name}: {entry.value?.toLocaleString()}
          </p>
        ))}
      </div>
    )
  }
  return null
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
    by_domain: []
  })
  const [loading, setLoading] = useState(true)
  const [period, setPeriod] = useState('today')

  useEffect(() => {
    fetchStats()
    const interval = setInterval(fetchStats, 5000)
    return () => clearInterval(interval)
  }, [period])

  const fetchStats = async () => {
    try {
      const response = await api.get(`/stats?period=${period}`)
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
    return num?.toLocaleString() || '0'
  }

  const successRate = stats.today_sent + stats.today_failed > 0
    ? Math.round((stats.today_sent / (stats.today_sent + stats.today_failed)) * 100)
    : 0

  const openRate = stats.today_sent > 0
    ? Math.round((stats.today_opened / stats.today_sent) * 100)
    : 0

  const clickRate = stats.today_opened > 0
    ? Math.round((stats.today_clicked / stats.today_opened) * 100)
    : 0

  // Pie chart data for domain distribution
  const COLORS = ['#3b82f6', '#8b5cf6', '#ec4899', '#f97316', '#10b981', '#6366f1', '#14b8a6', '#f59e0b']
  const domainPieData = (stats.by_domain || []).slice(0, 8).map(d => ({
    name: d.domain,
    value: d.sent
  }))

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>
          <p className="text-sm text-gray-500 mt-1">Visao geral do desempenho de email marketing</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 bg-green-50 text-green-700 px-4 py-2 rounded-full text-sm font-medium">
            <Activity className="h-4 w-4" />
            <span>{stats.sending_rate}/seg</span>
          </div>
          <div className="flex items-center gap-2 bg-gray-100 text-gray-600 px-4 py-2 rounded-full text-sm">
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
            Tempo real
          </div>
        </div>
      </div>

      {/* Main Stats - Big Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
        <MainStatCard
          icon={Send}
          label="Emails Enviados"
          value={formatNumber(stats.today_sent)}
          gradient="bg-gradient-to-br from-blue-500 to-blue-600"
          subValue="Hoje"
        />
        <MainStatCard
          icon={Eye}
          label="Aberturas"
          value={formatNumber(stats.today_opened)}
          gradient="bg-gradient-to-br from-purple-500 to-purple-600"
          subValue={`${openRate}% taxa de abertura`}
        />
        <MainStatCard
          icon={MousePointer}
          label="Cliques"
          value={formatNumber(stats.today_clicked)}
          gradient="bg-gradient-to-br from-orange-500 to-orange-600"
          subValue={`${clickRate}% CTR`}
        />
        <MainStatCard
          icon={Zap}
          label="Na Fila"
          value={formatNumber(stats.queue_size)}
          gradient="bg-gradient-to-br from-emerald-500 to-emerald-600"
          subValue="Aguardando envio"
        />
      </div>

      {/* Secondary Stats Row */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <SmallStatCard
          icon={Server}
          label="SMTPs Ativos"
          value={`${stats.active_smtps}/${stats.total_smtps}`}
          iconBg="bg-gray-500"
        />
        <SmallStatCard
          icon={TrendingUp}
          label="Campanhas Ativas"
          value={stats.active_campaigns}
          iconBg="bg-indigo-500"
        />
        <SmallStatCard
          icon={Mail}
          label="Total de Emails"
          value={formatNumber(stats.total_emails)}
          subValue={`${stats.total_lists} listas`}
          iconBg="bg-teal-500"
        />
        <SmallStatCard
          icon={CheckCircle}
          label="Falhas Hoje"
          value={formatNumber(stats.today_failed)}
          iconBg="bg-red-500"
        />
      </div>

      {/* Charts Section */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Main Area Chart */}
        <div className="lg:col-span-2 bg-white rounded-2xl p-6 shadow-sm border border-gray-100">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h2 className="text-lg font-semibold text-gray-800">
                {period === 'today' ? 'Atividade por Hora' : period === 'week' ? 'Ultimos 7 Dias' : 'Ultimos 30 Dias'}
              </h2>
              <p className="text-sm text-gray-500">
                {period === 'today' ? 'Desempenho de hoje' : period === 'week' ? 'Desempenho semanal' : 'Desempenho mensal'}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <div className="flex bg-gray-100 rounded-lg p-1">
                <button
                  onClick={() => setPeriod('today')}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                    period === 'today'
                      ? 'bg-white text-blue-600 shadow-sm'
                      : 'text-gray-600 hover:text-gray-800'
                  }`}
                >
                  Hoje
                </button>
                <button
                  onClick={() => setPeriod('week')}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                    period === 'week'
                      ? 'bg-white text-blue-600 shadow-sm'
                      : 'text-gray-600 hover:text-gray-800'
                  }`}
                >
                  7 Dias
                </button>
                <button
                  onClick={() => setPeriod('month')}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                    period === 'month'
                      ? 'bg-white text-blue-600 shadow-sm'
                      : 'text-gray-600 hover:text-gray-800'
                  }`}
                >
                  30 Dias
                </button>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-4 text-sm mb-4">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-blue-500"></div>
              <span className="text-gray-600">Enviados</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-purple-500"></div>
              <span className="text-gray-600">Abertos</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-orange-500"></div>
              <span className="text-gray-600">Cliques</span>
            </div>
          </div>
          <div className="h-72">
            <ResponsiveContainer width="100%" height="100%">
              {period === 'today' ? (
                <AreaChart data={stats.hourly || []}>
                  <defs>
                    <linearGradient id="colorSent" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                    </linearGradient>
                    <linearGradient id="colorOpened" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#8b5cf6" stopOpacity={0.3}/>
                      <stop offset="95%" stopColor="#8b5cf6" stopOpacity={0}/>
                    </linearGradient>
                    <linearGradient id="colorClicked" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#f97316" stopOpacity={0.3}/>
                      <stop offset="95%" stopColor="#f97316" stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                  <XAxis
                    dataKey="hour"
                    tickFormatter={(val) => `${val}h`}
                    stroke="#9ca3af"
                    fontSize={12}
                  />
                  <YAxis stroke="#9ca3af" fontSize={12} />
                  <Tooltip content={<CustomTooltip />} />
                  <Area
                    type="monotone"
                    dataKey="sent"
                    stroke="#3b82f6"
                    strokeWidth={2}
                    fill="url(#colorSent)"
                    name="Enviados"
                  />
                  <Area
                    type="monotone"
                    dataKey="opened"
                    stroke="#8b5cf6"
                    strokeWidth={2}
                    fill="url(#colorOpened)"
                    name="Abertos"
                  />
                  <Area
                    type="monotone"
                    dataKey="clicked"
                    stroke="#f97316"
                    strokeWidth={2}
                    fill="url(#colorClicked)"
                    name="Cliques"
                  />
                </AreaChart>
              ) : (
                <BarChart data={stats.hourly || []}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                  <XAxis
                    dataKey="label"
                    stroke="#9ca3af"
                    fontSize={11}
                    angle={-45}
                    textAnchor="end"
                    height={60}
                  />
                  <YAxis stroke="#9ca3af" fontSize={12} />
                  <Tooltip content={<CustomTooltip />} />
                  <Bar dataKey="sent" fill="#3b82f6" name="Enviados" radius={[4, 4, 0, 0]} />
                  <Bar dataKey="opened" fill="#8b5cf6" name="Abertos" radius={[4, 4, 0, 0]} />
                  <Bar dataKey="clicked" fill="#f97316" name="Cliques" radius={[4, 4, 0, 0]} />
                </BarChart>
              )}
            </ResponsiveContainer>
          </div>
        </div>

        {/* Performance Rates */}
        <div className="bg-white rounded-2xl p-6 shadow-sm border border-gray-100">
          <h2 className="text-lg font-semibold text-gray-800 mb-2">Taxas de Performance</h2>
          <p className="text-sm text-gray-500 mb-6">Metricas do dia</p>

          <div className="space-y-8">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Taxa de Sucesso</p>
                <p className="text-xs text-gray-400 mt-1">Emails entregues</p>
              </div>
              <CircularProgress value={successRate} size={80} strokeWidth={6} color="#10b981" />
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Taxa de Abertura</p>
                <p className="text-xs text-gray-400 mt-1">Emails abertos</p>
              </div>
              <CircularProgress value={openRate} size={80} strokeWidth={6} color="#8b5cf6" />
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Taxa de Cliques</p>
                <p className="text-xs text-gray-400 mt-1">CTR</p>
              </div>
              <CircularProgress value={clickRate} size={80} strokeWidth={6} color="#f97316" />
            </div>
          </div>
        </div>
      </div>

      {/* Domain Stats Section */}
      {(stats.by_domain || []).length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Domain Pie Chart */}
          <div className="bg-white rounded-2xl p-6 shadow-sm border border-gray-100">
            <div className="flex items-center gap-2 mb-4">
              <Globe className="h-5 w-5 text-gray-500" />
              <h2 className="text-lg font-semibold text-gray-800">Distribuicao por Dominio</h2>
            </div>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={domainPieData}
                    cx="50%"
                    cy="50%"
                    innerRadius={50}
                    outerRadius={80}
                    paddingAngle={2}
                    dataKey="value"
                  >
                    {domainPieData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="grid grid-cols-2 gap-2 mt-4">
              {domainPieData.slice(0, 6).map((entry, index) => (
                <div key={index} className="flex items-center gap-2 text-xs">
                  <div
                    className="w-2 h-2 rounded-full"
                    style={{ backgroundColor: COLORS[index % COLORS.length] }}
                  ></div>
                  <span className="text-gray-600 truncate">{entry.name}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Domain Stats Table */}
          <div className="lg:col-span-2 bg-white rounded-2xl p-6 shadow-sm border border-gray-100">
            <h2 className="text-lg font-semibold text-gray-800 mb-4">Detalhes por Dominio</h2>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-gray-100">
                    <th className="text-left py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">Dominio</th>
                    <th className="text-center py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">Enviados</th>
                    <th className="text-center py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">Abertos</th>
                    <th className="text-center py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">Cliques</th>
                    <th className="text-center py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">Abertura</th>
                    <th className="text-center py-3 px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider">CTR</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {(stats.by_domain || []).slice(0, 10).map((domain, index) => {
                    const domainOpenRate = domain.sent > 0 ? Math.round((domain.opened / domain.sent) * 100) : 0
                    const domainCTR = domain.opened > 0 ? Math.round((domain.clicked / domain.opened) * 100) : 0

                    return (
                      <tr key={index} className="hover:bg-gray-50 transition-colors">
                        <td className="py-3 px-3">
                          <div className="flex items-center gap-2">
                            <div
                              className="w-2 h-2 rounded-full"
                              style={{ backgroundColor: COLORS[index % COLORS.length] }}
                            ></div>
                            <span className="font-medium text-gray-800 text-sm">{domain.domain}</span>
                          </div>
                        </td>
                        <td className="py-3 px-3 text-center">
                          <span className="text-sm font-semibold text-gray-700">{domain.sent?.toLocaleString()}</span>
                        </td>
                        <td className="py-3 px-3 text-center">
                          <span className="text-sm text-gray-600">{domain.opened?.toLocaleString()}</span>
                        </td>
                        <td className="py-3 px-3 text-center">
                          <span className="text-sm text-gray-600">{domain.clicked?.toLocaleString()}</span>
                        </td>
                        <td className="py-3 px-3 text-center">
                          <div className="flex items-center justify-center gap-2">
                            <div className="w-16 h-2 bg-gray-100 rounded-full overflow-hidden">
                              <div
                                className="h-full bg-purple-500 rounded-full transition-all"
                                style={{ width: `${domainOpenRate}%` }}
                              ></div>
                            </div>
                            <span className="text-xs font-medium text-gray-600 w-10">{domainOpenRate}%</span>
                          </div>
                        </td>
                        <td className="py-3 px-3 text-center">
                          <div className="flex items-center justify-center gap-2">
                            <div className="w-16 h-2 bg-gray-100 rounded-full overflow-hidden">
                              <div
                                className="h-full bg-orange-500 rounded-full transition-all"
                                style={{ width: `${domainCTR}%` }}
                              ></div>
                            </div>
                            <span className="text-xs font-medium text-gray-600 w-10">{domainCTR}%</span>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default Dashboard
