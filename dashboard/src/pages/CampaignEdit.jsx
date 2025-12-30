import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Save, Loader2, Clock, Play, X } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function CampaignEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [lists, setLists] = useState([])

  // Countdown modal state
  const [countdown, setCountdown] = useState(null)
  const [countdownCampaignId, setCountdownCampaignId] = useState(null)
  const [countdownInfo, setCountdownInfo] = useState(null)

  const [form, setForm] = useState({
    name: '',
    from_name: '',
    subject: '',
    html_content: '',
    text_content: '',
    list_id: '',
    send_rate: 0,
    track_opens: true,
    track_clicks: true
  })

  useEffect(() => {
    fetchLists()
    if (id) {
      fetchCampaign()
    }
  }, [id])

  // Countdown timer effect
  useEffect(() => {
    if (countdown === null || countdown < 0) return

    if (countdown === 0) {
      // Timer finished, start the campaign
      actuallyStartCampaign(countdownCampaignId)
      return
    }

    const timer = setTimeout(() => {
      setCountdown(countdown - 1)
    }, 1000)

    return () => clearTimeout(timer)
  }, [countdown, countdownCampaignId])

  const fetchLists = async () => {
    try {
      const response = await api.get('/lists')
      setLists(response.data.data || [])
    } catch (error) {
      console.error('Failed to fetch lists:', error)
    }
  }

  const fetchCampaign = async () => {
    setLoading(true)
    try {
      const response = await api.get(`/campaigns/${id}`)
      setForm(response.data)
    } catch (error) {
      toast.error('Erro ao carregar campanha')
      navigate('/campaigns')
    } finally {
      setLoading(false)
    }
  }

  const actuallyStartCampaign = async (campaignId) => {
    try {
      const response = await api.post(`/campaigns/${campaignId}/start`)
      toast.success(`Campanha iniciada! ${response.data.emails_queued} emails na fila`)
      setCountdown(null)
      setCountdownCampaignId(null)
      setCountdownInfo(null)
      navigate('/campaigns')
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar')
      setCountdown(null)
      setCountdownCampaignId(null)
      setCountdownInfo(null)
    }
  }

  const cancelCountdown = () => {
    setCountdown(null)
    setCountdownCampaignId(null)
    setCountdownInfo(null)
    toast.info('Início cancelado. A campanha permanece como rascunho.')
    navigate('/campaigns')
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setSaving(true)

    try {
      if (id) {
        await api.put(`/campaigns/${id}`, form)
        toast.success('Campanha atualizada!')
        navigate('/campaigns')
      } else {
        const response = await api.post('/campaigns', form)
        toast.success('Campanha criada!')

        // If auto_start is true, show countdown
        if (response.data.auto_start) {
          setCountdownCampaignId(response.data.id)
          setCountdownInfo({
            total_emails: response.data.total_emails,
            smtp_count: response.data.smtp_count
          })
          setCountdown(60) // 60 second countdown
        } else {
          navigate('/campaigns')
        }
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
      </div>
    )
  }

  return (
    <div>
      {/* Countdown Modal */}
      {countdown !== null && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-8 max-w-md w-full mx-4 text-center">
            <div className="flex justify-center mb-4">
              <div className="w-24 h-24 rounded-full bg-blue-100 flex items-center justify-center">
                <Clock className="w-12 h-12 text-blue-600" />
              </div>
            </div>
            <h2 className="text-2xl font-bold mb-2">Iniciando Campanha</h2>
            <p className="text-gray-600 mb-4">
              A campanha será iniciada em
            </p>
            <div className="text-6xl font-bold text-blue-600 mb-4">
              {countdown}s
            </div>
            {countdownInfo && (
              <div className="bg-gray-100 rounded-lg p-4 mb-6 text-sm">
                <div className="flex justify-between mb-2">
                  <span className="text-gray-600">Emails a enviar:</span>
                  <span className="font-semibold">{countdownInfo.total_emails?.toLocaleString()}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">SMTPs ativos:</span>
                  <span className="font-semibold">{countdownInfo.smtp_count}</span>
                </div>
              </div>
            )}
            <div className="flex gap-3 justify-center">
              <button
                onClick={() => {
                  setCountdown(0) // Force immediate start
                }}
                className="btn btn-primary flex items-center gap-2"
              >
                <Play className="w-4 h-4" />
                Iniciar Agora
              </button>
              <button
                onClick={cancelCountdown}
                className="btn btn-secondary flex items-center gap-2"
              >
                <X className="w-4 h-4" />
                Cancelar
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="flex items-center gap-4 mb-6">
        <button
          onClick={() => navigate('/campaigns')}
          className="p-2 hover:bg-gray-100 rounded"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <h1 className="text-2xl font-bold">
          {id ? 'Editar Campanha' : 'Nova Campanha'}
        </h1>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Informações Básicas</h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="label">Nome da Campanha</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input"
                placeholder="Black Friday 2024"
                required
              />
            </div>
            <div>
              <label className="label">Lista de Emails</label>
              <select
                value={form.list_id}
                onChange={(e) => setForm({ ...form, list_id: e.target.value })}
                className="input"
                required
              >
                <option value="">Selecione uma lista</option>
                {lists.map((list) => (
                  <option key={list.id} value={list.id}>
                    {list.name} ({list.valid_emails?.toLocaleString()} emails)
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="label">Taxa de Envio (emails/min, 0 = ilimitado)</label>
              <input
                type="number"
                value={form.send_rate}
                onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 0 })}
                className="input"
                min="0"
              />
            </div>
            <div className="flex flex-col gap-3 pt-6">
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  id="track_opens"
                  checked={form.track_opens}
                  onChange={(e) => setForm({ ...form, track_opens: e.target.checked })}
                  className="w-5 h-5 rounded border-gray-600 bg-gray-700 text-blue-500 focus:ring-blue-500"
                />
                <label htmlFor="track_opens" className="text-sm">
                  Rastrear aberturas
                </label>
              </div>
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  id="track_clicks"
                  checked={form.track_clicks}
                  onChange={(e) => setForm({ ...form, track_clicks: e.target.checked })}
                  className="w-5 h-5 rounded border-gray-600 bg-gray-700 text-blue-500 focus:ring-blue-500"
                />
                <label htmlFor="track_clicks" className="text-sm">
                  Rastrear cliques
                </label>
              </div>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Conteúdo do Email</h2>

          <div className="space-y-4">
            <div>
              <label className="label">Nome do Remetente</label>
              <input
                type="text"
                value={form.from_name}
                onChange={(e) => setForm({ ...form, from_name: e.target.value })}
                className="input"
                placeholder="Empresa XYZ"
                required
              />
              <p className="text-xs text-gray-500 mt-1">
                Nome que aparece no campo "De:" do email. O email será do SMTP.
              </p>
            </div>

            <div>
              <label className="label">Assunto</label>
              <input
                type="text"
                value={form.subject}
                onChange={(e) => setForm({ ...form, subject: e.target.value })}
                className="input"
                placeholder="Olá {{nome}}, confira nossa oferta!"
                required
              />
              <p className="text-xs text-gray-500 mt-1">
                Use variáveis: {'{{nome}}'}, {'{{email}}'}, {'{{custom1}}'}, etc.
              </p>
            </div>

            <div>
              <label className="label">Conteúdo HTML</label>
              <textarea
                value={form.html_content}
                onChange={(e) => setForm({ ...form, html_content: e.target.value })}
                className="input font-mono text-sm"
                rows={15}
                placeholder="<html><body>Olá {{nome}}!</body></html>"
                required
              />
            </div>

            <div>
              <label className="label">Conteúdo Texto (opcional)</label>
              <textarea
                value={form.text_content}
                onChange={(e) => setForm({ ...form, text_content: e.target.value })}
                className="input"
                rows={5}
                placeholder="Versão texto do email..."
              />
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-3">
          <button
            type="button"
            onClick={() => navigate('/campaigns')}
            className="btn btn-secondary"
          >
            Cancelar
          </button>
          <button type="submit" disabled={saving} className="btn btn-primary flex items-center gap-2">
            {saving ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <Save className="w-5 h-5" />
            )}
            Salvar Campanha
          </button>
        </div>
      </form>
    </div>
  )
}

export default CampaignEdit
