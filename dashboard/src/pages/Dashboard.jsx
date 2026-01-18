import { useState, useEffect, useRef, useCallback } from 'react'
import {
  Send,
  CheckCircle,
  Eye,
  MousePointer,
  Server,
  Mail,
  TrendingUp,
  Activity,
  Flame,
  AlertTriangle,
  Users,
  Zap,
  RefreshCw,
  Inbox,
  MessageSquare,
  Globe,
  Monitor,
  Smartphone,
  Tablet,
  Chrome,
  Bot,
  MapPin,
  Shield
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
  Bar,
  PieChart,
  Pie,
  Cell
} from 'recharts'
import { useNavigate } from 'react-router-dom'
import api from '../services/api'

// Cache duration in milliseconds
const CACHE_DURATION = 30000 // 30 seconds
const ACTIVITY_CACHE_DURATION = 10000 // 10 seconds

// Colorful Stat Card
function ColorStatCard({ icon: Icon, label, value, subtitle, color, onClick }) {
  const colorClasses = {
    blue: 'bg-gradient-to-br from-blue-500 to-blue-600 dark:from-blue-600 dark:to-blue-700',
    green: 'bg-gradient-to-br from-emerald-500 to-emerald-600 dark:from-emerald-600 dark:to-emerald-700',
    purple: 'bg-gradient-to-br from-purple-500 to-purple-600 dark:from-purple-600 dark:to-purple-700',
    orange: 'bg-gradient-to-br from-orange-500 to-orange-600 dark:from-orange-600 dark:to-orange-700',
    pink: 'bg-gradient-to-br from-pink-500 to-pink-600 dark:from-pink-600 dark:to-pink-700',
    cyan: 'bg-gradient-to-br from-cyan-500 to-cyan-600 dark:from-cyan-600 dark:to-cyan-700',
  }

  return (
    <div
      onClick={onClick}
      className={`${colorClasses[color]} rounded-2xl p-5 text-white shadow-lg ${
        onClick ? 'cursor-pointer hover:shadow-xl hover:scale-[1.02] transition-all duration-200' : ''
      }`}
    >
      <div className="flex items-start justify-between">
        <div className="p-2.5 bg-white/20 rounded-xl backdrop-blur-sm">
          <Icon className="h-5 w-5" />
        </div>
      </div>
      <div className="mt-4">
        <p className="text-3xl font-bold">{value}</p>
        <p className="text-sm text-white/80 mt-1">{label}</p>
        {subtitle && <p className="text-xs text-white/60 mt-0.5">{subtitle}</p>}
      </div>
    </div>
  )
}

// Small Stat Card with Icon
function SmallStatCard({ icon: Icon, label, value, subtitle, iconBg, iconColor, onClick }) {
  return (
    <div
      onClick={onClick}
      className={`bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700 ${
        onClick ? 'cursor-pointer hover:shadow-md hover:border-gray-200 dark:hover:border-gray-600 transition-all' : ''
      }`}
    >
      <div className="flex items-center gap-4">
        <div className={`p-3 rounded-xl ${iconBg}`}>
          <Icon className={`h-5 w-5 ${iconColor}`} />
        </div>
        <div>
          <p className="text-2xl font-bold text-gray-900 dark:text-white">{value}</p>
          <p className="text-sm text-gray-500 dark:text-gray-400">{label}</p>
          {subtitle && <p className="text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>}
        </div>
      </div>
    </div>
  )
}

// Progress Ring (Donut)
function ProgressRing({ value, size = 120, strokeWidth = 12, color = '#3b82f6', label }) {
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
        {label && <span className="text-xs text-gray-500 dark:text-gray-400">{label}</span>}
      </div>
    </div>
  )
}

