import { useState, useEffect, useRef } from 'react'
import {
  Flame,
  Plus,
  Trash2,
  Play,
  Pause,
  Settings,
  Mail,
  Server,
  CheckCircle,
  XCircle,
  AlertCircle,
  Loader2,
  TestTube,
  TrendingUp,
  Inbox,
  MessageSquare,
  BarChart3,
  Calendar,
  Clock,
  X,
  ChevronDown,
  RefreshCw,
  Edit,
  Save,
  ChevronLeft,
  ChevronRight,
  Send,
  Link2,
  FileText,
  Reply,
  ArrowUpRight,
  ClipboardPaste,
  Upload
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

// Componente do grafico interativo de barras com circulos arrastaveis
function WarmupChart({ schedule, onChange, maxEmails, startDate }) {
  const chartRef = useRef(null)
  const sliderRef = useRef(null)
  const [dragging, setDragging] = useState(null)
  const [startIndex, setStartIndex] = useState(0)
  const [isDraggingSlider, setIsDraggingSlider] = useState(false)
  const visibleBars = 15
  const totalBars = schedule.length

  const handleMouseDown = (index, e) => {
    e.preventDefault()
    setDragging(startIndex + index)
  }

  const handleMouseMove = (e) => {
    if (dragging === null || !chartRef.current) return

    const rect = chartRef.current.getBoundingClientRect()
    const y = e.clientY - rect.top
    const chartHeight = rect.height - 50
    const value = Math.round(maxEmails - (y / chartHeight) * maxEmails)
    const clampedValue = Math.max(1, Math.min(maxEmails, value))

    const newSchedule = [...schedule]
    newSchedule[dragging] = clampedValue
    onChange(newSchedule)
  }

  const handleMouseUp = () => {
    setDragging(null)
    setIsDraggingSlider(false)
  }

  // Slider drag handling
  const handleSliderMouseDown = (e) => {
    e.preventDefault()
    setIsDraggingSlider(true)
    updateSliderPosition(e)
  }

  const updateSliderPosition = (e) => {
    if (!sliderRef.current) return
    const rect = sliderRef.current.getBoundingClientRect()
    const x = e.clientX - rect.left
    const percentage = Math.max(0, Math.min(1, x / rect.width))
    const newStartIndex = Math.round(percentage * (totalBars - visibleBars))
    setStartIndex(Math.max(0, Math.min(newStartIndex, totalBars - visibleBars)))
  }

  const handleSliderMouseMove = (e) => {
    if (isDraggingSlider) {
      updateSliderPosition(e)
    }
  }

  useEffect(() => {
    if (dragging !== null || isDraggingSlider) {
      const handleMove = (e) => {
        handleMouseMove(e)
        handleSliderMouseMove(e)
      }
      const handleUp = () => handleMouseUp()

      window.addEventListener('mousemove', handleMove)
      window.addEventListener('mouseup', handleUp)
      return () => {
        window.removeEventListener('mousemove', handleMove)
        window.removeEventListener('mouseup', handleUp)
      }
    }
  }, [dragging, isDraggingSlider, schedule])

  const getDateLabel = (index) => {
    const date = new Date(startDate || new Date())
    date.setDate(date.getDate() + index)
    return date.toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' }).replace('.', '')
  }

  const maxValue = Math.max(...schedule, maxEmails)
  const yAxisSteps = [0, Math.round(maxValue * 0.25), Math.round(maxValue * 0.5), Math.round(maxValue * 0.75), maxValue]

  const canGoLeft = startIndex > 0
  const canGoRight = startIndex < totalBars - visibleBars
  const sliderWidth = (visibleBars / totalBars) * 100
  const sliderPosition = (startIndex / (totalBars - visibleBars || 1)) * (100 - sliderWidth)

  const goLeft = () => setStartIndex(Math.max(0, startIndex - visibleBars))
  const goRight = () => setStartIndex(Math.min(totalBars - visibleBars, startIndex + visibleBars))

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-6">
      {/* Navigation header */}
      <div className="flex items-center justify-between mb-4">
        <div className="text-sm text-gray-500">
          Dias {startIndex + 1} - {Math.min(startIndex + visibleBars, totalBars)} de {totalBars}
        </div>
        <div className="flex gap-2">
          <button
            onClick={goLeft}
            disabled={!canGoLeft}
            className={`p-2 rounded-lg transition-colors ${canGoLeft ? 'hover:bg-gray-100 text-gray-600' : 'text-gray-300 cursor-not-allowed'}`}
          >
            <ChevronLeft className="w-5 h-5" />
          </button>
          <button
            onClick={goRight}
            disabled={!canGoRight}
            className={`p-2 rounded-lg transition-colors ${canGoRight ? 'hover:bg-gray-100 text-gray-600' : 'text-gray-300 cursor-not-allowed'}`}
          >
            <ChevronRight className="w-5 h-5" />
          </button>
        </div>
      </div>

      {/* Slider track - draggable */}
      <div
        ref={sliderRef}
        className="mb-4 cursor-pointer"
        onMouseDown={handleSliderMouseDown}
      >
        <div className="h-3 bg-gray-200 rounded-full relative">
          <div
            className={`h-3 bg-blue-500 rounded-full absolute transition-all ${isDraggingSlider ? 'bg-blue-600' : 'hover:bg-blue-600'}`}
            style={{
              width: `${sliderWidth}%`,
              left: `${sliderPosition}%`
            }}
          ></div>
        </div>
      </div>

      <div
        ref={chartRef}
        className="relative"
        style={{ height: '280px', cursor: dragging !== null ? 'ns-resize' : 'default' }}
      >
        {/* Y-axis */}
        <div className="absolute left-0 top-0 bottom-8 w-10 flex flex-col justify-between text-xs text-gray-400">
          {[...yAxisSteps].reverse().map((val, i) => (
            <span key={i} className="text-right pr-2">{val}</span>
          ))}
        </div>

        {/* Grid lines */}
        <div className="absolute left-10 right-0 top-0 bottom-8">
          {[0, 25, 50, 75, 100].map((percent) => (
            <div
              key={percent}
              className="absolute left-0 right-0 border-t border-gray-100"
              style={{ top: `${percent}%` }}
            ></div>
          ))}
        </div>

        {/* Bars container */}
        <div className="absolute left-10 right-0 top-0 bottom-0 flex items-end pb-8">
          {schedule.slice(startIndex, startIndex + visibleBars).map((value, index) => {
            const actualIndex = startIndex + index
            const barHeight = (value / maxValue) * 100
            return (
              <div
                key={actualIndex}
                className="flex-1 flex flex-col items-center px-0.5 h-full justify-end relative"
                onMouseDown={(e) => handleMouseDown(index, e)}
              >
                {/* Value label above bar */}
                <div className="absolute text-xs font-medium text-gray-600 select-none" style={{ bottom: `calc(${barHeight}% + 24px)` }}>
                  {value}
                </div>

                {/* Draggable circle */}
                <div
                  className={`absolute w-3 h-3 rounded-full border-2 border-white shadow-md cursor-ns-resize z-10 transition-transform ${
                    dragging === actualIndex ? 'bg-blue-700 scale-125' : 'bg-blue-500 hover:scale-110'
                  }`}
                  style={{ bottom: `calc(${barHeight}% + 4px)` }}
                ></div>

                {/* Bar */}
                <div
                  className={`w-full rounded-t-md transition-colors ${
                    dragging === actualIndex ? 'bg-blue-600' : 'bg-blue-500'
                  }`}
                  style={{
                    height: `${Math.max(barHeight, 1)}%`,
                    minHeight: '4px'
                  }}
                ></div>

                {/* Date label */}
                <div className="absolute -bottom-6 text-[10px] text-gray-400 whitespace-nowrap select-none">
                  {getDateLabel(actualIndex)}
                </div>
              </div>
            )
          })}
        </div>
      </div>

      <p className="text-xs text-gray-400 mt-6 text-center">
        Arraste os circulos azuis para ajustar | Arraste a barra azul acima para navegar entre os dias
      </p>
    </div>
  )
}

