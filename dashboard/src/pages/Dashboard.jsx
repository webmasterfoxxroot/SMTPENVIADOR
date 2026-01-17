import { useState, useEffect } from 'react'
import {
  Send,
  CheckCircle,
  Eye,
  MousePointer,
  Server,
  Mail,
  TrendingUp,
  ArrowUpRight,
  ArrowDownRight,
  Activity,
  Flame,
  AlertTriangle,
  Users,
  Zap,
  RefreshCw
} from 'lucide-react'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  BarChart,
  Bar
} from 'recharts'
import { useNavigate } from 'react-router-dom'
import api from '../services/api'

// Modern Stat Card
function StatCard({ icon: Icon, label, value, change, changeType, subtitle, onClick, highlight }) {
  return (
    <div
      onClick={onClick}
      className={`bg-white dark:bg-gray-800 rounded-2xl p-6 border transition-all duration-200 ${
        onClick ? 'cursor-pointer hover:shadow-lg hover:border-gray-200 dark:hover:border-gray-600' : ''
      } ${highlight ? 'border-blue-200 dark:border-blue-800 bg-blue-50/30 dark:bg-blue-900/20' : 'border-gray-100 dark:border-gray-700'}`}
    >
      <div className="flex items-start justify-between">
        <div className={`p-3 rounded-xl ${highlight ? 'bg-blue-100 dark:bg-blue-900/50' : 'bg-gray-100 dark:bg-gray-700'}`}>
          <Icon className={`h-5 w-5 ${highlight ? 'text-blue-600 dark:text-blue-400' : 'text-gray-600 dark:text-gray-400'}`} />
        </div>
        {change !== undefined && (
          <div className={`flex items-center gap-1 text-sm font-medium ${
            changeType === 'positive' ? 'text-emerald-600 dark:text-emerald-400' :
            changeType === 'negative' ? 'text-red-500 dark:text-red-400' : 'text-gray-400 dark:text-gray-500'
          }`}>
            {changeType === 'positive' ? <ArrowUpRight className="h-4 w-4" /> :
             changeType === 'negative' ? <ArrowDownRight className="h-4 w-4" /> : null}
            {change}
          </div>
        )}
      </div>
      <div className="mt-4">
        <p className="text-3xl font-bold text-gray-900 dark:text-white">{value}</p>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{label}</p>
        {subtitle && <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">{subtitle}</p>}
      </div>
    </div>
  )
}

// Progress Ring
function ProgressRing({ value, size = 120, strokeWidth = 10, color = '#3b82f6' }) {
  const radius = (size - strokeWidth) / 2
  const circumference = radius * 2 * Math.PI
  const offset = circumference - (value / 100) * circumference

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg className="transform -rotate-90" width={size} height={size}>
        <circle
          className="text-gray-100 dark:text-gray-700"
          strokeWidth={strokeWidth}
          stroke="currentColor"
          fill="transparent"
          r={radius}
          cx={size / 2}
          cy={size / 2}
        />
        <circle
          className="transition-all duration-500 ease-out"
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          stroke={color}
          fill="transparent"
          r={radius}
          cx={size / 2}
          cy={size / 2}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold text-gray-900 dark:text-white">{value}%</span>
      </div>
    </div>
  )
}

// Activity Item
function ActivityItem({ type, email, time }) {
  return (
    <div className="flex items-center gap-3 py-3 border-b border-gray-50 dark:border-gray-700/50 last:border-0">
      <div className={`w-2 h-2 rounded-full ${
        type === 'open' ? 'bg-emerald-500' :
        type === 'click' ? 'bg-blue-500' : 'bg-gray-300 dark:bg-gray-600'
      }`} />
      <div className="flex-1 min-w-0">
        <p className="text-sm text-gray-700 dark:text-gray-300 truncate">{email}</p>
        <p className="text-xs text-gray-400 dark:text-gray-500">{type === 'open' ? 'Abriu' : 'Clicou'} - {time}</p>
      </div>
    </div>
  )
}

