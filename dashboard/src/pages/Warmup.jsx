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
  Send
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
          <div className="grid grid-cols-5 gap-4">
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
              <label className="block text-sm font-medium text-gray-700 mb-2">Resposta %</label>
              <input
                type="number"
                value={form.reply_rate}
                onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                min="0"
                max="45"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
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
          <div className="grid grid-cols-5 gap-4">
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
              <label className="block text-sm font-medium text-gray-700 mb-2">Resposta %</label>
              <input
                type="number"
                value={form.reply_rate}
                onChange={(e) => setForm({ ...form, reply_rate: parseInt(e.target.value) || 0 })}
                min="0"
                max="45"
                className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-transparent outline-none text-center"
              />
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
  const [form, setForm] = useState({
    email: '',
    password: '',
    provider: '',
    imap_host: '',
    imap_port: 993,
    smtp_host: '',
    smtp_port: 587,
    use_tls: true
  })
  const [loading, setLoading] = useState(false)

  const detectProvider = (email) => {
    const domain = email.split('@')[1]?.toLowerCase() || ''
    if (domain.includes('gmail')) {
      setForm({ ...form, email, provider: 'gmail', imap_host: 'imap.gmail.com', imap_port: 993, smtp_host: 'smtp.gmail.com', smtp_port: 587 })
    } else if (domain.includes('yahoo')) {
      setForm({ ...form, email, provider: 'yahoo', imap_host: 'imap.mail.yahoo.com', imap_port: 993, smtp_host: 'smtp.mail.yahoo.com', smtp_port: 587 })
    } else if (domain.includes('outlook') || domain.includes('hotmail') || domain.includes('live')) {
      setForm({ ...form, email, provider: 'outlook', imap_host: 'outlook.office365.com', imap_port: 993, smtp_host: 'smtp.office365.com', smtp_port: 587 })
    } else {
      setForm({ ...form, email, provider: 'other' })
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

        <form onSubmit={handleSubmit} className="p-6 space-y-4">
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

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">IMAP Host</label>
              <input
                type="text"
                value={form.imap_host}
                onChange={(e) => setForm({ ...form, imap_host: e.target.value })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
                placeholder="imap.gmail.com"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">IMAP Porta</label>
              <input
                type="number"
                value={form.imap_port}
                onChange={(e) => setForm({ ...form, imap_port: parseInt(e.target.value) })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">SMTP Host</label>
              <input
                type="text"
                value={form.smtp_host}
                onChange={(e) => setForm({ ...form, smtp_host: e.target.value })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
                placeholder="smtp.gmail.com"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">SMTP Porta</label>
              <input
                type="number"
                value={form.smtp_port}
                onChange={(e) => setForm({ ...form, smtp_port: parseInt(e.target.value) })}
                className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              />
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-4 border-t">
            <button type="button" onClick={onClose} className="px-6 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50">
              Cancelar
            </button>
            <button type="submit" disabled={loading} className="px-6 py-3 bg-blue-600 text-white rounded-xl font-medium hover:bg-blue-700 flex items-center gap-2">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Plus className="w-5 h-5" />}
              Adicionar
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
  const [editingWarmup, setEditingWarmup] = useState(null)

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

      try {
        const [statsRes, smtpsRes, seedsRes, activityRes] = await Promise.all([
          api.get('/warmup/stats'),
          api.get('/warmup/smtps'),
          api.get('/warmup/seeds'),
          api.get('/warmup/activity')
        ])
        setStats(statsRes.data || {})
        setWarmupSMTPs(smtpsRes.data || [])
        setSeeds(seedsRes.data || [])
        setActivity(activityRes.data || [])
      } catch (warmupError) {
        // Silent fail for auto-refresh
      }
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

      // Fetch warmup data (may fail if tables don't exist yet)
      try {
        const [statsRes, smtpsRes, seedsRes, activityRes] = await Promise.all([
          api.get('/warmup/stats'),
          api.get('/warmup/smtps'),
          api.get('/warmup/seeds'),
          api.get('/warmup/activity')
        ])
        setStats(statsRes.data || {})
        setWarmupSMTPs(smtpsRes.data || [])
        setSeeds(seedsRes.data || [])
        setActivity(activityRes.data || [])
      } catch (warmupError) {
        console.log('Warmup data not available yet:', warmupError)
        // Set defaults
        setStats({})
        setWarmupSMTPs([])
        setSeeds([])
        setActivity([])
      }
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

  const testSeed = async (id) => {
    try {
      await api.post(`/warmup/seeds/${id}/test`)
      toast.success('Conexao IMAP OK!')
      fetchAll()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Falha na conexao')
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

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-orange-500" />
      </div>
    )
  }

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
        </div>
      </div>

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

      {/* Inbox vs Spam Donut */}
      <div className="grid grid-cols-3 gap-6 mb-6">
        <div className="col-span-2 bg-white rounded-2xl p-6 border border-gray-100">
          <h3 className="font-semibold text-gray-900 mb-4">SMTPs em Aquecimento</h3>
          {warmupSMTPs.length === 0 ? (
            <div className="text-center py-8 text-gray-500">
              <Flame className="w-12 h-12 mx-auto mb-4 opacity-30" />
              <p>Nenhum SMTP em aquecimento</p>
              <button onClick={() => setShowAddSMTP(true)} className="text-orange-600 hover:underline mt-2">
                Adicionar SMTP ao warmup
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {warmupSMTPs.map(smtp => (
                <div key={smtp.id} className="flex items-center justify-between p-4 bg-gray-50 rounded-xl">
                  <div className="flex items-center gap-4">
                    <div className={`p-2 rounded-lg ${smtp.status === 'active' ? 'bg-green-100' : 'bg-gray-200'}`}>
                      <Server className={`w-5 h-5 ${smtp.status === 'active' ? 'text-green-600' : 'text-gray-400'}`} />
                    </div>
                    <div>
                      <p className="font-medium text-gray-900">{smtp.smtp_name}</p>
                      <p className="text-sm text-gray-500">
                        Dia {smtp.current_day} | {smtp.total_sent} enviados | {smtp.inbox_rate?.toFixed(1)}% entrada
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`px-3 py-1 rounded-full text-xs font-medium ${
                      smtp.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-200 text-gray-600'
                    }`}>
                      {smtp.status === 'active' ? 'Ativo' : 'Pausado'}
                    </span>
                    <button
                      onClick={() => setEditingWarmup(smtp)}
                      className="p-2 hover:bg-orange-100 rounded-lg transition-colors"
                      title="Configurar"
                    >
                      <Settings className="w-4 h-4 text-orange-600" />
                    </button>
                    <button
                      onClick={() => toggleWarmup(smtp.id)}
                      className="p-2 hover:bg-gray-200 rounded-lg transition-colors"
                      title={smtp.status === 'active' ? 'Pausar' : 'Ativar'}
                    >
                      {smtp.status === 'active' ? (
                        <Pause className="w-4 h-4 text-gray-600" />
                      ) : (
                        <Play className="w-4 h-4 text-green-600" />
                      )}
                    </button>
                    <button
                      onClick={() => triggerWarmup(smtp.id, 3)}
                      className="p-2 hover:bg-blue-100 rounded-lg transition-colors"
                      title="Enviar 3 emails agora (teste)"
                    >
                      <Mail className="w-4 h-4 text-blue-600" />
                    </button>
                    <button
                      onClick={() => deleteWarmupSMTP(smtp.id)}
                      className="p-2 hover:bg-red-100 rounded-lg transition-colors"
                      title="Remover"
                    >
                      <Trash2 className="w-4 h-4 text-red-500" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

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
      </div>

      {/* Seeds Section */}
      <div className="bg-white rounded-2xl p-6 border border-gray-100 mb-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="font-semibold text-gray-900">Contas Seed (IMAP)</h3>
          <div className="flex items-center gap-3">
            <button
              onClick={checkIMAP}
              className="flex items-center gap-2 px-3 py-1.5 bg-blue-50 text-blue-600 rounded-lg hover:bg-blue-100 transition-colors text-sm"
            >
              <RefreshCw className="w-4 h-4" />
              Verificar Emails
            </button>
            <span className="text-sm text-gray-500">{seeds.length} contas</span>
          </div>
        </div>

        {seeds.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            <Mail className="w-12 h-12 mx-auto mb-4 opacity-30" />
            <p>Nenhuma conta seed cadastrada</p>
            <button onClick={() => setShowAddSeed(true)} className="text-blue-600 hover:underline mt-2">
              Adicionar conta seed
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-4">
            {seeds.map(seed => (
              <div key={seed.id} className="p-4 border border-gray-100 rounded-xl">
                <div className="flex items-center justify-between mb-2">
                  <span className={`px-2 py-1 rounded text-xs font-medium ${
                    seed.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                  }`}>
                    {seed.provider?.toUpperCase()}
                  </span>
                  <div className="flex gap-1">
                    <button
                      onClick={() => testSeed(seed.id)}
                      className="p-1.5 hover:bg-gray-100 rounded-lg"
                      title="Testar conexao"
                    >
                      <TestTube className="w-4 h-4 text-gray-400" />
                    </button>
                    <button
                      onClick={() => deleteSeed(seed.id)}
                      className="p-1.5 hover:bg-red-50 rounded-lg"
                      title="Remover"
                    >
                      <Trash2 className="w-4 h-4 text-red-400" />
                    </button>
                  </div>
                </div>
                <p className="font-medium text-gray-900 text-sm truncate">{seed.email}</p>
                <p className="text-xs text-gray-500 mt-1">
                  {seed.status === 'active' ? (
                    <span className="text-green-600 flex items-center gap-1">
                      <CheckCircle className="w-3 h-3" /> Conectado
                    </span>
                  ) : (
                    <span className="text-red-600 flex items-center gap-1">
                      <XCircle className="w-3 h-3" /> {seed.error_message || 'Erro'}
                    </span>
                  )}
                </p>
                {/* Seed Statistics */}
                <div className="mt-3 pt-3 border-t border-gray-100">
                  <div className="grid grid-cols-2 gap-2 text-xs">
                    <div className="flex items-center gap-1">
                      <Send className="w-3 h-3 text-cyan-500" />
                      <span className="text-gray-600">Enviados:</span>
                      <span className="font-medium text-cyan-600">{seed.total_sent || 0}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <Mail className="w-3 h-3 text-blue-500" />
                      <span className="text-gray-600">Recebidos:</span>
                      <span className="font-medium text-gray-900">{seed.total_received || 0}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <Inbox className="w-3 h-3 text-green-500" />
                      <span className="text-gray-600">Entrada:</span>
                      <span className="font-medium text-green-600">{seed.total_inbox || 0}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <AlertCircle className="w-3 h-3 text-red-500" />
                      <span className="text-gray-600">Spam:</span>
                      <span className="font-medium text-red-600">{seed.total_spam || 0}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <TrendingUp className="w-3 h-3 text-orange-500" />
                      <span className="text-gray-600">Movidos:</span>
                      <span className="font-medium text-orange-600">{seed.total_moved || 0}</span>
                    </div>
                    <div className="flex items-center gap-1 col-span-2">
                      <MessageSquare className="w-3 h-3 text-purple-500" />
                      <span className="text-gray-600">Respondidos:</span>
                      <span className="font-medium text-purple-600">{seed.total_replied || 0}</span>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
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
                  item.status === 'replied' ? 'bg-purple-100' :
                  item.status === 'opened' ? 'bg-green-100' : 'bg-blue-100'
                }`}>
                  {item.status === 'replied' ? (
                    <MessageSquare className="w-4 h-4 text-purple-600" />
                  ) : item.status === 'opened' ? (
                    <CheckCircle className="w-4 h-4 text-green-600" />
                  ) : (
                    <Mail className="w-4 h-4 text-blue-600" />
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-gray-900 truncate">{item.subject}</p>
                  <p className="text-xs text-gray-500">{item.seed_email} via {item.smtp_name}</p>
                </div>
                <div className="text-xs text-gray-400">
                  {new Date(item.sent_at).toLocaleString('pt-BR')}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

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

      {editingWarmup && (
        <EditWarmupModal
          warmupSMTP={editingWarmup}
          onClose={() => setEditingWarmup(null)}
          onSave={() => { setEditingWarmup(null); fetchAll() }}
        />
      )}
    </div>
  )
}

export default Warmup