// Activity Item
function ActivityItem({ type, email, time, subject }) {
  return (
    <div className="flex items-center gap-3 py-3 px-4 bg-gray-50 dark:bg-gray-700/50 rounded-xl mb-2">
      <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
        type === 'open' ? 'bg-emerald-500' :
        type === 'click' ? 'bg-blue-500' :
        type === 'sent' ? 'bg-purple-500' : 'bg-gray-300 dark:bg-gray-600'
      }`} />
      <div className="flex-1 min-w-0">
        <p className="text-sm text-gray-700 dark:text-gray-300 truncate">{subject || email}</p>
        <p className="text-xs text-gray-400 dark:text-gray-500">
          {type === 'open' ? 'Abriu' : type === 'click' ? 'Clicou' : 'Enviado'} - {email}
        </p>
      </div>
      <span className="text-xs text-gray-400 dark:text-gray-500 flex-shrink-0">{time}</span>
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

// Stat Bar Component for lists
function StatBar({ label, value, total, color = 'blue' }) {
  const percentage = total > 0 ? (value / total) * 100 : 0
  const colorClasses = {
    blue: 'bg-blue-500',
    green: 'bg-emerald-500',
    purple: 'bg-purple-500',
    orange: 'bg-orange-500',
    red: 'bg-red-500',
    cyan: 'bg-cyan-500',
  }

  return (
    <div className="flex items-center gap-3">
      <div className="w-24 text-sm text-gray-600 dark:text-gray-400 truncate" title={label}>
        {label}
      </div>
      <div className="flex-1 h-2 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
        <div
          className={`h-full ${colorClasses[color]} rounded-full transition-all duration-500`}
          style={{ width: `${Math.min(percentage, 100)}%` }}
        />
      </div>
      <div className="w-16 text-sm text-gray-900 dark:text-white text-right font-medium">
        {value.toLocaleString()}
      </div>
    </div>
  )
}

// Device Icon component
function DeviceIcon({ type }) {
  switch (type?.toLowerCase()) {
    case 'mobile':
      return <Smartphone className="h-4 w-4" />
    case 'tablet':
      return <Tablet className="h-4 w-4" />
    default:
      return <Monitor className="h-4 w-4" />
  }
}

// Country flag emoji
function CountryFlag({ code }) {
  if (!code || code.length !== 2) return <Globe className="h-4 w-4" />
  const codePoints = code
    .toUpperCase()
    .split('')
    .map(char => 127397 + char.charCodeAt(0))
  return <span className="text-lg">{String.fromCodePoint(...codePoints)}</span>
}

function Dashboard() {
  const navigate = useNavigate()

  // Main stats state
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

  // Tracking stats state (geolocation, browsers, devices)
  const [trackingStats, setTrackingStats] = useState({
    countries: [],
    regions: [],
    cities: [],
    browsers: [],
    devices: [],
    os: [],
    email_clients: [],
    bot_stats: {},
    bot_types: []
  })

  const [warmupStats, setWarmupStats] = useState({
    total_sent: 0,
    total_replies: 0,
    total_interactions: 0,
    inbox_rate: 0,
    active_smtps: 0,
    active_seeds: 0
  })

  const [smtpHealth, setSmtpHealth] = useState([])
  const [loading, setLoading] = useState(true)
  const [period, setPeriod] = useState('today')
  const [activities, setActivities] = useState([])
  const [activeTab, setActiveTab] = useState('campaigns')

  // Cache refs to persist data between re-renders
  const cacheRef = useRef({
    stats: { data: null, timestamp: 0, period: null },
    tracking: { data: null, timestamp: 0, period: null },
    activities: { data: null, timestamp: 0 },
    warmup: { data: null, timestamp: 0 },
    smtp: { data: null, timestamp: 0 }
  })

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

  // Fetch stats with cache
  const fetchStats = useCallback(async (forceRefresh = false) => {
    const cache = cacheRef.current.stats
    const now = Date.now()

    // Use cache if valid and same period
    if (!forceRefresh && cache.data && cache.period === period && (now - cache.timestamp) < CACHE_DURATION) {
      setStats(prev => ({ ...prev, ...cache.data }))
      return
    }

    try {
      const response = await api.get(`/stats?period=${period}`)
      const data = response.data

      // Update cache
      cacheRef.current.stats = { data, timestamp: now, period }
      setStats(data)
    } catch (error) {
      console.error('Failed to fetch stats:', error)
    } finally {
      setLoading(false)
    }
  }, [period])

  // Fetch tracking stats with cache
  const fetchTrackingStats = useCallback(async (forceRefresh = false) => {
    const cache = cacheRef.current.tracking
    const now = Date.now()

    // Use cache if valid and same period
    if (!forceRefresh && cache.data && cache.period === period && (now - cache.timestamp) < CACHE_DURATION) {
      setTrackingStats(cache.data)
      return
    }

    try {
      const response = await api.get(`/stats/tracking?period=${period}`)
      const data = response.data

      // Update cache
      cacheRef.current.tracking = { data, timestamp: now, period }
      setTrackingStats(data)
    } catch (error) {
      console.error('Failed to fetch tracking stats:', error)
    }
  }, [period])

  // Fetch activities with cache
  const fetchActivities = useCallback(async (forceRefresh = false) => {
    const cache = cacheRef.current.activities
    const now = Date.now()

    // Use cache if valid
    if (!forceRefresh && cache.data && (now - cache.timestamp) < ACTIVITY_CACHE_DURATION) {
      setActivities(cache.data)
      return
    }

    try {
      const response = await api.get('/stats/activity?limit=10')
      const data = response.data.activities || []

      // Update cache
      cacheRef.current.activities = { data, timestamp: now }
      setActivities(data)
    } catch (error) {
      console.error('Failed to fetch activities:', error)
    }
  }, [])

  // Fetch warmup stats with cache
  const fetchWarmupStats = useCallback(async (forceRefresh = false) => {
    const cache = cacheRef.current.warmup
    const now = Date.now()

    // Use cache if valid
    if (!forceRefresh && cache.data && (now - cache.timestamp) < CACHE_DURATION) {
      setWarmupStats(cache.data)
      return
    }

    try {
      const response = await api.get('/warmup/stats')
      const data = response.data || {}

      // Update cache
      cacheRef.current.warmup = { data, timestamp: now }
      setWarmupStats(data)
    } catch (error) {
      // Warmup might not be set up yet
    }
  }, [])

  // Fetch SMTP health with cache
  const fetchSmtpHealth = useCallback(async (forceRefresh = false) => {
    const cache = cacheRef.current.smtp
    const now = Date.now()

    // Use cache if valid
    if (!forceRefresh && cache.data && (now - cache.timestamp) < CACHE_DURATION) {
      setSmtpHealth(cache.data)
      return
    }

    try {
      const response = await api.get('/smtp')
      const smtps = response.data?.data || []
      const healthIssues = smtps.filter(s =>
        s.status === 'error' || s.status === 'disabled' || s.fail_count > 10
      )

      // Update cache
      cacheRef.current.smtp = { data: healthIssues, timestamp: now }
      setSmtpHealth(healthIssues)
    } catch (error) {
      // Silent fail
    }
  }, [])

  // Initial fetch and setup intervals
  useEffect(() => {
    // Initial fetch
    fetchStats()
    fetchTrackingStats()
    fetchActivities()
    fetchWarmupStats()
    fetchSmtpHealth()

    // Set up intervals with longer durations to reduce load
    const statsInterval = setInterval(() => fetchStats(true), 15000) // 15 seconds
    const trackingInterval = setInterval(() => fetchTrackingStats(true), 30000) // 30 seconds
    const activityInterval = setInterval(() => fetchActivities(true), 10000) // 10 seconds
    const warmupInterval = setInterval(() => fetchWarmupStats(true), 30000) // 30 seconds

    return () => {
      clearInterval(statsInterval)
      clearInterval(trackingInterval)
      clearInterval(activityInterval)
      clearInterval(warmupInterval)
    }
  }, [fetchStats, fetchTrackingStats, fetchActivities, fetchWarmupStats, fetchSmtpHealth])

  // Refetch when period changes
  useEffect(() => {
    fetchStats(true)
    fetchTrackingStats(true)
  }, [period, fetchStats, fetchTrackingStats])

  const openRate = stats.today_sent > 0
    ? Math.round((stats.today_opened / stats.today_sent) * 100)
    : 0

  const clickRate = stats.today_opened > 0
    ? Math.round((stats.today_clicked / stats.today_opened) * 100)
    : 0

  const deliveryRate = stats.today_sent > 0
    ? Math.round(((stats.today_sent - stats.today_failed) / stats.today_sent) * 100)
    : 100

  // Calculate bot percentage
  const botStats = trackingStats.bot_stats || {}
  const totalOpens = (botStats.human_opens || 0) + (botStats.bot_opens || 0) + (botStats.suspicious_opens || 0)
  const realOpenRate = totalOpens > 0 ? Math.round((botStats.human_opens / totalOpens) * 100) : 100

  // Get max value for stat bars
  const getMaxTotal = (items) => {
    if (!items || items.length === 0) return 1
    return Math.max(...items.map(i => i.total || 0), 1)
  }

  return (
    <div className="space-y-6 pb-8">
      {/* Alert Banner */}
      {smtpHealth.length > 0 && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-2xl p-4 flex items-center gap-4">
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
          <div className="flex items-center gap-2 px-4 py-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-xl">
            <div className="w-2 h-2 bg-emerald-500 rounded-full animate-pulse" />
            <span className="text-sm font-medium text-emerald-700 dark:text-emerald-400">{stats.sending_rate}/s</span>
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
          {/* Colorful Campaign Stats */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <ColorStatCard
              icon={Send}
              label="Emails Enviados"
              value={formatNumber(stats.today_sent)}
              subtitle="Hoje"
              color="blue"
            />
            <ColorStatCard
              icon={Eye}
              label="Aberturas"
              value={formatNumber(stats.today_opened)}
              subtitle={`${openRate}% taxa de abertura`}
              color="green"
            />
            <ColorStatCard
              icon={MousePointer}
              label="Cliques"
              value={formatNumber(stats.today_clicked)}
              subtitle={`${clickRate}% CTR`}
              color="purple"
            />
            <ColorStatCard
              icon={Zap}
              label="Na Fila"
              value={formatNumber(stats.queue_size)}
              subtitle="Aguardando envio"
              color="orange"
            />
          </div>

          {/* Secondary Stats Row */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <SmallStatCard
              icon={Server}
              label="SMTPs Ativos"
              value={`${stats.active_smtps}/${stats.total_smtps}`}
              iconBg="bg-blue-100 dark:bg-blue-900/30"
              iconColor="text-blue-600 dark:text-blue-400"
              onClick={() => navigate('/smtp')}
            />
            <SmallStatCard
              icon={TrendingUp}
              label="Campanhas Ativas"
              value={stats.active_campaigns}
              iconBg="bg-purple-100 dark:bg-purple-900/30"
              iconColor="text-purple-600 dark:text-purple-400"
              onClick={() => navigate('/campaigns')}
            />
            <SmallStatCard
              icon={Mail}
              label="Total de Emails"
              value={formatNumber(stats.total_emails)}
              subtitle={`${stats.total_lists} listas`}
              iconBg="bg-cyan-100 dark:bg-cyan-900/30"
              iconColor="text-cyan-600 dark:text-cyan-400"
              onClick={() => navigate('/lists')}
            />
            <SmallStatCard
              icon={CheckCircle}
              label="Taxa de Entrega"
              value={`${deliveryRate}%`}
              subtitle="sucesso"
              iconBg="bg-emerald-100 dark:bg-emerald-900/30"
              iconColor="text-emerald-600 dark:text-emerald-400"
            />
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
                  <div className="w-3 h-3 rounded-full bg-purple-500" />
                  <span className="text-sm text-gray-600 dark:text-gray-400">Cliques</span>
                </div>
              </div>

              <div className="h-64">
                <ResponsiveContainer width="100%" height="100%">
                  {period === 'today' ? (
                    <AreaChart data={stats.hourly || []}>
                      <defs>
                        <linearGradient id="colorSent" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.2}/>
                          <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                        </linearGradient>
                        <linearGradient id="colorOpened" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="#10b981" stopOpacity={0.2}/>
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
                  <RefreshCw className="h-3 w-3 text-emerald-500 animate-spin" />
                  <span className="text-xs text-emerald-600 dark:text-emerald-400">Auto</span>
                </div>
              </div>

              {activities.length > 0 ? (
                <div className="space-y-0">
                  {activities.slice(0, 6).map((activity, index) => (
                    <ActivityItem
                      key={index}
                      type={activity.type}
                      email={activity.email}
                      subject={activity.subject}
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

          {/* Tracking Stats Section */}
          <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-4 gap-6">
            {/* Countries */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-xl">
                  <Globe className="h-5 w-5 text-blue-600 dark:text-blue-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Paises</h3>
              </div>
              {trackingStats.countries?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.countries.slice(0, 5).map((item, index) => (
                    <div key={index} className="flex items-center gap-3">
                      <CountryFlag code={item.country_code} />
                      <StatBar
                        label={item.country || 'Unknown'}
                        value={item.total}
                        total={getMaxTotal(trackingStats.countries)}
                        color="blue"
                      />
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* Regions/States */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-purple-100 dark:bg-purple-900/30 rounded-xl">
                  <MapPin className="h-5 w-5 text-purple-600 dark:text-purple-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Estados</h3>
              </div>
              {trackingStats.regions?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.regions.slice(0, 5).map((item, index) => (
                    <StatBar
                      key={index}
                      label={item.region || 'Unknown'}
                      value={item.total}
                      total={getMaxTotal(trackingStats.regions)}
                      color="purple"
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* Cities */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-cyan-100 dark:bg-cyan-900/30 rounded-xl">
                  <MapPin className="h-5 w-5 text-cyan-600 dark:text-cyan-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Cidades</h3>
              </div>
              {trackingStats.cities?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.cities.slice(0, 5).map((item, index) => (
                    <StatBar
                      key={index}
                      label={item.city || 'Unknown'}
                      value={item.total}
                      total={getMaxTotal(trackingStats.cities)}
                      color="cyan"
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* Browsers */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-orange-100 dark:bg-orange-900/30 rounded-xl">
                  <Chrome className="h-5 w-5 text-orange-600 dark:text-orange-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Navegadores</h3>
              </div>
              {trackingStats.browsers?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.browsers.slice(0, 5).map((item, index) => (
                    <StatBar
                      key={index}
                      label={item.browser || 'Unknown'}
                      value={item.total}
                      total={getMaxTotal(trackingStats.browsers)}
                      color="orange"
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>
          </div>

          {/* Second Row: Devices, OS, Email Clients, Bot Stats */}
          <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-4 gap-6">
            {/* Devices */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-xl">
                  <Monitor className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Dispositivos</h3>
              </div>
              {trackingStats.devices?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.devices.map((item, index) => (
                    <div key={index} className="flex items-center gap-3">
                      <DeviceIcon type={item.device_type} />
                      <StatBar
                        label={item.device_type === 'desktop' ? 'Desktop' : item.device_type === 'mobile' ? 'Mobile' : item.device_type === 'tablet' ? 'Tablet' : item.device_type || 'Unknown'}
                        value={item.total}
                        total={getMaxTotal(trackingStats.devices)}
                        color="green"
                      />
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* OS */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-pink-100 dark:bg-pink-900/30 rounded-xl">
                  <Monitor className="h-5 w-5 text-pink-600 dark:text-pink-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Sist. Operacional</h3>
              </div>
              {trackingStats.os?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.os.slice(0, 5).map((item, index) => (
                    <StatBar
                      key={index}
                      label={item.os || 'Unknown'}
                      value={item.total}
                      total={getMaxTotal(trackingStats.os)}
                      color="pink"
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* Email Clients */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-indigo-100 dark:bg-indigo-900/30 rounded-xl">
                  <Mail className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Clientes de Email</h3>
              </div>
              {trackingStats.email_clients?.length > 0 ? (
                <div className="space-y-3">
                  {trackingStats.email_clients.slice(0, 5).map((item, index) => (
                    <StatBar
                      key={index}
                      label={item.email_client || 'Unknown'}
                      value={item.total}
                      total={getMaxTotal(trackingStats.email_clients)}
                      color="purple"
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">Sem dados</p>
              )}
            </div>

            {/* Bot Stats */}
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-xl">
                  <Shield className="h-5 w-5 text-red-600 dark:text-red-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Engajamento Real</h3>
              </div>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-emerald-500" />
                    <span className="text-sm text-gray-600 dark:text-gray-400">Humanos</span>
                  </div>
                  <span className="font-semibold text-gray-900 dark:text-white">
                    {formatNumber(botStats.human_opens || 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-red-500" />
                    <span className="text-sm text-gray-600 dark:text-gray-400">Bots</span>
                  </div>
                  <span className="font-semibold text-gray-900 dark:text-white">
                    {formatNumber(botStats.bot_opens || 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-yellow-500" />
                    <span className="text-sm text-gray-600 dark:text-gray-400">Suspeitos</span>
                  </div>
                  <span className="font-semibold text-gray-900 dark:text-white">
                    {formatNumber(botStats.suspicious_opens || 0)}
                  </span>
                </div>
                <div className="pt-3 border-t border-gray-100 dark:border-gray-700">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700 dark:text-gray-300">Taxa Real</span>
                    <span className={`font-bold ${realOpenRate >= 70 ? 'text-emerald-600' : realOpenRate >= 40 ? 'text-yellow-600' : 'text-red-600'}`}>
                      {realOpenRate}%
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Domain Stats */}
          {stats.by_domain && stats.by_domain.length > 0 && (
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-violet-100 dark:bg-violet-900/30 rounded-xl">
                  <Mail className="h-5 w-5 text-violet-600 dark:text-violet-400" />
                </div>
                <h3 className="font-semibold text-gray-900 dark:text-white">Desempenho por Dominio</h3>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {stats.by_domain.slice(0, 9).map((item, index) => (
                  <div key={index} className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-xl">
                    <span className="text-sm font-medium text-gray-700 dark:text-gray-300">{item.domain}</span>
                    <div className="flex items-center gap-3 text-xs">
                      <span className="text-blue-600 dark:text-blue-400">{formatNumber(item.sent)} env</span>
                      <span className="text-emerald-600 dark:text-emerald-400">{formatNumber(item.opened)} abr</span>
                      <span className="text-purple-600 dark:text-purple-400">{formatNumber(item.clicked)} cli</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      ) : (
        <>
          {/* Warmup Stats - Colorful */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <ColorStatCard
              icon={Mail}
              label="Emails Enviados"
              value={formatNumber(warmupStats.total_sent || 0)}
              subtitle="Total warmup"
              color="blue"
              onClick={() => navigate('/warmup')}
            />
            <ColorStatCard
              icon={MessageSquare}
              label="Interacoes"
              value={formatNumber(warmupStats.total_interactions || 0)}
              subtitle="Respostas e aberturas"
              color="green"
            />
            <ColorStatCard
              icon={MessageSquare}
              label="Respostas"
              value={formatNumber(warmupStats.total_replies || 0)}
              subtitle="Respostas automaticas"
              color="purple"
            />
            <div className="bg-white dark:bg-gray-800 rounded-2xl p-5 border border-gray-100 dark:border-gray-700 flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">Inbox Rate</p>
                <p className="text-3xl font-bold text-gray-900 dark:text-white">
                  {warmupStats.inbox_rate?.toFixed(1) || 0}%
                </p>
                <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">Taxa de entrada</p>
              </div>
              <ProgressRing
                value={Math.round(warmupStats.inbox_rate || 0)}
                size={80}
                strokeWidth={8}
                color={warmupStats.inbox_rate >= 80 ? '#10b981' : warmupStats.inbox_rate >= 50 ? '#f59e0b' : '#ef4444'}
              />
            </div>
          </div>

          {/* Warmup Secondary Stats */}
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
            <SmallStatCard
              icon={Server}
              label="SMTPs em Warmup"
              value={warmupStats.active_smtps || 0}
              iconBg="bg-orange-100 dark:bg-orange-900/30"
              iconColor="text-orange-600 dark:text-orange-400"
              onClick={() => navigate('/warmup')}
            />
            <SmallStatCard
              icon={Users}
              label="Seeds Ativos"
              value={warmupStats.active_seeds || 0}
              iconBg="bg-cyan-100 dark:bg-cyan-900/30"
              iconColor="text-cyan-600 dark:text-cyan-400"
              onClick={() => navigate('/warmup')}
            />
            <SmallStatCard
              icon={Flame}
              label="Templates"
              value="75"
              iconBg="bg-pink-100 dark:bg-pink-900/30"
              iconColor="text-pink-600 dark:text-pink-400"
              onClick={() => navigate('/warmup')}
            />
          </div>

          {/* Warmup Quick Actions */}
          <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 border border-gray-100 dark:border-gray-700">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Acoes Rapidas</h2>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <button
                onClick={() => navigate('/warmup')}
                className="flex items-center gap-4 p-4 bg-gradient-to-r from-orange-50 to-amber-50 dark:from-orange-900/20 dark:to-amber-900/20 border border-orange-100 dark:border-orange-800 rounded-xl hover:shadow-md transition-all text-left"
              >
                <div className="p-3 bg-orange-100 dark:bg-orange-900/50 rounded-xl">
                  <Flame className="h-5 w-5 text-orange-600 dark:text-orange-400" />
                </div>
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">Gerenciar Warmup</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Configurar SMTPs e Seeds</p>
                </div>
              </button>
              <button
                onClick={() => navigate('/smtp')}
                className="flex items-center gap-4 p-4 bg-gradient-to-r from-blue-50 to-indigo-50 dark:from-blue-900/20 dark:to-indigo-900/20 border border-blue-100 dark:border-blue-800 rounded-xl hover:shadow-md transition-all text-left"
              >
                <div className="p-3 bg-blue-100 dark:bg-blue-900/50 rounded-xl">
                  <Server className="h-5 w-5 text-blue-600 dark:text-blue-400" />
                </div>
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">Servidores SMTP</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Adicionar novos servidores</p>
                </div>
              </button>
              <button
                onClick={() => navigate('/warmup')}
                className="flex items-center gap-4 p-4 bg-gradient-to-r from-emerald-50 to-teal-50 dark:from-emerald-900/20 dark:to-teal-900/20 border border-emerald-100 dark:border-emerald-800 rounded-xl hover:shadow-md transition-all text-left"
              >
                <div className="p-3 bg-emerald-100 dark:bg-emerald-900/50 rounded-xl">
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
          <div className="bg-gradient-to-br from-orange-500 to-amber-500 dark:from-orange-600 dark:to-amber-600 rounded-2xl p-6 text-white">
            <div className="flex items-start gap-4">
              <div className="p-3 bg-white/20 rounded-xl backdrop-blur-sm">
                <Flame className="h-6 w-6" />
              </div>
              <div className="flex-1">
                <h3 className="font-semibold text-lg">Sobre o Warmup</h3>
                <p className="text-sm text-white/80 mt-1">
                  O warmup ajuda a construir a reputacao dos seus servidores SMTP enviando emails
                  gradualmente para contas seed. Isso aumenta a taxa de entrega na caixa de entrada.
                </p>
                <div className="flex gap-8 mt-4">
                  <div>
                    <p className="text-3xl font-bold">{warmupStats.active_smtps || 0}</p>
                    <p className="text-xs text-white/70">SMTPs em warmup</p>
                  </div>
                  <div>
                    <p className="text-3xl font-bold">{formatNumber(warmupStats.total_sent || 0)}</p>
                    <p className="text-xs text-white/70">Emails enviados</p>
                  </div>
                  <div>
                    <p className="text-3xl font-bold">{warmupStats.inbox_rate?.toFixed(0) || 0}%</p>
                    <p className="text-xs text-white/70">Taxa inbox</p>
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