// Custom Tooltip
function CustomTooltip({ active, payload, label }) {
  if (active && payload && payload.length) {
    return (
      <div className="bg-white dark:bg-gray-800 px-4 py-3 rounded-xl shadow-lg border border-gray-100 dark:border-gray-700">
        <p className="text-sm font-medium text-gray-900 dark:text-white mb-1">{label}</p>
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
  const navigate = useNavigate()
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
  const [warmupStats, setWarmupStats] = useState({
    total_sent: 0,
    total_replies: 0,
    total_interactions: 0,
    inbox_rate: 0,
    active_smtps: 0
  })
  const [smtpHealth, setSmtpHealth] = useState([])
  const [loading, setLoading] = useState(true)
  const [period, setPeriod] = useState('today')
  const [activities, setActivities] = useState([])
  const [activeTab, setActiveTab] = useState('campaigns')

  useEffect(() => {
    fetchStats()
    fetchActivities()
    fetchWarmupStats()
    fetchSmtpHealth()
    const statsInterval = setInterval(fetchStats, 5000)
    const activityInterval = setInterval(fetchActivities, 3000)
    const warmupInterval = setInterval(fetchWarmupStats, 10000)
    return () => {
      clearInterval(statsInterval)
      clearInterval(activityInterval)
      clearInterval(warmupInterval)
    }
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

  const fetchActivities = async () => {
    try {
      const response = await api.get('/stats/activity?limit=10')
      setActivities(response.data.activities || [])
    } catch (error) {
      console.error('Failed to fetch activities:', error)
    }
  }

  const fetchWarmupStats = async () => {
    try {
      const response = await api.get('/warmup/stats')
      setWarmupStats(response.data || {})
    } catch (error) {
      // Warmup might not be set up yet
    }
  }

  const fetchSmtpHealth = async () => {
    try {
      const response = await api.get('/smtp')
      const smtps = response.data?.data || []
      const healthIssues = smtps.filter(s =>
        s.status === 'error' || s.status === 'disabled' || s.fail_count > 10
      )
      setSmtpHealth(healthIssues)
    } catch (error) {
      // Silent fail
    }
  }

  const formatTimeAgo = (timestamp) => {
    const now = new Date()
    const time = new Date(timestamp)
    const diff = Math.floor((now - time) / 1000)

    if (diff < 60) return `${diff}s`
    if (diff < 3600) return `${Math.floor(diff / 60)}m`
    if (diff < 86400) return `${Math.floor(diff / 3600)}h`
    return time.toLocaleDateString('pt-BR')
  }

  const formatNumber = (num) => {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
    return num?.toLocaleString() || '0'
  }

  const openRate = stats.today_sent > 0
    ? Math.round((stats.today_opened / stats.today_sent) * 100)
    : 0

  const clickRate = stats.today_opened > 0
    ? Math.round((stats.today_clicked / stats.today_opened) * 100)
    : 0

  const deliveryRate = stats.today_sent > 0
    ? Math.round(((stats.today_sent - stats.today_failed) / stats.today_sent) * 100)
    : 100

  return (
    <div className="space-y-6 pb-8">
      {/* Alert Banner */}
      {smtpHealth.length > 0 && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-100 dark:border-red-800 rounded-2xl p-4 flex items-center gap-4">
          <div className="p-2 bg-red-100 dark:bg-red-900/50 rounded-xl">
            <AlertTriangle className="h-5 w-5 text-red-600 dark:text-red-400" />
          </div>
          <div className="flex-1">
            <p className="font-medium text-red-800 dark:text-red-300">{smtpHealth.length} SMTP(s) com problemas</p>
            <p className="text-sm text-red-600 dark:text-red-400">{smtpHealth.map(s => s.host).join(', ')}</p>
          </div>
          <button
            onClick={() => navigate('/smtp')}
            className="px-4 py-2 bg-red-600 text-white rounded-xl text-sm font-medium hover:bg-red-700 transition-colors"
          >
            Verificar
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Dashboard</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">Visao geral do sistema</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-800 rounded-xl">
            <div className="w-2 h-2 bg-emerald-500 rounded-full animate-pulse" />
            <span className="text-sm text-gray-600 dark:text-gray-400">{stats.sending_rate}/s</span>
          </div>
          <button
            onClick={() => navigate('/campaigns')}
            className="px-4 py-2 bg-blue-600 text-white rounded-xl text-sm font-medium hover:bg-blue-700 transition-colors"
          >
            Nova Campanha
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 p-1 bg-gray-100 dark:bg-gray-800 rounded-xl w-fit">
        <button
          onClick={() => setActiveTab('campaigns')}
          className={`px-6 py-2.5 rounded-lg text-sm font-medium transition-all ${
            activeTab === 'campaigns'
              ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm'
              : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
          }`}
        >
          Campanhas
        </button>
        <button
          onClick={() => setActiveTab('warmup')}
          className={`px-6 py-2.5 rounded-lg text-sm font-medium transition-all ${
            activeTab === 'warmup'
              ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm'
              : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
          }`}
        >
          Warmup
        </button>
      </div>

      {activeTab === 'campaigns' ? (
        <>
          {/* Campaign Stats */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <StatCard
              icon={Send}
              label="Emails Enviados"
              value={formatNumber(stats.today_sent)}
              subtitle="Hoje"
              highlight
            />
            <StatCard
              icon={Eye}
              label="Aberturas"
              value={formatNumber(stats.today_opened)}
              change={`${openRate}%`}
              changeType={openRate > 20 ? 'positive' : openRate > 0 ? 'neutral' : 'neutral'}
              subtitle="Taxa de abertura"
            />
            <StatCard
              icon={MousePointer}
              label="Cliques"
              value={formatNumber(stats.today_clicked)}
              change={`${clickRate}%`}
              changeType={clickRate > 5 ? 'positive' : clickRate > 0 ? 'neutral' : 'neutral'}
              subtitle="CTR"
            />
            <StatCard
              icon={Zap}
              label="Na Fila"
              value={formatNumber(stats.queue_size)}
              subtitle="Aguardando envio"
            />
          </div>

          {/* Secondary Stats Row */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3">
                <Server className="h-5 w-5 text-gray-400" />
                <div>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">{stats.active_smtps}/{stats.total_smtps}</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">SMTPs Ativos</p>
                </div>
              </div>
            </div>
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3">
                <TrendingUp className="h-5 w-5 text-gray-400" />
                <div>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">{stats.active_campaigns}</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Campanhas Ativas</p>
                </div>
              </div>
            </div>
            <div
              className="bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700 cursor-pointer hover:border-gray-200 dark:hover:border-gray-600 transition-colors"
              onClick={() => navigate('/lists')}
            >
              <div className="flex items-center gap-3">
                <Mail className="h-5 w-5 text-gray-400" />
                <div>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">{formatNumber(stats.total_emails)}</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">{stats.total_lists} listas</p>
                </div>
              </div>
            </div>
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3">
                <CheckCircle className="h-5 w-5 text-gray-400" />
                <div>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">{deliveryRate}%</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Taxa de Entrega</p>
                </div>
              </div>
            </div>
          </div>

          {/* Chart and Activity */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Chart */}
            <div className="lg:col-span-2 bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Atividade</h2>
                  <p className="text-sm text-gray-500 dark:text-gray-400">
                    {period === 'today' ? 'Hoje por hora' : period === 'week' ? 'Ultimos 7 dias' : 'Ultimos 30 dias'}
                  </p>
                </div>
                <div className="flex gap-1 p-1 bg-gray-100 dark:bg-gray-700 rounded-lg">
                  {['today', 'week', 'month'].map((p) => (
                    <button
                      key={p}
                      onClick={() => setPeriod(p)}
                      className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                        period === p
                          ? 'bg-white dark:bg-gray-600 text-gray-900 dark:text-white shadow-sm'
                          : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
                      }`}
                    >
                      {p === 'today' ? 'Hoje' : p === 'week' ? '7D' : '30D'}
                    </button>
                  ))}
                </div>
              </div>

              <div className="flex gap-6 mb-4">
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-blue-500" />
                  <span className="text-sm text-gray-600 dark:text-gray-400">Enviados</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-emerald-500" />
                  <span className="text-sm text-gray-600 dark:text-gray-400">Abertos</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-amber-500" />
                  <span className="text-sm text-gray-600 dark:text-gray-400">Cliques</span>
                </div>
              </div>

              <div className="h-64">
                <ResponsiveContainer width="100%" height="100%">
                  {period === 'today' ? (
                    <AreaChart data={stats.hourly || []}>
                      <defs>
                        <linearGradient id="colorSent" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.1}/>
                          <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                        </linearGradient>
                        <linearGradient id="colorOpened" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#10b981" stopOpacity={0.1}/>
                          <stop offset="95%" stopColor="#10b981" stopOpacity={0}/>
                        </linearGradient>
                      </defs>
                      <CartesianGrid strokeDasharray="3 3" className="stroke-gray-200 dark:stroke-gray-700" />
                      <XAxis
                        dataKey="hour"
                        tickFormatter={(val) => `${val}h`}
                        className="text-gray-500 dark:text-gray-400"
                        stroke="currentColor"
                        fontSize={12}
                        axisLine={false}
                        tickLine={false}
                      />
                      <YAxis
                        className="text-gray-500 dark:text-gray-400"
                        stroke="currentColor"
                        fontSize={12}
                        axisLine={false}
                        tickLine={false}
                      />
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
                        stroke="#10b981"
                        strokeWidth={2}
                        fill="url(#colorOpened)"
                        name="Abertos"
                      />
                    </AreaChart>
                  ) : (
                    <BarChart data={stats.hourly || []}>
                      <CartesianGrid strokeDasharray="3 3" className="stroke-gray-200 dark:stroke-gray-700" />
                      <XAxis
                        dataKey="label"
                        className="text-gray-500 dark:text-gray-400"
                        stroke="currentColor"
                        fontSize={11}
                        axisLine={false}
                        tickLine={false}
                      />
                      <YAxis
                        className="text-gray-500 dark:text-gray-400"
                        stroke="currentColor"
                        fontSize={12}
                        axisLine={false}
                        tickLine={false}
                      />
                      <Tooltip content={<CustomTooltip />} />
                      <Bar dataKey="sent" fill="#3b82f6" name="Enviados" radius={[4, 4, 0, 0]} />
                      <Bar dataKey="opened" fill="#10b981" name="Abertos" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  )}
                </ResponsiveContainer>
              </div>
            </div>

            {/* Live Activity */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Atividade ao Vivo</h2>
                <div className="flex items-center gap-2">
                  <RefreshCw className="h-3 w-3 text-gray-400 animate-spin" />
                  <span className="text-xs text-gray-400">Auto</span>
                </div>
              </div>

              {activities.length > 0 ? (
                <div className="space-y-1">
                  {activities.slice(0, 8).map((activity, index) => (
                    <ActivityItem
                      key={index}
                      type={activity.type}
                      email={activity.email}
                      time={formatTimeAgo(activity.timestamp)}
                    />
                  ))}
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-gray-400 dark:text-gray-500">
                  <Activity className="h-10 w-10 mb-3 opacity-50" />
                  <p className="text-sm">Nenhuma atividade</p>
                </div>
              )}
            </div>
          </div>
        </>
      ) : (
        <>
          {/* Warmup Stats */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <StatCard
              icon={Flame}
              label="Emails Enviados"
              value={formatNumber(warmupStats.total_sent || 0)}
              subtitle="Total warmup"
              highlight
              onClick={() => navigate('/warmup')}
            />
            <StatCard
              icon={Mail}
              label="Respostas"
              value={formatNumber(warmupStats.total_replies || 0)}
              subtitle="Respostas automaticas"
            />
            <StatCard
              icon={Server}
              label="SMTPs Ativos"
              value={warmupStats.active_smtps || 0}
              subtitle="Em warmup"
            />
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">Inbox Rate</p>
                  <p className="text-3xl font-bold text-gray-900 dark:text-white">
                    {warmupStats.inbox_rate?.toFixed(1) || 0}%
                  </p>
                </div>
                <ProgressRing
                  value={Math.round(warmupStats.inbox_rate || 0)}
                  size={80}
                  strokeWidth={8}
                  color={warmupStats.inbox_rate >= 80 ? '#10b981' : warmupStats.inbox_rate >= 50 ? '#f59e0b' : '#ef4444'}
                />
              </div>
            </div>
          </div>

          {/* Warmup Quick Actions */}
          <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Acoes Rapidas</h2>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <button
                onClick={() => navigate('/warmup')}
                className="flex items-center gap-4 p-4 bg-gray-50 dark:bg-gray-700/50 rounded-xl hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-left"
              >
                <div className="p-3 bg-orange-100 dark:bg-orange-900/30 rounded-xl">
                  <Flame className="h-5 w-5 text-orange-600 dark:text-orange-400" />
                </div>
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">Gerenciar Warmup</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Configurar SMTPs e Seeds</p>
                </div>
              </button>
              <button
                onClick={() => navigate('/smtp')}
                className="flex items-center gap-4 p-4 bg-gray-50 dark:bg-gray-700/50 rounded-xl hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-left"
              >
                <div className="p-3 bg-blue-100 dark:bg-blue-900/30 rounded-xl">
                  <Server className="h-5 w-5 text-blue-600 dark:text-blue-400" />
                </div>
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">Servidores SMTP</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Adicionar novos servidores</p>
                </div>
              </button>
              <button
                onClick={() => navigate('/warmup')}
                className="flex items-center gap-4 p-4 bg-gray-50 dark:bg-gray-700/50 rounded-xl hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-left"
              >
                <div className="p-3 bg-emerald-100 dark:bg-emerald-900/30 rounded-xl">
                  <Users className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
                </div>
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">Seeds</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Gerenciar contas seed</p>
                </div>
              </button>
            </div>
          </div>

          {/* Warmup Info Card */}
          <div className="bg-gradient-to-br from-orange-50 to-amber-50 dark:from-orange-900/20 dark:to-amber-900/20 rounded-2xl p-6 border border-orange-100 dark:border-orange-800">
            <div className="flex items-start gap-4">
              <div className="p-3 bg-orange-100 dark:bg-orange-900/50 rounded-xl">
                <Flame className="h-6 w-6 text-orange-600 dark:text-orange-400" />
              </div>
              <div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Sobre o Warmup</h3>
                <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">
                  O warmup ajuda a construir a reputacao dos seus servidores SMTP enviando emails
                  gradualmente para contas seed. Isso aumenta a taxa de entrega na caixa de entrada.
                </p>
                <div className="flex gap-6 mt-4">
                  <div>
                    <p className="text-2xl font-bold text-orange-600 dark:text-orange-400">{warmupStats.active_smtps || 0}</p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">SMTPs em warmup</p>
                  </div>
                  <div>
                    <p className="text-2xl font-bold text-orange-600 dark:text-orange-400">{formatNumber(warmupStats.total_sent || 0)}</p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">Emails enviados</p>
                  </div>
                  <div>
                    <p className="text-2xl font-bold text-orange-600 dark:text-orange-400">{warmupStats.inbox_rate?.toFixed(0) || 0}%</p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">Taxa inbox</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

export default Dashboard