// Modal para editar configurações do SMTP em warmup
function EditWarmupModal({ warmupSMTP, onClose, onSave }) {
  const [form, setForm] = useState({
    recipe_type: warmupSMTP.recipe_type || 'progressive',
    min_emails_per_day: warmupSMTP.min_emails_per_day || 5,
    max_emails_per_day: warmupSMTP.max_emails_per_day || 40,
    send_rate: warmupSMTP.send_rate || 30,
    reply_rate: warmupSMTP.reply_rate || 30,
    start_hour: warmupSMTP.start_hour || 8,
    end_hour: warmupSMTP.end_hour || 18
  })
  const [schedule, setSchedule] = useState([])
  const [loading, setLoading] = useState(false)
  const [loadingStats, setLoadingStats] = useState(true)
  const [dailyStats, setDailyStats] = useState([])

  // Generate or load schedule
  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    setLoadingStats(true)
    try {
      // Get stats and schedule
      const res = await api.get(`/warmup/smtps/${warmupSMTP.id}/stats`)
      setDailyStats(res.data.daily_stats || [])

      // Parse custom schedule or generate progressive
      if (warmupSMTP.custom_schedule) {
        try {
          const parsed = JSON.parse(warmupSMTP.custom_schedule)
          setSchedule(parsed)
        } catch {
          generateSchedule(form.recipe_type, form.min_emails_per_day, form.max_emails_per_day)
        }
      } else {
        generateSchedule(form.recipe_type, form.min_emails_per_day, form.max_emails_per_day)
      }
    } catch (error) {
      console.error('Error loading stats:', error)
      generateSchedule(form.recipe_type, form.min_emails_per_day, form.max_emails_per_day)
    } finally {
      setLoadingStats(false)
    }
  }

  const generateSchedule = (type, minEmails, maxEmails) => {
    const days = 45
    const newSchedule = []
    const increment = (maxEmails - minEmails) / (days - 1)

    for (let i = 0; i < days; i++) {
      let value
      if (type === 'flat') {
        value = minEmails
      } else if (type === 'randomized') {
        value = minEmails + Math.floor(Math.random() * (maxEmails - minEmails + 1))
      } else if (type === 'progressive') {
        value = minEmails + Math.round(i * increment)
      } else {
        value = minEmails
      }
      newSchedule.push(Math.min(value, maxEmails))
    }
    setSchedule(newSchedule)
  }

  const handleRecipeChange = (type) => {
    setForm({ ...form, recipe_type: type })
    generateSchedule(type, form.min_emails_per_day, form.max_emails_per_day)
  }

  const handleMinChange = (val) => {
    setForm({ ...form, min_emails_per_day: val })
    generateSchedule(form.recipe_type, val, form.max_emails_per_day)
  }

  const handleMaxChange = (val) => {
    setForm({ ...form, max_emails_per_day: val })
    generateSchedule(form.recipe_type, form.min_emails_per_day, val)
  }

  const handleSubmit = async () => {
    setLoading(true)
    try {
      // Update settings
      await api.put(`/warmup/smtps/${warmupSMTP.id}`, {
        ...form,
        status: warmupSMTP.status
      })

      // Update schedule
      await api.put(`/warmup/smtps/${warmupSMTP.id}/schedule`, {
        schedule: schedule
      })

      toast.success('Configurações salvas!')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl w-full max-w-4xl max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-gray-100 sticky top-0 bg-white">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-orange-100 rounded-xl">
              <Settings className="w-6 h-6 text-orange-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900">Configurar Warmup</h2>
              <p className="text-sm text-gray-500">{warmupSMTP.smtp_name}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-100 rounded-lg">
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        <div className="p-6 space-y-6">
          {/* Recipe Type */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-3">Tipo de Aquecimento</label>
            <div className="grid grid-cols-4 gap-3">
              {[
                { value: 'progressive', label: 'Progressivo', desc: 'Recomendado', icon: TrendingUp },
                { value: 'flat', label: 'Fixo', desc: 'Volume constante', icon: BarChart3 },
                { value: 'randomized', label: 'Aleatorio', desc: 'Variacao natural', icon: RefreshCw },
                { value: 'custom', label: 'Personalizado', desc: 'Controle total', icon: Settings }
              ].map(recipe => (
                <button
                  key={recipe.value}
                  type="button"
                  onClick={() => handleRecipeChange(recipe.value)}
                  className={`p-3 rounded-xl border-2 transition-all text-center ${
                    form.recipe_type === recipe.value
                      ? 'border-orange-500 bg-orange-50'
                      : 'border-gray-200 hover:border-gray-300'
                  }`}
                >
                  <recipe.icon className={`w-5 h-5 mx-auto mb-1 ${
                    form.recipe_type === recipe.value ? 'text-orange-600' : 'text-gray-400'
                  }`} />
                  <div className="font-medium text-sm">{recipe.label}</div>
                  <div className="text-xs text-gray-500">{recipe.desc}</div>
                </button>
              ))}
            </div>
          </div>

          {/* Settings Grid */}
          <div className="grid grid-cols-3 gap-4 mb-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Min/dia</label>
              <input
                type="number"
                value={form.min_emails_per_day}
                onChange={(e) => handleMinChange(parseInt(e.target.value) || 1)}
                min="1"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Max/dia</label>
              <input
                type="number"
                value={form.max_emails_per_day}
                onChange={(e) => handleMaxChange(parseInt(e.target.value) || 40)}
                min="1"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Envio %</label>
              <input
                type="number"
                value={form.send_rate}
                onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 30 })}
                min="1"
                max="100"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
              <p className="text-xs text-gray-400 mt-1">Chance de enviar</p>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Resposta %</label>
              <input
                type="number"
                value={form.reply_rate}
                onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                min="0"
                max="100"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
              <p className="text-xs text-gray-400 mt-1">Chance de responder</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Hora inicio</label>
              <input
                type="number"
                value={form.start_hour}
                onChange={(e) => setForm({ ...form, start_hour: parseInt(e.target.value) || 0 })}
                min="0"
                max="23"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Hora fim</label>
              <input
                type="number"
                value={form.end_hour}
                onChange={(e) => setForm({ ...form, end_hour: parseInt(e.target.value) || 18 })}
                min="0"
                max="23"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
          </div>

          {/* Interactive Chart */}
          {loadingStats ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="w-8 h-8 animate-spin text-orange-500" />
            </div>
          ) : (
            <WarmupChart
              schedule={schedule}
              onChange={setSchedule}
              maxEmails={form.max_emails_per_day}
              startDate={warmupSMTP.start_date}
            />
          )}

          {/* Daily Stats Table (if any) */}
          {dailyStats.length > 0 && (
            <div className="bg-gray-50 rounded-xl p-4">
              <h4 className="font-medium text-gray-700 mb-3">Historico Recente</h4>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-gray-500">
                      <th className="pb-2">Data</th>
                      <th className="pb-2 text-center">Agendado</th>
                      <th className="pb-2 text-center">Enviado</th>
                      <th className="pb-2 text-center">Entrada</th>
                      <th className="pb-2 text-center">Spam</th>
                      <th className="pb-2 text-center">Respostas</th>
                      <th className="pb-2 text-center">Progresso</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {dailyStats.slice(0, 10).map((stat, idx) => (
                      <tr key={idx}>
                        <td className="py-2">{stat.date}</td>
                        <td className="py-2 text-center">{stat.scheduled}</td>
                        <td className="py-2 text-center font-medium">{stat.sent}</td>
                        <td className="py-2 text-center text-green-600">{stat.inbox}</td>
                        <td className="py-2 text-center text-red-600">{stat.spam}</td>
                        <td className="py-2 text-center text-purple-600">{stat.replies}</td>
                        <td className="py-2">
                          <div className="w-full bg-gray-200 rounded-full h-2">
                            <div
                              className="bg-green-500 h-2 rounded-full"
                              style={{ width: `${stat.scheduled > 0 ? (stat.sent / stat.scheduled) * 100 : 0}%` }}
                            ></div>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-3 p-6 border-t border-gray-100 sticky bottom-0 bg-white">
          <button
            type="button"
            onClick={onClose}
            className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50"
          >
            Cancelar
          </button>
          <button
            onClick={handleSubmit}
            disabled={loading}
            className="px-6 py-3 bg-orange-600 text-white rounded-xl font-medium hover:bg-orange-700 flex items-center gap-2"
          >
            {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Save className="w-5 h-5" />}
            Salvar Alteracoes
          </button>
        </div>
      </div>
    </div>
  )
}

// Modal para adicionar SMTP ao warmup
function AddWarmupSMTPModal({ smtps, onClose, onSave }) {
  const [form, setForm] = useState({
    smtp_id: '',
    recipe_type: 'progressive',
    start_date: new Date().toISOString().split('T')[0],
    end_date: '',
    min_emails_per_day: 5,
    max_emails_per_day: 40,
    send_rate: 30,
    reply_rate: 30,
    start_hour: 8,
    end_hour: 18
  })
  const [loading, setLoading] = useState(false)
  const [schedule, setSchedule] = useState([])

  // Gerar schedule inicial
  useEffect(() => {
    generateSchedule(form.recipe_type, form.min_emails_per_day, form.max_emails_per_day)
  }, [])

  const generateSchedule = (type, minEmails, maxEmails) => {
    const days = 45
    const newSchedule = []
    const increment = (maxEmails - minEmails) / (days - 1)

    for (let i = 0; i < days; i++) {
      let value
      if (type === 'flat') {
        value = minEmails
      } else if (type === 'randomized') {
        value = minEmails + Math.floor(Math.random() * (maxEmails - minEmails + 1))
      } else if (type === 'progressive') {
        value = minEmails + Math.round(i * increment)
      } else {
        value = minEmails
      }
      newSchedule.push(Math.min(value, maxEmails))
    }
    setSchedule(newSchedule)
  }

  const handleRecipeChange = (type) => {
    setForm({ ...form, recipe_type: type })
    generateSchedule(type, form.min_emails_per_day, form.max_emails_per_day)
  }

  const handleMinChange = (val) => {
    setForm({ ...form, min_emails_per_day: val })
    generateSchedule(form.recipe_type, val, form.max_emails_per_day)
  }

  const handleMaxChange = (val) => {
    setForm({ ...form, max_emails_per_day: val })
    generateSchedule(form.recipe_type, form.min_emails_per_day, val)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!form.smtp_id) {
      toast.error('Selecione um SMTP')
      return
    }
    setLoading(true)
    try {
      await api.post('/warmup/smtps', { ...form, custom_schedule: schedule })
      toast.success('SMTP adicionado ao warmup!')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao adicionar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl w-full max-w-4xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-gray-100 sticky top-0 bg-white">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-orange-100 rounded-xl">
              <Flame className="w-6 h-6 text-orange-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900">Adicionar SMTP ao Warmup</h2>
              <p className="text-sm text-gray-500">Configure o aquecimento do servidor</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-100 rounded-lg">
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-6">
          {/* SMTP Selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Servidor SMTP</label>
            <select
              value={form.smtp_id}
              onChange={(e) => setForm({ ...form, smtp_id: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none"
              required
            >
              <option value="">Selecione um SMTP...</option>
              {smtps.map(smtp => (
                <option key={smtp.id} value={smtp.id}>{smtp.name} ({smtp.host})</option>
              ))}
            </select>
          </div>

          {/* Recipe Type */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-3">Tipo de Aquecimento</label>
            <div className="grid grid-cols-4 gap-3">
              {[
                { value: 'progressive', label: 'Progressivo', desc: 'Recomendado', icon: TrendingUp },
                { value: 'flat', label: 'Fixo', desc: 'Volume constante', icon: BarChart3 },
                { value: 'randomized', label: 'Aleatorio', desc: 'Variacao natural', icon: RefreshCw },
                { value: 'custom', label: 'Personalizado', desc: 'Controle total', icon: Settings }
              ].map(recipe => (
                <button
                  key={recipe.value}
                  type="button"
                  onClick={() => handleRecipeChange(recipe.value)}
                  className={`p-3 rounded-xl border-2 transition-all text-center ${
                    form.recipe_type === recipe.value
                      ? 'border-orange-500 bg-orange-50'
                      : 'border-gray-200 hover:border-gray-300'
                  }`}
                >
                  <recipe.icon className={`w-5 h-5 mx-auto mb-1 ${
                    form.recipe_type === recipe.value ? 'text-orange-600' : 'text-gray-400'
                  }`} />
                  <div className="font-medium text-sm">{recipe.label}</div>
                  <div className="text-xs text-gray-500">{recipe.desc}</div>
                </button>
              ))}
            </div>
          </div>

          {/* Interactive Chart */}
          {schedule.length > 0 && (
            <WarmupChart
              schedule={schedule}
              onChange={setSchedule}
              maxEmails={form.max_emails_per_day}
              startDate={form.start_date}
            />
          )}

          {/* Settings Grid */}
          <div className="grid grid-cols-3 gap-4 mb-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Min/dia</label>
              <input
                type="number"
                value={form.min_emails_per_day}
                onChange={(e) => handleMinChange(parseInt(e.target.value) || 1)}
                min="1"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Max/dia</label>
              <input
                type="number"
                value={form.max_emails_per_day}
                onChange={(e) => handleMaxChange(parseInt(e.target.value) || 40)}
                min="1"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Envio %</label>
              <input
                type="number"
                value={form.send_rate}
                onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 30 })}
                min="1"
                max="100"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
              <p className="text-xs text-gray-400 mt-1">Chance de enviar</p>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Resposta %</label>
              <input
                type="number"
                value={form.reply_rate}
                onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                min="0"
                max="100"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
              <p className="text-xs text-gray-400 mt-1">Chance de responder</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Hora inicio</label>
              <input
                type="number"
                value={form.start_hour}
                onChange={(e) => setForm({ ...form, start_hour: parseInt(e.target.value) || 0 })}
                min="0"
                max="23"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Hora fim</label>
              <input
                type="number"
                value={form.end_hour}
                onChange={(e) => setForm({ ...form, end_hour: parseInt(e.target.value) || 18 })}
                min="0"
                max="23"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
            </div>
          </div>

          {/* Date Range */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Data Inicio</label>
              <input
                type="date"
                value={form.start_date}
                onChange={(e) => setForm({ ...form, start_date: e.target.value })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Data Fim (opcional)</label>
              <input
                type="date"
                value={form.end_date}
                onChange={(e) => setForm({ ...form, end_date: e.target.value })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none"
              />
              <p className="text-xs text-gray-400 mt-1">45 dias minimo recomendado</p>
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-4 border-t sticky bottom-0 bg-white">
            <button type="button" onClick={onClose} className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50">
              Cancelar
            </button>
            <button type="submit" disabled={loading} className="px-6 py-3 bg-orange-600 text-white rounded-xl font-medium hover:bg-orange-700 flex items-center gap-2">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Flame className="w-5 h-5" />}
              Iniciar Warmup
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Modal para adicionar Seed
function AddSeedModal({ onClose, onSave }) {
  const [bulkMode, setBulkMode] = useState(false)
  const [bulkText, setBulkText] = useState('')
  const [bulkLoading, setBulkLoading] = useState(false)
  const [bulkSettings, setBulkSettings] = useState({
    send_rate: 50,
    reply_rate: 50,
    emails_per_day: 20,
    auto_reply: true
  })
  const [form, setForm] = useState({
    email: '',
    password: '',
    provider: '',
    imap_host: '',
    imap_port: 993,
    imap_tls_mode: 'tls',
    smtp_host: '',
    smtp_port: 587,
    smtp_tls_mode: 'starttls',
    send_rate: 50,
    reply_rate: 50,
    emails_per_day: 20,
    auto_reply: true,
    oauth_token: '', // Microsoft OAuth2 refresh token
    oauth_client_id: '' // Microsoft OAuth2 client_id
  })
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResults, setTestResults] = useState(null)

  // Configurações pré-definidas por provedor
  const providerConfigs = {
    gmail: { provider: 'gmail', imap_host: 'imap.gmail.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.gmail.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    googlemail: { provider: 'gmail', imap_host: 'imap.gmail.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.gmail.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    yahoo: { provider: 'yahoo', imap_host: 'imap.mail.yahoo.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.yahoo.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    outlook: { provider: 'outlook', imap_host: 'outlook.office365.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.office365.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    hotmail: { provider: 'outlook', imap_host: 'outlook.office365.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.office365.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    live: { provider: 'outlook', imap_host: 'outlook.office365.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.office365.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    msn: { provider: 'outlook', imap_host: 'outlook.office365.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.office365.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    aol: { provider: 'aol', imap_host: 'imap.aol.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.aol.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    gmx: { provider: 'gmx', imap_host: 'imap.gmx.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'mail.gmx.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    icloud: { provider: 'icloud', imap_host: 'imap.mail.me.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.me.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    me: { provider: 'icloud', imap_host: 'imap.mail.me.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.me.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    mac: { provider: 'icloud', imap_host: 'imap.mail.me.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.me.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    zoho: { provider: 'zoho', imap_host: 'imap.zoho.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.zoho.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    zohomail: { provider: 'zoho', imap_host: 'imap.zoho.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.zoho.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    yandex: { provider: 'yandex', imap_host: 'imap.yandex.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.yandex.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    mail: { provider: 'mail.com', imap_host: 'imap.mail.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    protonmail: { provider: 'protonmail', imap_host: '127.0.0.1', imap_port: 1143, imap_tls_mode: 'starttls', smtp_host: '127.0.0.1', smtp_port: 1025, smtp_tls_mode: 'starttls' },
    proton: { provider: 'protonmail', imap_host: '127.0.0.1', imap_port: 1143, imap_tls_mode: 'starttls', smtp_host: '127.0.0.1', smtp_port: 1025, smtp_tls_mode: 'starttls' },
    fastmail: { provider: 'fastmail', imap_host: 'imap.fastmail.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.fastmail.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    // Brasil
    uol: { provider: 'uol', imap_host: 'imap.uol.com.br', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtps.uol.com.br', smtp_port: 587, smtp_tls_mode: 'starttls' },
    bol: { provider: 'bol', imap_host: 'imap.bol.com.br', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtps.bol.com.br', smtp_port: 587, smtp_tls_mode: 'starttls' },
    terra: { provider: 'terra', imap_host: 'imap.terra.com.br', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.terra.com.br', smtp_port: 587, smtp_tls_mode: 'starttls' },
    ig: { provider: 'ig', imap_host: 'imap.ig.com.br', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.ig.com.br', smtp_port: 587, smtp_tls_mode: 'starttls' },
    globo: { provider: 'globo', imap_host: 'imap.globo.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.globo.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    r7: { provider: 'r7', imap_host: 'imap.r7.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.r7.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    // Portugal
    sapo: { provider: 'sapo', imap_host: 'imap.sapo.pt', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.sapo.pt', smtp_port: 587, smtp_tls_mode: 'starttls' },
    // Outros
    mailru: { provider: 'mail.ru', imap_host: 'imap.mail.ru', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.mail.ru', smtp_port: 587, smtp_tls_mode: 'starttls' },
    seznam: { provider: 'seznam', imap_host: 'imap.seznam.cz', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.seznam.cz', smtp_port: 587, smtp_tls_mode: 'starttls' },
    web: { provider: 'web.de', imap_host: 'imap.web.de', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.web.de', smtp_port: 587, smtp_tls_mode: 'starttls' },
    t_online: { provider: 't-online', imap_host: 'secureimap.t-online.de', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'securesmtp.t-online.de', smtp_port: 587, smtp_tls_mode: 'starttls' },
    freenet: { provider: 'freenet', imap_host: 'mx.freenet.de', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'mx.freenet.de', smtp_port: 587, smtp_tls_mode: 'starttls' },
    // Empresariais
    office365: { provider: 'office365', imap_host: 'outlook.office365.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.office365.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    godaddy: { provider: 'godaddy', imap_host: 'imap.secureserver.net', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtpout.secureserver.net', smtp_port: 587, smtp_tls_mode: 'starttls' },
    bluehost: { provider: 'bluehost', imap_host: 'mail.bluehost.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'mail.bluehost.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    hostgator: { provider: 'hostgator', imap_host: 'mail.hostgator.com', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'mail.hostgator.com', smtp_port: 587, smtp_tls_mode: 'starttls' },
    locaweb: { provider: 'locaweb', imap_host: 'imap.email.locaweb.com.br', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.email.locaweb.com.br', smtp_port: 587, smtp_tls_mode: 'starttls' },
    kinghost: { provider: 'kinghost', imap_host: 'imap.kinghost.net', imap_port: 993, imap_tls_mode: 'tls', smtp_host: 'smtp.kinghost.net', smtp_port: 587, smtp_tls_mode: 'starttls' }
  }

  const detectProvider = (email) => {
    const domain = email.split('@')[1]?.toLowerCase() || ''

    // Procura match direto por nome de domínio
    for (const [key, config] of Object.entries(providerConfigs)) {
      if (domain.includes(key.replace('_', '-'))) {
        setForm(prev => ({ ...prev, email, ...config }))
        return config
      }
    }

    // Se não encontrar, mantém como 'other' mas não limpa os campos (caso o usuário já tenha preenchido)
    setForm(prev => ({ ...prev, email, provider: 'other' }))
    return null
  }

  // Função para processar credenciais no formato: email\tsenha\ttoken\tclient_id
  const processCredentials = (text) => {
    if (!text || !text.trim()) {
      toast.error('Texto vazio')
      return
    }

    // Tenta parsear o formato: email  senha  token  client_id (separado por tab ou múltiplos espaços)
    const parts = text.trim().split(/\t+|\s{2,}/)

    if (parts.length >= 2) {
      const email = parts[0].trim()
      const password = parts[1].trim()
      const rawToken = parts.length >= 3 ? parts[2].trim() : ''
      const rawClientId = parts.length >= 4 ? parts[3].trim() : ''

      // Detecta o provedor e preenche automaticamente
      const domain = email.split('@')[1]?.toLowerCase() || ''
      let providerConfig = null
      let isOutlook = false

      for (const [key, config] of Object.entries(providerConfigs)) {
        if (domain.includes(key.replace('_', '-'))) {
          providerConfig = config
          // Verifica se é Outlook/Hotmail/Live/MSN (provedores Microsoft)
          isOutlook = ['outlook', 'hotmail', 'live', 'msn'].includes(key)
          break
        }
      }

      // OAuth token e client_id só são usados para Outlook/Hotmail/Live/MSN
      const oauthToken = isOutlook ? rawToken : ''
      const oauthClientId = isOutlook ? rawClientId : ''

      if (providerConfig) {
        setForm(prev => ({ ...prev, email, password, oauth_token: oauthToken, oauth_client_id: oauthClientId, ...providerConfig }))
      } else {
        setForm(prev => ({ ...prev, email, password, oauth_token: '', oauth_client_id: '', provider: 'other' }))
      }

      // Mostra mensagem diferente se tem token OAuth
      if (oauthToken && oauthClientId) {
        toast.success(`OAuth2 completo: ${email} (token + client_id)`)
      } else if (oauthToken) {
        toast.success(`Credenciais + OAuth Token: ${email}`)
      } else if (rawToken && !isOutlook) {
        toast.success(`Credenciais preenchidas: ${email} (token ignorado - não é Outlook)`)
      } else {
        toast.success(`Credenciais preenchidas: ${email}`)
      }
    } else {
      toast.error('Formato inválido. Use: email  senha  token  client_id')
    }
  }

  const testConnection = async (testType) => {
    if (!form.email || !form.password) {
      toast.error('Preencha email e senha primeiro')
      return
    }
    setTesting(true)
    setTestResults(null)
    try {
      const res = await api.post('/warmup/seeds/test-connection', { ...form, test_type: testType })
      setTestResults(res.data)
      if (res.data.imap?.success && (testType === 'imap' || !res.data.smtp)) {
        toast.success('Conexão IMAP OK!')
      } else if (res.data.smtp?.success && testType === 'smtp') {
        toast.success('Conexão SMTP OK!')
      } else if (res.data.imap?.success && res.data.smtp?.success) {
        toast.success('Conexões OK!')
      } else {
        const errors = []
        if (res.data.imap?.error) errors.push(`IMAP: ${res.data.imap.error}`)
        if (res.data.smtp?.error) errors.push(`SMTP: ${res.data.smtp.error}`)
        toast.error(errors.join('\n') || 'Erro na conexão')
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao testar')
    } finally {
      setTesting(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      await api.post('/warmup/seeds', form)
      toast.success('Conta seed adicionada!')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao adicionar')
    } finally {
      setLoading(false)
    }
  }

  // Função para importação em massa
  const handleBulkImport = async () => {
    if (!bulkText.trim()) {
      toast.error('Cole as credenciais primeiro')
      return
    }

    const lines = bulkText.trim().split('\n').filter(line => line.trim())
    if (lines.length === 0) {
      toast.error('Nenhuma linha válida encontrada')
      return
    }

    setBulkLoading(true)
    let success = 0
    let failed = 0
    const errors = []

    for (const line of lines) {
      const parts = line.trim().split(/\t+|\s{2,}/)
      if (parts.length < 2) {
        failed++
        errors.push(`Linha inválida: ${line.substring(0, 30)}...`)
        continue
      }

      const email = parts[0].trim()
      const password = parts[1].trim()
      const rawToken = parts.length >= 3 ? parts[2].trim() : ''
      const rawClientId = parts.length >= 4 ? parts[3].trim() : ''

      // Detecta o provedor
      const domain = email.split('@')[1]?.toLowerCase() || ''
      let providerConfig = null
      let isOutlook = false

      for (const [key, config] of Object.entries(providerConfigs)) {
        if (domain.includes(key.replace('_', '-'))) {
          providerConfig = config
          isOutlook = ['outlook', 'hotmail', 'live', 'msn'].includes(key)
          break
        }
      }

      // Monta o objeto da seed
      const seedData = {
        email,
        password,
        provider: providerConfig?.provider || 'other',
        imap_host: providerConfig?.imap_host || '',
        imap_port: providerConfig?.imap_port || 993,
        imap_tls_mode: providerConfig?.imap_tls_mode || 'tls',
        smtp_host: providerConfig?.smtp_host || '',
        smtp_port: providerConfig?.smtp_port || 587,
        smtp_tls_mode: providerConfig?.smtp_tls_mode || 'starttls',
        send_rate: bulkSettings.send_rate,
        reply_rate: bulkSettings.reply_rate,
        emails_per_day: bulkSettings.emails_per_day,
        auto_reply: bulkSettings.auto_reply,
        oauth_token: isOutlook ? rawToken : '',
        oauth_client_id: isOutlook ? rawClientId : ''
      }

      try {
        await api.post('/warmup/seeds', seedData)
        success++
      } catch (error) {
        failed++
        errors.push(`${email}: ${error.response?.data?.error || 'Erro'}`)
      }
    }

    setBulkLoading(false)

    if (success > 0) {
      toast.success(`${success} contas importadas com sucesso!`)
    }
    if (failed > 0) {
      toast.error(`${failed} falhas: ${errors.slice(0, 3).join(', ')}${errors.length > 3 ? '...' : ''}`)
    }

    if (success > 0) {
      onSave()
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-gray-100">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-blue-100 rounded-xl">
              <Mail className="w-6 h-6 text-blue-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900">Adicionar Conta Seed</h2>
              <p className="text-sm text-gray-500">Conta IMAP para receber emails de warmup</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-100 rounded-lg">
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        {/* Toggle Modo Bulk */}
        <div className="px-6 pt-4">
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={bulkMode}
              onChange={(e) => setBulkMode(e.target.checked)}
              className="w-5 h-5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <div>
              <span className="font-medium text-gray-900">Importar em Massa</span>
              <p className="text-xs text-gray-500">Adicionar múltiplas contas de uma vez</p>
            </div>
          </label>
        </div>

        {bulkMode ? (
          /* Modo Bulk - Textarea para múltiplas linhas */
          <div className="p-6 space-y-4">
            <div className="p-4 bg-gradient-to-r from-blue-50 to-indigo-50 border border-blue-200 rounded-xl">
              <div className="flex items-center gap-2 mb-3">
                <Upload className="w-5 h-5 text-blue-600" />
                <span className="font-medium text-blue-900">Cole as credenciais abaixo</span>
              </div>
              <textarea
                value={bulkText}
                onChange={(e) => setBulkText(e.target.value)}
                placeholder={`email1@outlook.com\tsenha1\ttoken1\tclient_id1
email2@outlook.com\tsenha2\ttoken2\tclient_id2
email3@gmail.com\tsenha3`}
                className="w-full h-48 px-3 py-2 border border-blue-200 rounded-lg text-sm font-mono focus:ring-2 focus:ring-blue-500 outline-none bg-white resize-none"
              />
              <div className="mt-2 text-xs text-blue-700 space-y-1">
                <p><strong>Formato:</strong> email TAB senha TAB token TAB client_id</p>
                <p>• Uma conta por linha</p>
                <p>• Token e client_id são opcionais (só para Outlook/Hotmail)</p>
                <p>• Provedor detectado automaticamente pelo domínio</p>
              </div>
            </div>

            {bulkText && (
              <div className="p-3 bg-gray-50 rounded-lg">
                <p className="text-sm text-gray-600">
                  <strong>{bulkText.trim().split('\n').filter(l => l.trim()).length}</strong> linhas detectadas
                </p>
              </div>
            )}

            {/* Configurações de Warmup para Importação em Massa */}
            <div className="p-4 bg-purple-50 rounded-xl space-y-3">
              <h4 className="font-medium text-purple-900">Configurações para todas as contas</h4>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Envio %</label>
                  <input
                    type="number"
                    min="0"
                    max="100"
                    value={bulkSettings.send_rate}
                    onChange={(e) => setBulkSettings({ ...bulkSettings, send_rate: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Resposta %</label>
                  <input
                    type="number"
                    min="0"
                    max="100"
                    value={bulkSettings.reply_rate}
                    onChange={(e) => setBulkSettings({ ...bulkSettings, reply_rate: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Emails/dia</label>
                  <input
                    type="number"
                    min="1"
                    value={bulkSettings.emails_per_day}
                    onChange={(e) => setBulkSettings({ ...bulkSettings, emails_per_day: parseInt(e.target.value) || 1 })}
                    className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                  />
                </div>
              </div>
              <div className="flex items-center gap-3 pt-2">
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={bulkSettings.auto_reply}
                    onChange={(e) => setBulkSettings({ ...bulkSettings, auto_reply: e.target.checked })}
                    className="sr-only peer"
                  />
                  <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-purple-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-purple-600"></div>
                </label>
                <span className="text-sm text-gray-700">Resposta automática ativada</span>
              </div>
            </div>

            <button
              type="button"
              onClick={handleBulkImport}
              disabled={bulkLoading || !bulkText.trim()}
              className="w-full py-3 bg-gradient-to-r from-blue-600 to-indigo-600 text-white rounded-xl font-semibold hover:from-blue-700 hover:to-indigo-700 transition-all disabled:opacity-50 flex items-center justify-center gap-2"
            >
              {bulkLoading ? (
                <>
                  <RefreshCw className="w-5 h-5 animate-spin" />
                  Importando...
                </>
              ) : (
                <>
                  <Upload className="w-5 h-5" />
                  Importar {bulkText.trim().split('\n').filter(l => l.trim()).length || 0} Contas
                </>
              )}
            </button>
          </div>
        ) : (
        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          {/* Campo para Colar Credenciais */}
          <div className="p-3 bg-gradient-to-r from-purple-50 to-indigo-50 border border-purple-200 rounded-xl">
            <div className="flex items-center gap-2 mb-2">
              <ClipboardPaste className="w-4 h-4 text-purple-600" />
              <label className="text-sm font-medium text-purple-900">Colar Credenciais (Ctrl+V)</label>
            </div>
            <input
              type="text"
              placeholder="Cole aqui: email    senha    token"
              className="w-full px-3 py-2 border border-purple-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none bg-white"
              onPaste={(e) => {
                e.preventDefault()
                const text = e.clipboardData.getData('text')
                processCredentials(text)
              }}
              onChange={(e) => {
                // Também processa se o usuário digitar/colar de outra forma
                if (e.target.value.includes('\t') || e.target.value.split(/\s{2,}/).length >= 2) {
                  processCredentials(e.target.value)
                  e.target.value = ''
                }
              }}
            />
            <p className="text-xs text-purple-600 mt-1">
              Formato: email TAB senha TAB token (preenche tudo automaticamente)
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Email</label>
            <input
              type="email"
              value={form.email}
              onChange={(e) => detectProvider(e.target.value)}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              placeholder="conta@gmail.com"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Senha / App Password</label>
            <input
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              placeholder="App password do Gmail/Yahoo/Outlook"
              required
            />
            <p className="text-xs text-gray-400 mt-1">Use "App Password" para Gmail/Yahoo/Outlook com 2FA</p>
          </div>

          {/* OAuth Token Indicator */}
          {form.oauth_token && (
            <div className={`flex items-center gap-2 p-3 rounded-xl ${form.oauth_client_id ? 'bg-green-50 border border-green-200' : 'bg-yellow-50 border border-yellow-200'}`}>
              <CheckCircle className={`w-5 h-5 ${form.oauth_client_id ? 'text-green-600' : 'text-yellow-600'}`} />
              <div className="flex-1">
                <p className={`text-sm font-medium ${form.oauth_client_id ? 'text-green-800' : 'text-yellow-800'}`}>
                  {form.oauth_client_id ? 'OAuth2 Completo (Token + Client ID)' : 'OAuth2 Token (falta Client ID)'}
                </p>
                <p className={`text-xs ${form.oauth_client_id ? 'text-green-600' : 'text-yellow-600'}`}>
                  {form.oauth_client_id
                    ? `Client ID: ${form.oauth_client_id.substring(0, 8)}...`
                    : 'Formato esperado: email  senha  token  client_id'}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setForm(prev => ({ ...prev, oauth_token: '', oauth_client_id: '' }))}
                className={`p-1 rounded ${form.oauth_client_id ? 'hover:bg-green-100' : 'hover:bg-yellow-100'}`}
                title="Remover OAuth"
              >
                <X className={`w-4 h-4 ${form.oauth_client_id ? 'text-green-600' : 'text-yellow-600'}`} />
              </button>
            </div>
          )}

          {/* IMAP Settings */}
          <div className="p-4 bg-blue-50 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <h4 className="font-medium text-blue-900">Configurações IMAP</h4>
              <button
                type="button"
                onClick={() => testConnection('imap')}
                disabled={testing}
                className="px-3 py-1 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700 flex items-center gap-1"
              >
                {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <TestTube className="w-4 h-4" />}
                Testar IMAP
              </button>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-1">
                <label className="block text-xs font-medium text-gray-600 mb-1">Host</label>
                <input
                  type="text"
                  value={form.imap_host}
                  onChange={(e) => setForm({ ...form, imap_host: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                  placeholder="imap.gmail.com"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Porta</label>
                <input
                  type="number"
                  value={form.imap_port}
                  onChange={(e) => setForm({ ...form, imap_port: parseInt(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Protocolo TLS</label>
                <select
                  value={form.imap_tls_mode}
                  onChange={(e) => setForm({ ...form, imap_tls_mode: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                >
                  <option value="tls">TLS (993)</option>
                  <option value="starttls">STARTTLS (143)</option>
                  <option value="none">Nenhum</option>
                </select>
              </div>
            </div>
            {testResults?.imap && (
              <div className={`text-sm p-2 rounded-lg ${testResults.imap.success ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                {testResults.imap.success ? '✓ ' + testResults.imap.message : '✗ ' + testResults.imap.error}
              </div>
            )}
          </div>

          {/* SMTP Settings */}
          <div className="p-4 bg-orange-50 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <h4 className="font-medium text-orange-900">Configurações SMTP (Opcional)</h4>
              <button
                type="button"
                onClick={() => testConnection('smtp')}
                disabled={testing || !form.smtp_host}
                className="px-3 py-1 bg-orange-600 text-white text-sm rounded-lg hover:bg-orange-700 flex items-center gap-1 disabled:opacity-50"
              >
                {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <TestTube className="w-4 h-4" />}
                Testar SMTP
              </button>
            </div>
            <p className="text-xs text-orange-700">Para seeds que também enviam emails (bidirectional warmup)</p>
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-1">
                <label className="block text-xs font-medium text-gray-600 mb-1">Host</label>
                <input
                  type="text"
                  value={form.smtp_host}
                  onChange={(e) => setForm({ ...form, smtp_host: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                  placeholder="smtp.gmail.com"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Porta</label>
                <input
                  type="number"
                  value={form.smtp_port}
                  onChange={(e) => setForm({ ...form, smtp_port: parseInt(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Protocolo TLS</label>
                <select
                  value={form.smtp_tls_mode}
                  onChange={(e) => setForm({ ...form, smtp_tls_mode: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                >
                  <option value="tls">TLS (465)</option>
                  <option value="starttls">STARTTLS (587)</option>
                  <option value="none">Nenhum</option>
                </select>
              </div>
            </div>
            {testResults?.smtp && (
              <div className={`text-sm p-2 rounded-lg ${testResults.smtp.success ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                {testResults.smtp.success ? '✓ ' + testResults.smtp.message : '✗ ' + testResults.smtp.error}
              </div>
            )}
          </div>

          {/* Configurações de Warmup */}
          <div className="p-4 bg-purple-50 rounded-xl space-y-3">
            <h4 className="font-medium text-purple-900">Configurações de Warmup</h4>
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Envio %</label>
                <input
                  type="number"
                  min="0"
                  max="100"
                  value={form.send_rate}
                  onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 0 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Chance de enviar</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Resposta %</label>
                <input
                  type="number"
                  min="0"
                  max="100"
                  value={form.reply_rate}
                  onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Chance de responder</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Emails/dia</label>
                <input
                  type="number"
                  min="1"
                  value={form.emails_per_day}
                  onChange={(e) => setForm({ ...form, emails_per_day: parseInt(e.target.value) || 1 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Limite diário</p>
              </div>
            </div>
            <div className="flex items-center gap-3 pt-2">
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.auto_reply}
                  onChange={(e) => setForm({ ...form, auto_reply: e.target.checked })}
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-purple-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-purple-600"></div>
              </label>
              <span className="text-sm text-gray-700">Resposta automática ativada</span>
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-4 border-t">
            <button type="button" onClick={onClose} className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50">
              Cancelar
            </button>
            <button
              type="button"
              onClick={() => testConnection('both')}
              disabled={testing}
              className="px-6 py-3 bg-gray-600 text-white rounded-xl font-medium hover:bg-gray-700 flex items-center gap-2"
            >
              {testing ? <Loader2 className="w-5 h-5 animate-spin" /> : <TestTube className="w-5 h-5" />}
              Testar Tudo
            </button>
            <button type="submit" disabled={loading} className="px-6 py-3 bg-blue-600 text-white rounded-xl font-medium hover:bg-blue-700 flex items-center gap-2">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Plus className="w-5 h-5" />}
              Adicionar
            </button>
          </div>
        </form>
        )}
      </div>
    </div>
  )
}

// Modal para editar conta seed existente
function EditSeedModal({ seed, onClose, onSave }) {
  const [form, setForm] = useState({
    email: seed.email || '',
    password: '',
    provider: seed.provider || 'other',
    imap_host: seed.imap_host || '',
    imap_port: seed.imap_port || 993,
    imap_tls_mode: seed.imap_tls_mode || 'tls',
    smtp_host: seed.smtp_host || '',
    smtp_port: seed.smtp_port || 587,
    smtp_tls_mode: seed.smtp_tls_mode || 'starttls',
    send_rate: seed.send_rate ?? 50,
    reply_rate: seed.reply_rate ?? 50,
    emails_per_day: seed.emails_per_day ?? 20,
    auto_reply: seed.auto_reply ?? true
  })
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResults, setTestResults] = useState(null)

  const testConnection = async (testType) => {
    if (!form.email) {
      toast.error('Email é obrigatório')
      return
    }
    setTesting(true)
    setTestResults(null)
    try {
      const testData = { ...form }
      if (!testData.password) {
        testData.password = '__KEEP_EXISTING__'
      }
      const res = await api.post('/warmup/seeds/test-connection', { ...testData, test_type: testType })
      setTestResults(res.data)
      if (res.data.imap?.success && (testType === 'imap' || !res.data.smtp)) {
        toast.success('Conexão IMAP OK!')
      } else if (res.data.smtp?.success && testType === 'smtp') {
        toast.success('Conexão SMTP OK!')
      } else if (res.data.imap?.success && res.data.smtp?.success) {
        toast.success('Conexões OK!')
      } else {
        const errors = []
        if (res.data.imap?.error) errors.push(`IMAP: ${res.data.imap.error}`)
        if (res.data.smtp?.error) errors.push(`SMTP: ${res.data.smtp.error}`)
        toast.error(errors.join('\n') || 'Erro na conexão')
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao testar')
    } finally {
      setTesting(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      const updateData = { ...form }
      if (!updateData.password) {
        delete updateData.password
      }
      await api.put(`/warmup/seeds/${seed.id}`, updateData)
      toast.success('Conta seed atualizada!')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao atualizar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-gray-100">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-green-100 rounded-xl">
              <Settings className="w-6 h-6 text-green-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900">Editar Conta Seed</h2>
              <p className="text-sm text-gray-500">{seed.email}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-100 rounded-lg">
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Email</label>
            <input
              type="email"
              value={form.email}
              disabled
              className="w-full px-4 py-3 border border-gray-200 rounded-xl bg-gray-50 text-gray-500 cursor-not-allowed"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Nova Senha (deixe vazio para manter)</label>
            <input
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-green-500 focus:border-transparent outline-none"
              placeholder="Deixe vazio para manter a senha atual"
            />
          </div>

          {/* IMAP Settings */}
          <div className="p-4 bg-blue-50 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <h4 className="font-medium text-blue-900">Configurações IMAP</h4>
              <button
                type="button"
                onClick={() => testConnection('imap')}
                disabled={testing}
                className="px-3 py-1 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700 flex items-center gap-1"
              >
                {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <TestTube className="w-4 h-4" />}
                Testar IMAP
              </button>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-1">
                <label className="block text-xs font-medium text-gray-600 mb-1">Host</label>
                <input
                  type="text"
                  value={form.imap_host}
                  onChange={(e) => setForm({ ...form, imap_host: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Porta</label>
                <input
                  type="number"
                  value={form.imap_port}
                  onChange={(e) => setForm({ ...form, imap_port: parseInt(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">TLS</label>
                <select
                  value={form.imap_tls_mode}
                  onChange={(e) => setForm({ ...form, imap_tls_mode: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                >
                  <option value="tls">TLS (993)</option>
                  <option value="starttls">STARTTLS</option>
                  <option value="none">Nenhum</option>
                </select>
              </div>
            </div>
            {testResults?.imap && (
              <div className={`text-sm p-2 rounded-lg ${testResults.imap.success ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                {testResults.imap.success ? '✓ ' + testResults.imap.message : '✗ ' + testResults.imap.error}
              </div>
            )}
          </div>

          {/* SMTP Settings */}
          <div className="p-4 bg-orange-50 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <h4 className="font-medium text-orange-900">Configurações SMTP</h4>
              <button
                type="button"
                onClick={() => testConnection('smtp')}
                disabled={testing || !form.smtp_host}
                className="px-3 py-1 bg-orange-600 text-white text-sm rounded-lg hover:bg-orange-700 flex items-center gap-1 disabled:opacity-50"
              >
                {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <TestTube className="w-4 h-4" />}
                Testar SMTP
              </button>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-1">
                <label className="block text-xs font-medium text-gray-600 mb-1">Host</label>
                <input
                  type="text"
                  value={form.smtp_host}
                  onChange={(e) => setForm({ ...form, smtp_host: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Porta</label>
                <input
                  type="number"
                  value={form.smtp_port}
                  onChange={(e) => setForm({ ...form, smtp_port: parseInt(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">TLS</label>
                <select
                  value={form.smtp_tls_mode}
                  onChange={(e) => setForm({ ...form, smtp_tls_mode: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-orange-500 outline-none"
                >
                  <option value="tls">TLS (465)</option>
                  <option value="starttls">STARTTLS (587)</option>
                  <option value="none">Nenhum</option>
                </select>
              </div>
            </div>
            {testResults?.smtp && (
              <div className={`text-sm p-2 rounded-lg ${testResults.smtp.success ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                {testResults.smtp.success ? '✓ ' + testResults.smtp.message : '✗ ' + testResults.smtp.error}
              </div>
            )}
          </div>

          {/* Configurações de Warmup */}
          <div className="p-4 bg-purple-50 rounded-xl space-y-3">
            <h4 className="font-medium text-purple-900">Configurações de Warmup</h4>
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Envio %</label>
                <input
                  type="number"
                  min="0"
                  max="100"
                  value={form.send_rate}
                  onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 0 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Chance de enviar</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Resposta %</label>
                <input
                  type="number"
                  min="0"
                  max="100"
                  value={form.reply_rate}
                  onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Chance de responder</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Emails/dia</label>
                <input
                  type="number"
                  min="1"
                  value={form.emails_per_day}
                  onChange={(e) => setForm({ ...form, emails_per_day: parseInt(e.target.value) || 1 })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:ring-2 focus:ring-purple-500 outline-none"
                />
                <p className="text-xs text-gray-400 mt-0.5">Limite diário</p>
              </div>
            </div>
            <div className="flex items-center gap-3 pt-2">
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.auto_reply}
                  onChange={(e) => setForm({ ...form, auto_reply: e.target.checked })}
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-purple-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-purple-600"></div>
              </label>
              <span className="text-sm text-gray-700">Resposta automática ativada</span>
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-4 border-t">
            <button type="button" onClick={onClose} className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={loading}
              className="px-6 py-3 bg-green-600 text-white rounded-xl font-medium hover:bg-green-700 flex items-center gap-2"
            >
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Save className="w-5 h-5" />}
              Salvar
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Componente principal
function Warmup() {
  const [loading, setLoading] = useState(true)
  const [stats, setStats] = useState({})
  const [warmupSMTPs, setWarmupSMTPs] = useState([])
  const [seeds, setSeeds] = useState([])
  const [activity, setActivity] = useState([])
  const [availableSMTPs, setAvailableSMTPs] = useState([])
  const [showAddSMTP, setShowAddSMTP] = useState(false)
  const [showAddSeed, setShowAddSeed] = useState(false)
  const [editingSeed, setEditingSeed] = useState(null)
  const [editingWarmup, setEditingWarmup] = useState(null)
  const [activeTab, setActiveTab] = useState('dashboard')
  const [templates, setTemplates] = useState([])
  const [showAddTemplate, setShowAddTemplate] = useState(false)
  const [newTemplate, setNewTemplate] = useState({ subject: '', body: '', category: 'business', template_type: 'send' })
  const [showDiagnostic, setShowDiagnostic] = useState(false)
  const [diagnostic, setDiagnostic] = useState(null)
  const [triggeringWarmup, setTriggeringWarmup] = useState(false)

  useEffect(() => {
    fetchAll()

    // Auto-refresh every 10 seconds
    const interval = setInterval(() => {
      fetchAllSilent()
    }, 10000)

    return () => clearInterval(interval)
  }, [])

  // Silent fetch without loading state (for auto-refresh)
  const fetchAllSilent = async () => {
    try {
      const availableRes = await api.get('/smtp')
      setAvailableSMTPs(availableRes.data?.data || [])

      // Fetch each endpoint separately so one failure doesn't affect others
      api.get('/warmup/stats').then(r => setStats(r.data || {})).catch(() => {})
      api.get('/warmup/smtps').then(r => setWarmupSMTPs(r.data || [])).catch(() => {})
      api.get('/warmup/seeds').then(r => setSeeds(r.data || [])).catch(() => {})
      api.get('/warmup/activity').then(r => setActivity(r.data || [])).catch(() => {})
      api.get('/warmup/templates').then(r => setTemplates(r.data || [])).catch(() => {})
    } catch (error) {
      // Silent fail for auto-refresh
    }
  }

  const fetchAll = async () => {
    setLoading(true)
    try {
      // Fetch SMTPs first (this should always work)
      const availableRes = await api.get('/smtp')
      setAvailableSMTPs(availableRes.data?.data || [])

      // Fetch each warmup endpoint separately so one failure doesn't affect others
      try {
        const statsRes = await api.get('/warmup/stats')
        setStats(statsRes.data || {})
      } catch (e) { console.log('Stats error:', e) }

      try {
        const smtpsRes = await api.get('/warmup/smtps')
        setWarmupSMTPs(smtpsRes.data || [])
      } catch (e) { console.log('SMTPs error:', e) }

      try {
        const seedsRes = await api.get('/warmup/seeds')
        setSeeds(seedsRes.data || [])
      } catch (e) { console.log('Seeds error:', e) }

      try {
        const activityRes = await api.get('/warmup/activity')
        setActivity(activityRes.data || [])
      } catch (e) { console.log('Activity error:', e) }

      try {
        const templatesRes = await api.get('/warmup/templates')
        setTemplates(templatesRes.data || [])
      } catch (e) { console.log('Templates error:', e) }

    } catch (error) {
      console.error('Error fetching data:', error)
    } finally {
      setLoading(false)
    }
  }

  const toggleWarmup = async (id) => {
    try {
      const res = await api.post(`/warmup/smtps/${id}/toggle`)
      toast.success(`Warmup ${res.data.status === 'active' ? 'ativado' : 'pausado'}`)
      fetchAll()
    } catch (error) {
      toast.error('Erro ao alterar status')
    }
  }

  const triggerWarmup = async (id, count = 1) => {
    try {
      const res = await api.post(`/warmup/smtps/${id}/trigger?count=${count}`)
      if (res.data.sent > 0) {
        toast.success(`✉️ ${res.data.message}`)
      } else {
        toast.error(res.data.errors?.[0] || 'Nenhum email enviado')
      }
      fetchAll()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao enviar warmup')
    }
  }

  const deleteWarmupSMTP = async (id) => {
    if (!confirm('Remover este SMTP do warmup?')) return
    try {
      await api.delete(`/warmup/smtps/${id}`)
      toast.success('SMTP removido do warmup')
      fetchAll()
    } catch (error) {
      toast.error('Erro ao remover')
    }
  }

  const toggleInternalWarmup = async (id) => {
    try {
      const res = await api.post(`/warmup/smtps/${id}/internal-warmup`)
      toast.success(`Aquecimento interno ${res.data.internal_warmup ? 'ativado' : 'desativado'}`)
      fetchAll()
    } catch (error) {
      toast.error('Erro ao alterar aquecimento interno')
    }
  }

  const testSeed = async (id) => {
    try {
      await api.post(`/warmup/seeds/${id}/test`)
      toast.success('Conexao IMAP OK!')
      fetchAll()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Falha na conexao')
    }
  }

  const fetchDiagnostic = async () => {
    try {
      const res = await api.get('/warmup/diagnostic')
      setDiagnostic(res.data)
      setShowDiagnostic(true)
    } catch (error) {
      toast.error('Erro ao buscar diagnóstico')
    }
  }

  const triggerWarmupNow = async (type) => {
    setTriggeringWarmup(true)
    try {
      await api.post('/warmup/trigger', { type })
      toast.success('Ciclo de warmup disparado! Verifique os logs.')
      setTimeout(fetchAll, 3000)
    } catch (error) {
      toast.error('Erro ao disparar warmup')
    } finally {
      setTriggeringWarmup(false)
    }
  }

  const checkIMAP = async () => {
    try {
      await api.post('/warmup/check-imap')
      toast.success('Verificação IMAP iniciada! Aguarde...')
      setTimeout(fetchAll, 3000)
    } catch (error) {
      toast.error('Erro ao verificar IMAP')
    }
  }

  const deleteSeed = async (id) => {
    if (!confirm('Remover esta conta seed?')) return
    try {
      await api.delete(`/warmup/seeds/${id}`)
      toast.success('Seed removida')
      fetchAll()
    } catch (error) {
      toast.error('Erro ao remover')
    }
  }

  const toggleSeed = async (id) => {
    try {
      const res = await api.post(`/warmup/seeds/${id}/toggle`)
      toast.success(`Seed ${res.data.status === 'active' ? 'ativada' : 'pausada'}`)
      fetchAll()
    } catch (error) {
      toast.error('Erro ao alterar status')
    }
  }

  const triggerSeedSend = async (id) => {
    try {
      const res = await api.post(`/warmup/seeds/${id}/trigger`)
      toast.success(`Email enviado para ${res.data.to}`)
      fetchAll()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao enviar')
    }
  }

  const addTemplate = async () => {
    if (!newTemplate.subject || !newTemplate.body) {
      toast.error('Preencha assunto e corpo')
      return
    }
    try {
      await api.post('/warmup/templates', newTemplate)
      toast.success('Template adicionado!')
      setNewTemplate({ subject: '', body: '', category: 'business', template_type: 'send' })
      setShowAddTemplate(false)
      fetchAll()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao adicionar')
    }
  }

  const deleteTemplate = async (id) => {
    if (!confirm('Remover este template?')) return
    try {
      await api.delete(`/warmup/templates/${id}`)
      toast.success('Template removido')
      fetchAll()
    } catch (error) {
      toast.error('Erro ao remover')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-orange-500" />
      </div>
    )
  }

  const sendTemplates = templates.filter(t => t.template_type !== 'reply')
  const replyTemplates = templates.filter(t => t.template_type === 'reply')

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
            <Flame className="w-7 h-7 text-orange-500" />
            Warmup de SMTP
          </h1>
          <p className="text-sm text-gray-500 mt-1">Aquecimento automatico com interacao simulada</p>
        </div>
        <div className="flex gap-3">
          {activeTab === 'dashboard' && (
            <>
              <button
                onClick={fetchDiagnostic}
                className="btn bg-yellow-100 text-yellow-700 hover:bg-yellow-200 flex items-center gap-2"
              >
                <AlertCircle className="w-4 h-4" />
                Diagnóstico
              </button>
              <button
                onClick={() => setShowAddSeed(true)}
                className="btn btn-secondary flex items-center gap-2"
              >
                <Mail className="w-4 h-4" />
                Nova Seed
              </button>
              <button
                onClick={() => setShowAddSMTP(true)}
                className="btn btn-primary flex items-center gap-2"
              >
                <Plus className="w-4 h-4" />
                Adicionar SMTP
              </button>
            </>
          )}
          {activeTab === 'templates' && (
            <button
              onClick={() => setShowAddTemplate(true)}
              className="btn btn-primary flex items-center gap-2"
            >
              <Plus className="w-4 h-4" />
              Novo Template
            </button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 mb-6 border-b border-gray-200">
        <button
          onClick={() => setActiveTab('dashboard')}
          className={`px-4 py-3 font-medium transition-colors border-b-2 -mb-px ${
            activeTab === 'dashboard'
              ? 'text-orange-600 border-orange-600'
              : 'text-gray-500 border-transparent hover:text-gray-700'
          }`}
        >
          <div className="flex items-center gap-2">
            <BarChart3 className="w-4 h-4" />
            Dashboard
          </div>
        </button>
        <button
          onClick={() => setActiveTab('smtps')}
          className={`px-4 py-3 font-medium transition-colors border-b-2 -mb-px ${
            activeTab === 'smtps'
              ? 'text-orange-600 border-orange-600'
              : 'text-gray-500 border-transparent hover:text-gray-700'
          }`}
        >
          <div className="flex items-center gap-2">
            <Server className="w-4 h-4" />
            SMTPs
            <span className="px-2 py-0.5 text-xs bg-gray-100 rounded-full">{warmupSMTPs.length}</span>
          </div>
        </button>
        <button
          onClick={() => setActiveTab('seeds')}
          className={`px-4 py-3 font-medium transition-colors border-b-2 -mb-px ${
            activeTab === 'seeds'
              ? 'text-orange-600 border-orange-600'
              : 'text-gray-500 border-transparent hover:text-gray-700'
          }`}
        >
          <div className="flex items-center gap-2">
            <Mail className="w-4 h-4" />
            Seeds
            <span className="px-2 py-0.5 text-xs bg-gray-100 rounded-full">{seeds.length}</span>
          </div>
        </button>
        <button
          onClick={() => setActiveTab('templates')}
          className={`px-4 py-3 font-medium transition-colors border-b-2 -mb-px ${
            activeTab === 'templates'
              ? 'text-orange-600 border-orange-600'
              : 'text-gray-500 border-transparent hover:text-gray-700'
          }`}
        >
          <div className="flex items-center gap-2">
            <FileText className="w-4 h-4" />
            Templates
            <span className="px-2 py-0.5 text-xs bg-gray-100 rounded-full">{templates.length}</span>
          </div>
        </button>
      </div>

      {/* Dashboard Tab */}
      {activeTab === 'dashboard' && (
        <>
      {/* Stats Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-blue-100 rounded-xl">
              <Mail className="w-6 h-6 text-blue-600" />
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-900">{stats.total_sent || 0}</p>
              <p className="text-sm text-gray-500">Emails Enviados</p>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-green-100 rounded-xl">
              <Inbox className="w-6 h-6 text-green-600" />
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-900">{stats.total_interactions || 0}</p>
              <p className="text-sm text-gray-500">Interacoes</p>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-purple-100 rounded-xl">
              <MessageSquare className="w-6 h-6 text-purple-600" />
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-900">{stats.total_replies || 0}</p>
              <p className="text-sm text-gray-500">Respostas</p>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-red-100 rounded-xl">
              <AlertCircle className="w-6 h-6 text-red-600" />
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-900">{stats.spam_rate?.toFixed(1) || 0}%</p>
              <p className="text-sm text-gray-500">Taxa de Spam</p>
            </div>
          </div>
        </div>
      </div>

      {/* Dashboard Stats Grid */}
      <div className="grid grid-cols-2 gap-6 mb-6">
        {/* Entrada vs Spam Donut */}
        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <h3 className="font-semibold text-gray-900 mb-4">Entrada vs Spam</h3>
          <div className="relative w-40 h-40 mx-auto">
            <svg className="w-full h-full" viewBox="0 0 36 36">
              <path
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                fill="none"
                stroke="#E5E7EB"
                strokeWidth="3"
              />
              <path
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                fill="none"
                stroke="#3B82F6"
                strokeWidth="3"
                strokeDasharray={`${stats.inbox_rate || 100}, 100`}
                strokeLinecap="round"
              />
            </svg>
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="text-center">
                <p className="text-2xl font-bold text-blue-600">{stats.inbox_rate?.toFixed(1) || 100}%</p>
                <p className="text-xs text-gray-500">Entrada</p>
              </div>
            </div>
          </div>
          <div className="flex justify-center gap-6 mt-4">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-blue-500 rounded-full"></div>
              <span className="text-sm text-gray-600">Entrada</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-gray-200 rounded-full"></div>
              <span className="text-sm text-gray-600">Spam</span>
            </div>
          </div>
        </div>

        {/* Resumo Rápido */}
        <div className="bg-white rounded-2xl p-6 border border-gray-100">
          <h3 className="font-semibold text-gray-900 mb-4">Resumo</h3>
          <div className="space-y-4">
            <div className="flex items-center justify-between p-3 bg-orange-50 rounded-lg">
              <div className="flex items-center gap-3">
                <Server className="w-5 h-5 text-orange-600" />
                <span className="text-gray-700">SMTPs em Aquecimento</span>
              </div>
              <span className="text-xl font-bold text-orange-600">{warmupSMTPs.length}</span>
            </div>
            <div className="flex items-center justify-between p-3 bg-blue-50 rounded-lg">
              <div className="flex items-center gap-3">
                <Mail className="w-5 h-5 text-blue-600" />
                <span className="text-gray-700">Contas Seed</span>
              </div>
              <span className="text-xl font-bold text-blue-600">{seeds.length}</span>
            </div>
            <div className="flex items-center justify-between p-3 bg-purple-50 rounded-lg">
              <div className="flex items-center gap-3">
                <FileText className="w-5 h-5 text-purple-600" />
                <span className="text-gray-700">Templates</span>
              </div>
              <span className="text-xl font-bold text-purple-600">{templates.length}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Recent Activity */}
      <div className="bg-white rounded-2xl p-6 border border-gray-100">
        <h3 className="font-semibold text-gray-900 mb-4">Atividade Recente</h3>
        {activity.length === 0 ? (
          <p className="text-gray-500 text-center py-8">Nenhuma atividade recente</p>
        ) : (
          <div className="space-y-3 max-h-80 overflow-y-auto">
            {activity.map(item => (
              <div key={item.id} className="flex items-center gap-4 p-3 bg-gray-50 rounded-lg">
                <div className={`p-2 rounded-lg ${
                  item.warmup_type === 'reply' ? 'bg-indigo-100' :
                  item.warmup_type === 'internal' ? 'bg-purple-100' :
                  item.warmup_type === 'seed_to_smtp' ? 'bg-teal-100' :
                  item.warmup_type === 'moved_to_inbox' ? 'bg-green-100' :
                  item.status === 'opened' ? 'bg-green-100' : 'bg-blue-100'
                }`}>
                  {item.warmup_type === 'reply' ? (
                    <MessageSquare className="w-4 h-4 text-indigo-600" />
                  ) : item.warmup_type === 'internal' ? (
                    <Link2 className="w-4 h-4 text-purple-600" />
                  ) : item.warmup_type === 'seed_to_smtp' ? (
                    <ArrowUpRight className="w-4 h-4 text-teal-600" />
                  ) : item.warmup_type === 'moved_to_inbox' ? (
                    <CheckCircle className="w-4 h-4 text-green-600" />
                  ) : item.status === 'opened' ? (
                    <CheckCircle className="w-4 h-4 text-green-600" />
                  ) : (
                    <Mail className="w-4 h-4 text-blue-600" />
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-medium text-gray-900 truncate">{item.subject}</p>
                    {item.warmup_type === 'reply' && (
                      <span className="px-1.5 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-700 rounded">Resposta</span>
                    )}
                    {item.warmup_type === 'internal' && (
                      <span className="px-1.5 py-0.5 text-xs font-medium bg-purple-100 text-purple-700 rounded">Interno</span>
                    )}
                    {item.warmup_type === 'seed_to_smtp' && (
                      <span className="px-1.5 py-0.5 text-xs font-medium bg-teal-100 text-teal-700 rounded">Seed→SMTP</span>
                    )}
                    {item.warmup_type === 'smtp_to_seed' && (
                      <span className="px-1.5 py-0.5 text-xs font-medium bg-blue-100 text-blue-700 rounded">SMTP→Seed</span>
                    )}
                    {item.warmup_type === 'moved_to_inbox' && (
                      <span className="px-1.5 py-0.5 text-xs font-medium bg-green-100 text-green-700 rounded">Movido p/ Entrada</span>
                    )}
                  </div>
                  <p className="text-xs text-gray-500 truncate">
                    {item.from_email && item.to_email
                      ? `${item.from_email} → ${item.to_email}`
                      : item.to_email || item.from_email || item.seed_email}
                  </p>
                </div>
                <div className="text-xs text-gray-400">
                  {new Date(item.sent_at).toLocaleString('pt-BR')}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
        </>
      )}

      {/* SMTPs Tab */}
      {activeTab === 'smtps' && (
        <div className="space-y-6">
          <div className="bg-white rounded-2xl p-6 border border-gray-100">
            <div className="flex items-center justify-between mb-4">
              <h3 className="font-semibold text-gray-900">SMTPs em Aquecimento</h3>
              <button
                onClick={() => setShowAddSMTP(true)}
                className="flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors"
              >
                <Plus className="w-4 h-4" />
                Adicionar SMTP
              </button>
            </div>
            {warmupSMTPs.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <Server className="w-16 h-16 mx-auto mb-4 opacity-30" />
                <p className="text-lg">Nenhum SMTP em aquecimento</p>
                <p className="text-sm mt-2">Adicione um SMTP para começar o aquecimento</p>
              </div>
            ) : (
              <div className="space-y-4">
                {warmupSMTPs.map(smtp => (
                  <div key={smtp.id} className="flex items-center justify-between p-4 bg-gray-50 rounded-xl border border-gray-100">
                    <div className="flex items-center gap-4">
                      <div className={`p-3 rounded-xl ${smtp.status === 'active' ? 'bg-green-100' : 'bg-gray-200'}`}>
                        <Server className={`w-6 h-6 ${smtp.status === 'active' ? 'text-green-600' : 'text-gray-400'}`} />
                      </div>
                      <div>
                        <p className="font-semibold text-gray-900">{smtp.smtp_name}</p>
                        <p className="text-sm text-gray-500">
                          Dia {smtp.current_day} | {smtp.min_emails_per_day}-{smtp.max_emails_per_day} emails/dia
                        </p>
                        <div className="flex gap-4 mt-1 text-xs">
                          <span className="text-blue-600">Enviados: {smtp.total_sent}</span>
                          <span className="text-green-600">Entrada: {smtp.total_inbox}</span>
                          <span className="text-red-600">Spam: {smtp.total_spam}</span>
                          <span className="text-purple-600">Respostas: {smtp.total_replies}</span>
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                        smtp.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-200 text-gray-600'
                      }`}>
                        {smtp.status === 'active' ? 'Ativo' : 'Pausado'}
                      </span>
                      <button
                        onClick={() => triggerWarmup(smtp.id)}
                        className="p-2 hover:bg-blue-50 rounded-lg"
                        title="Enviar email"
                      >
                        <Send className="w-5 h-5 text-blue-500" />
                      </button>
                      <button
                        onClick={() => toggleWarmup(smtp.id)}
                        className="p-2 hover:bg-gray-100 rounded-lg"
                        title={smtp.status === 'active' ? 'Pausar' : 'Ativar'}
                      >
                        {smtp.status === 'active' ? (
                          <Pause className="w-5 h-5 text-gray-400" />
                        ) : (
                          <Play className="w-5 h-5 text-green-500" />
                        )}
                      </button>
                      <button
                        onClick={() => setEditingWarmup(smtp)}
                        className="p-2 hover:bg-gray-100 rounded-lg"
                        title="Editar"
                      >
                        <Settings className="w-5 h-5 text-gray-400" />
                      </button>
                      <button
                        onClick={() => deleteWarmupSMTP(smtp.id)}
                        className="p-2 hover:bg-red-50 rounded-lg"
                        title="Remover"
                      >
                        <Trash2 className="w-5 h-5 text-red-400" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Seeds Tab */}
      {activeTab === 'seeds' && (
        <div className="space-y-6">
          <div className="bg-white rounded-2xl p-6 border border-gray-100">
            <div className="flex items-center justify-between mb-4">
              <h3 className="font-semibold text-gray-900">Contas Seed (IMAP)</h3>
              <div className="flex items-center gap-3">
                <button
                  onClick={checkIMAP}
                  className="flex items-center gap-2 px-4 py-2 bg-blue-50 text-blue-600 rounded-lg hover:bg-blue-100 transition-colors"
                >
                  <RefreshCw className="w-4 h-4" />
                  Verificar Emails
                </button>
                <button
                  onClick={() => setShowAddSeed(true)}
                  className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
                >
                  <Plus className="w-4 h-4" />
                  Nova Seed
                </button>
              </div>
            </div>
            {seeds.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <Mail className="w-16 h-16 mx-auto mb-4 opacity-30" />
                <p className="text-lg">Nenhuma conta seed cadastrada</p>
                <p className="text-sm mt-2">Adicione contas seed para receber emails de warmup</p>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                {seeds.map(seed => (
                  <div key={seed.id} className="p-4 border border-gray-200 rounded-xl hover:border-gray-300 transition-colors">
                    <div className="flex items-center justify-between mb-3">
                      <span className={`px-2 py-1 rounded text-xs font-medium ${
                        seed.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                      }`}>
                        {seed.provider?.toUpperCase()}
                      </span>
                      <div className="flex gap-1">
                        <button onClick={() => triggerSeedSend(seed.id)} className="p-1.5 hover:bg-cyan-50 rounded-lg" title="Enviar email">
                          <Send className="w-4 h-4 text-cyan-500" />
                        </button>
                        <button onClick={() => toggleSeed(seed.id)} className="p-1.5 hover:bg-gray-100 rounded-lg">
                          {seed.status === 'active' ? <Pause className="w-4 h-4 text-gray-400" /> : <Play className="w-4 h-4 text-green-500" />}
                        </button>
                        <button onClick={() => setEditingSeed(seed)} className="p-1.5 hover:bg-gray-100 rounded-lg">
                          <Edit className="w-4 h-4 text-gray-400" />
                        </button>
                        <button onClick={() => deleteSeed(seed.id)} className="p-1.5 hover:bg-red-50 rounded-lg">
                          <Trash2 className="w-4 h-4 text-red-400" />
                        </button>
                      </div>
                    </div>
                    <p className="font-medium text-gray-900 truncate">{seed.email}</p>
                    <p className="text-xs text-gray-500 mt-1">{seed.imap_host}</p>
                    <div className="mt-3 pt-3 border-t border-gray-100">
                      <div className="grid grid-cols-3 gap-2 text-xs">
                        <div><span className="text-gray-500">Enviados:</span> <span className="font-medium">{seed.total_sent || 0}</span></div>
                        <div><span className="text-gray-500">Recebidos:</span> <span className="font-medium">{seed.total_received || 0}</span></div>
                        <div><span className="text-gray-500">Respondidos:</span> <span className="font-medium">{seed.total_replied || 0}</span></div>
                      </div>
                      <div className="flex justify-between mt-2 text-xs">
                        <span className="text-green-600">Entrada: {seed.total_inbox || 0}</span>
                        <span className="text-red-600">Spam: {seed.total_spam || 0}</span>
                        <span className="text-orange-600">Movidos: {seed.total_moved || 0}</span>
                      </div>
                    </div>
                    <div className="mt-2 text-xs text-gray-400">
                      Taxas: Envio {seed.send_rate}% | Resposta {seed.reply_rate}% | Limite {seed.emails_per_day}/dia
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Templates Tab */}
      {activeTab === 'templates' && (
        <div className="space-y-6">
          {/* Send Templates */}
          <div className="bg-white rounded-2xl p-6 border border-gray-100">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-blue-100 rounded-lg">
                  <Send className="w-5 h-5 text-blue-600" />
                </div>
                <div>
                  <h3 className="font-semibold text-gray-900">Templates de Envio</h3>
                  <p className="text-sm text-gray-500">Usados para enviar emails de warmup</p>
                </div>
              </div>
              <span className="px-3 py-1 bg-blue-100 text-blue-700 rounded-full text-sm font-medium">
                {sendTemplates.length} templates
              </span>
            </div>
            {sendTemplates.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <FileText className="w-12 h-12 mx-auto mb-4 opacity-30" />
                <p>Nenhum template de envio</p>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                {sendTemplates.map(template => (
                  <div key={template.id} className="p-4 border border-gray-100 rounded-xl hover:border-blue-200 transition-colors">
                    <div className="flex items-start justify-between mb-2">
                      <span className={`px-2 py-1 text-xs font-medium rounded ${
                        template.category === 'business' ? 'bg-blue-100 text-blue-700' :
                        template.category === 'casual' ? 'bg-green-100 text-green-700' :
                        'bg-purple-100 text-purple-700'
                      }`}>
                        {template.category}
                      </span>
                      <button
                        onClick={() => deleteTemplate(template.id)}
                        className="p-1 hover:bg-red-50 rounded"
                      >
                        <Trash2 className="w-4 h-4 text-red-400" />
                      </button>
                    </div>
                    <h4 className="font-medium text-gray-900 mb-2">{template.subject}</h4>
                    <p className="text-sm text-gray-500 line-clamp-3">{template.body}</p>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Reply Templates */}
          <div className="bg-white rounded-2xl p-6 border border-gray-100">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-purple-100 rounded-lg">
                  <Reply className="w-5 h-5 text-purple-600" />
                </div>
                <div>
                  <h3 className="font-semibold text-gray-900">Templates de Resposta</h3>
                  <p className="text-sm text-gray-500">Usados para responder emails recebidos</p>
                </div>
              </div>
              <span className="px-3 py-1 bg-purple-100 text-purple-700 rounded-full text-sm font-medium">
                {replyTemplates.length} templates
              </span>
            </div>
            {replyTemplates.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <Reply className="w-12 h-12 mx-auto mb-4 opacity-30" />
                <p>Nenhum template de resposta</p>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                {replyTemplates.map(template => (
                  <div key={template.id} className="p-4 border border-gray-100 rounded-xl hover:border-purple-200 transition-colors">
                    <div className="flex items-start justify-between mb-2">
                      <span className="px-2 py-1 text-xs font-medium rounded bg-purple-100 text-purple-700">
                        resposta
                      </span>
                      <button
                        onClick={() => deleteTemplate(template.id)}
                        className="p-1 hover:bg-red-50 rounded"
                      >
                        <Trash2 className="w-4 h-4 text-red-400" />
                      </button>
                    </div>
                    <h4 className="font-medium text-gray-900 mb-2">{template.subject}</h4>
                    <p className="text-sm text-gray-500 line-clamp-3">{template.body}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Add Template Modal */}
      {showAddTemplate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-2xl w-full max-w-lg">
            <div className="flex items-center justify-between p-6 border-b border-gray-100">
              <h2 className="text-lg font-bold text-gray-900">Novo Template</h2>
              <button onClick={() => setShowAddTemplate(false)} className="p-2 hover:bg-gray-100 rounded-lg">
                <X className="w-5 h-5 text-gray-400" />
              </button>
            </div>
            <div className="p-6 space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Tipo</label>
                  <select
                    value={newTemplate.template_type}
                    onChange={(e) => setNewTemplate({ ...newTemplate, template_type: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 outline-none"
                  >
                    <option value="send">Envio</option>
                    <option value="reply">Resposta</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Categoria</label>
                  <select
                    value={newTemplate.category}
                    onChange={(e) => setNewTemplate({ ...newTemplate, category: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 outline-none"
                  >
                    <option value="business">Business</option>
                    <option value="casual">Casual</option>
                    <option value="newsletter">Newsletter</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Assunto</label>
                <input
                  type="text"
                  value={newTemplate.subject}
                  onChange={(e) => setNewTemplate({ ...newTemplate, subject: e.target.value })}
                  className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 outline-none"
                  placeholder="Ex: Duvida sobre seus servicos"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Corpo do Email</label>
                <textarea
                  value={newTemplate.body}
                  onChange={(e) => setNewTemplate({ ...newTemplate, body: e.target.value })}
                  rows={6}
                  className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-orange-500 outline-none resize-none"
                  placeholder="Ola,&#10;&#10;Escreva o conteudo do email aqui...&#10;&#10;Atenciosamente"
                />
              </div>
              <div className="flex justify-end gap-3 pt-4 border-t">
                <button
                  onClick={() => setShowAddTemplate(false)}
                  className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50"
                >
                  Cancelar
                </button>
                <button
                  onClick={addTemplate}
                  className="px-6 py-3 bg-orange-600 text-white rounded-xl font-medium hover:bg-orange-700 flex items-center gap-2"
                >
                  <Plus className="w-5 h-5" />
                  Adicionar
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Modals */}
      {showAddSMTP && (
        <AddWarmupSMTPModal
          smtps={availableSMTPs.filter(s => !warmupSMTPs.find(w => w.smtp_id === s.id))}
          onClose={() => setShowAddSMTP(false)}
          onSave={() => { setShowAddSMTP(false); fetchAll() }}
        />
      )}

      {showAddSeed && (
        <AddSeedModal
          onClose={() => setShowAddSeed(false)}
          onSave={() => { setShowAddSeed(false); fetchAll() }}
        />
      )}

      {editingSeed && (
        <EditSeedModal
          seed={editingSeed}
          onClose={() => setEditingSeed(null)}
          onSave={() => { setEditingSeed(null); fetchAll() }}
        />
      )}

      {editingWarmup && (
        <EditWarmupModal
          warmupSMTP={editingWarmup}
          onClose={() => setEditingWarmup(null)}
          onSave={() => { setEditingWarmup(null); fetchAll() }}
        />
      )}

      {/* Diagnostic Modal */}
      {showDiagnostic && diagnostic && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-2xl w-full max-w-3xl max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between p-6 border-b border-gray-100">
              <div className="flex items-center gap-3">
                <div className={`p-3 rounded-xl ${diagnostic.ok ? 'bg-green-100' : 'bg-red-100'}`}>
                  {diagnostic.ok ? (
                    <CheckCircle className="w-6 h-6 text-green-600" />
                  ) : (
                    <AlertCircle className="w-6 h-6 text-red-600" />
                  )}
                </div>
                <div>
                  <h2 className="text-lg font-bold text-gray-900">Diagnóstico do Warmup</h2>
                  <p className="text-sm text-gray-500">{diagnostic.timestamp} (Hora: {diagnostic.current_hour})</p>
                </div>
              </div>
              <button onClick={() => setShowDiagnostic(false)} className="p-2 hover:bg-gray-100 rounded-lg">
                <X className="w-5 h-5 text-gray-400" />
              </button>
            </div>

            <div className="p-6 space-y-6">
              {/* Issues */}
              {diagnostic.issues && diagnostic.issues.length > 0 && (
                <div className="bg-red-50 border border-red-200 rounded-xl p-4">
                  <h3 className="font-semibold text-red-800 mb-2 flex items-center gap-2">
                    <XCircle className="w-5 h-5" />
                    Problemas Encontrados ({diagnostic.issues.length})
                  </h3>
                  <ul className="space-y-1">
                    {diagnostic.issues.map((issue, i) => (
                      <li key={i} className="text-sm text-red-700 flex items-start gap-2">
                        <span className="text-red-400">•</span>
                        {issue}
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {diagnostic.ok && (
                <div className="bg-green-50 border border-green-200 rounded-xl p-4">
                  <h3 className="font-semibold text-green-800 flex items-center gap-2">
                    <CheckCircle className="w-5 h-5" />
                    Tudo OK! O sistema de warmup está configurado corretamente.
                  </h3>
                </div>
              )}

              {/* External Warmup */}
              <div className="bg-blue-50 rounded-xl p-4">
                <h3 className="font-semibold text-blue-900 mb-3">SMTP → Seeds (Externo)</h3>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                  <div>
                    <p className="text-gray-500">Seeds Ativas</p>
                    <p className="font-bold text-blue-700">{diagnostic.external_warmup?.active_seeds || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">Templates Ativos</p>
                    <p className="font-bold text-blue-700">{diagnostic.external_warmup?.active_templates || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">SMTPs Ativos</p>
                    <p className="font-bold text-blue-700">{diagnostic.external_warmup?.active_smtps || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">No Horário</p>
                    <p className="font-bold text-blue-700">{diagnostic.external_warmup?.smtps_in_hours || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">Enviados Hoje</p>
                    <p className="font-bold text-green-700">{diagnostic.external_warmup?.sent_today || 0}</p>
                  </div>
                </div>
              </div>

              {/* Internal Warmup */}
              <div className="bg-purple-50 rounded-xl p-4">
                <h3 className="font-semibold text-purple-900 mb-3">SMTP → SMTP (Interno)</h3>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                  <div>
                    <p className="text-gray-500">SMTPs c/ Interno</p>
                    <p className="font-bold text-purple-700">{diagnostic.internal_warmup?.smtps_with_internal || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">No Horário</p>
                    <p className="font-bold text-purple-700">{diagnostic.internal_warmup?.smtps_in_hours || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">Senders c/ IMAP</p>
                    <p className="font-bold text-purple-700">{diagnostic.internal_warmup?.senders_with_imap || 0}</p>
                  </div>
                  <div>
                    <p className="text-gray-500">Enviados Hoje</p>
                    <p className="font-bold text-green-700">{diagnostic.internal_warmup?.sent_today || 0}</p>
                  </div>
                </div>
              </div>

              {/* SMTP Details */}
              {diagnostic.smtp_details && diagnostic.smtp_details.length > 0 && (
                <div>
                  <h3 className="font-semibold text-gray-900 mb-3">Detalhes por SMTP</h3>
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead className="bg-gray-50">
                        <tr>
                          <th className="px-3 py-2 text-left">Host</th>
                          <th className="px-3 py-2 text-center">Status</th>
                          <th className="px-3 py-2 text-center">Interno</th>
                          <th className="px-3 py-2 text-center">Horário</th>
                          <th className="px-3 py-2 text-center">No Horário</th>
                          <th className="px-3 py-2 text-center">Limite</th>
                          <th className="px-3 py-2 text-center">Enviados</th>
                          <th className="px-3 py-2 text-center">Senders</th>
                          <th className="px-3 py-2 text-center">c/ IMAP</th>
                        </tr>
                      </thead>
                      <tbody>
                        {diagnostic.smtp_details.map((smtp, i) => (
                          <tr key={i} className="border-t">
                            <td className="px-3 py-2 font-medium">{smtp.host}</td>
                            <td className="px-3 py-2 text-center">
                              <span className={`px-2 py-0.5 rounded text-xs ${smtp.status === 'active' && smtp.server_active ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                                {smtp.status === 'active' && smtp.server_active ? 'Ativo' : 'Inativo'}
                              </span>
                            </td>
                            <td className="px-3 py-2 text-center">
                              {smtp.internal ? '✓' : '-'}
                            </td>
                            <td className="px-3 py-2 text-center">{smtp.start_hour}-{smtp.end_hour}h</td>
                            <td className="px-3 py-2 text-center">
                              {smtp.in_hours ? (
                                <span className="text-green-600">✓</span>
                              ) : (
                                <span className="text-red-500">✗</span>
                              )}
                            </td>
                            <td className="px-3 py-2 text-center">{smtp.today_limit}</td>
                            <td className="px-3 py-2 text-center">{smtp.sent_today}</td>
                            <td className="px-3 py-2 text-center">{smtp.senders_count}</td>
                            <td className="px-3 py-2 text-center">{smtp.senders_with_imap}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Actions */}
              <div className="flex justify-between items-center pt-4 border-t">
                <div className="flex gap-2">
                  <button
                    onClick={() => triggerWarmupNow('external')}
                    disabled={triggeringWarmup}
                    className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm flex items-center gap-2"
                  >
                    {triggeringWarmup ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                    Forçar SMTP→Seed
                  </button>
                  <button
                    onClick={() => triggerWarmupNow('internal')}
                    disabled={triggeringWarmup}
                    className="px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 text-sm flex items-center gap-2"
                  >
                    {triggeringWarmup ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                    Forçar SMTP→SMTP
                  </button>
                  <button
                    onClick={() => triggerWarmupNow('both')}
                    disabled={triggeringWarmup}
                    className="px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 text-sm flex items-center gap-2"
                  >
                    {triggeringWarmup ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                    Forçar Ambos
                  </button>
                </div>
                <button
                  onClick={fetchDiagnostic}
                  className="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 text-sm flex items-center gap-2"
                >
                  <RefreshCw className="w-4 h-4" />
                  Atualizar
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default Warmup
