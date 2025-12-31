import { useState, useEffect } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import {
  Plus,
  Play,
  Pause,
  StopCircle,
  Trash2,
  Eye,
  Send,
  Loader2,
  Copy,
  RefreshCw,
  AlertCircle,
  EyeOff,
  Download,
  MoreVertical,
  List,
  X,
  Clock,
  Calendar
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function Campaigns() {
  const [campaigns, setCampaigns] = useState([])
  const [loading, setLoading] = useState(true)
  const [actionMenu, setActionMenu] = useState(null)
  const [serverTimeOffset, setServerTimeOffset] = useState(0) // Offset between server and client time
  const [scheduleModal, setScheduleModal] = useState({ open: false, campaignId: null, campaignName: '' })
  const [scheduleDate, setScheduleDate] = useState('')
  const [scheduleTime, setScheduleTime] = useState('')
  const navigate = useNavigate()
  const location = useLocation()

  // Force re-render every second to update countdowns
  const [, setTick] = useState(0)

  useEffect(() => {
    fetchCampaigns()
    const interval = setInterval(fetchCampaigns, 5000) // Refresh every 5 seconds
    return () => clearInterval(interval)
  }, [])

  // Tick every second to update countdown display
  useEffect(() => {
    const interval = setInterval(() => {
      setTick(t => t + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [])

  // Close menu when clicking outside
  useEffect(() => {
    const handleClick = () => setActionMenu(null)
    document.addEventListener('click', handleClick)
    return () => document.removeEventListener('click', handleClick)
  }, [])

  const fetchCampaigns = async () => {
    try {
      const response = await api.get('/campaigns')
      const campaignsData = response.data.data || []
      setCampaigns(campaignsData)

      // Debug: log auto_start_at values
      campaignsData.forEach(c => {
        if (c.auto_start_at) {
          console.log(`[Countdown] Campaign ${c.name}: auto_start_at=${c.auto_start_at}, status=${c.status}`)
        }
      })

      // Calculate time offset between server and client
      // If server_time is 100 and client is at 118, offset = 100 - 118 = -18
      // This means client is 18 seconds ahead of server
      if (response.data.server_time) {
        const clientNow = Math.floor(Date.now() / 1000)
        const offset = response.data.server_time - clientNow
        console.log(`[Countdown] Server time: ${response.data.server_time}, Client: ${clientNow}, Offset: ${offset}`)
        setServerTimeOffset(offset)
      }
    } catch (error) {
      toast.error('Erro ao carregar campanhas')
    } finally {
      setLoading(false)
    }
  }

  const startCampaign = async (id) => {
    try {
      const response = await api.post(`/campaigns/${id}/start`)
      toast.success(`Campanha iniciada! ${response.data.emails_queued} emails na fila`)
      fetchCampaigns()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar')
    }
  }

  const cancelAutoStart = async (campaignId) => {
    try {
      await api.post(`/campaigns/${campaignId}/cancel-auto-start`)
      toast.info('Auto-start cancelado. A campanha permanece como rascunho.')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao cancelar auto-start')
    }
  }

  const forceStartNow = async (campaignId) => {
    try {
      const response = await api.post(`/campaigns/${campaignId}/start`)
      toast.success(`Campanha iniciada! ${response.data.emails_queued} emails na fila`)
      fetchCampaigns()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar')
    }
  }

  const pauseCampaign = async (id) => {
    try {
      await api.post(`/campaigns/${id}/pause`)
      toast.success('Campanha pausada')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao pausar')
    }
  }

  const resumeCampaign = async (id) => {
    try {
      await api.post(`/campaigns/${id}/resume`)
      toast.success('Campanha retomada')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao retomar')
    }
  }

  const cancelCampaign = async (id) => {
    if (!confirm('Tem certeza que deseja cancelar esta campanha?')) return

    try {
      await api.post(`/campaigns/${id}/cancel`)
      toast.success('Campanha cancelada')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao cancelar')
    }
  }

  const deleteCampaign = async (id) => {
    if (!confirm('Tem certeza que deseja excluir esta campanha?')) return

    try {
      await api.delete(`/campaigns/${id}`)
      toast.success('Campanha excluída')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao excluir')
    }
  }

  const cloneCampaign = async (id) => {
    try {
      await api.post(`/campaigns/${id}/clone`)
      toast.success('Campanha clonada! Auto-start em 60 segundos.')
      fetchCampaigns()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao clonar campanha')
    }
  }

  const openScheduleModal = (campaign) => {
    // Set default date/time to tomorrow at 9:00 AM
    const tomorrow = new Date()
    tomorrow.setDate(tomorrow.getDate() + 1)
    tomorrow.setHours(9, 0, 0, 0)

    setScheduleDate(tomorrow.toISOString().split('T')[0])
    setScheduleTime('09:00')
    setScheduleModal({ open: true, campaignId: campaign.id, campaignName: campaign.name })
    setActionMenu(null)
  }

  const scheduleCampaign = async () => {
    if (!scheduleDate || !scheduleTime) {
      toast.error('Selecione data e hora')
      return
    }

    const scheduledAt = new Date(`${scheduleDate}T${scheduleTime}:00`)

    if (scheduledAt <= new Date()) {
      toast.error('A data deve ser no futuro')
      return
    }

    try {
      await api.post(`/campaigns/${scheduleModal.campaignId}/schedule`, {
        scheduled_at: scheduledAt.toISOString()
      })
      toast.success(`Campanha agendada para ${scheduledAt.toLocaleString('pt-BR')}`)
      setScheduleModal({ open: false, campaignId: null, campaignName: '' })
      fetchCampaigns()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao agendar campanha')
    }
  }

  const cancelSchedule = async (id) => {
    try {
      await api.post(`/campaigns/${id}/cancel-schedule`)
      toast.success('Agendamento cancelado')
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao cancelar agendamento')
    }
  }

  const resendCampaign = async (id) => {
    if (!confirm('Reenviar para TODOS os emails da lista?')) return

    try {
      const response = await api.post(`/campaigns/${id}/resend`)
      toast.success(`Reenviando! ${response.data.emails_queued} emails na fila`)
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao reenviar')
    }
  }

  const resendToFailed = async (id) => {
    try {
      const response = await api.post(`/campaigns/${id}/resend-failed`)
      if (response.data.emails_queued === 0) {
        toast.info('Nenhum email falho para reenviar')
      } else {
        toast.success(`Reenviando para ${response.data.emails_queued} emails que falharam`)
      }
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao reenviar para falhos')
    }
  }

  const resendToNonOpeners = async (id) => {
    try {
      const response = await api.post(`/campaigns/${id}/resend-non-openers`)
      if (response.data.emails_queued === 0) {
        toast.info('Todos os emails foram abertos!')
      } else {
        toast.success(`Reenviando para ${response.data.emails_queued} que não abriram`)
      }
      fetchCampaigns()
    } catch (error) {
      toast.error('Erro ao reenviar para não abertos')
    }
  }

  const exportCSV = async (id, name) => {
    try {
      const response = await api.get(`/campaigns/${id}/export`, { responseType: 'blob' })
      const url = window.URL.createObjectURL(new Blob([response.data]))
      const link = document.createElement('a')
      link.href = url
      link.setAttribute('download', `${name}.csv`)
      document.body.appendChild(link)
      link.click()
      link.remove()
      toast.success('CSV exportado!')
    } catch (error) {
      toast.error('Erro ao exportar CSV')
    }
  }

  // Calculate remaining seconds until auto_start_at (Unix timestamp in seconds)
  // Uses serverTimeOffset to sync with server clock
  const getCountdownSeconds = (autoStartAtUnix) => {
    if (!autoStartAtUnix) return null
    // Apply server time offset to client time to get server-relative time
    // If offset is -18 (client is 18s ahead), we add -18 to client time
    const clientNow = Math.floor(Date.now() / 1000)
    const serverNow = clientNow + serverTimeOffset // Adjusted to server time
    const remaining = autoStartAtUnix - serverNow
    return remaining > 0 ? remaining : 0
  }

  const getStatusBadge = (campaign) => {
    // Check if campaign has auto_start_at
    const countdown = getCountdownSeconds(campaign.auto_start_at)

    // Debug log
    console.log(`[StatusBadge] Campaign: ${campaign.name}, auto_start_at: ${campaign.auto_start_at}, countdown: ${countdown}, status: ${campaign.status}`)

    if (countdown !== null && countdown > 0 && campaign.status === 'draft') {
      return (
        <div className="flex flex-col items-start gap-1">
          <div className="flex items-center gap-2 text-blue-600">
            <Clock className="w-4 h-4 animate-pulse" />
            <span className="font-bold text-lg">{countdown}s</span>
          </div>
          <div className="flex gap-1">
            <button
              onClick={(e) => {
                e.stopPropagation()
                forceStartNow(campaign.id)
              }}
              className="text-xs px-2 py-1 bg-green-500 text-white rounded hover:bg-green-600"
            >
              Iniciar
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation()
                cancelAutoStart(campaign.id)
              }}
              className="text-xs px-2 py-1 bg-gray-500 text-white rounded hover:bg-gray-600"
            >
              Cancelar
            </button>
          </div>
        </div>
      )
    }

    // Show scheduled date/time for scheduled campaigns
    if (campaign.status === 'scheduled' && campaign.scheduled_at) {
      return (
        <div className="flex flex-col items-start gap-1">
          <span className="badge badge-info flex items-center gap-1">
            <Calendar className="w-3 h-3" />
            Agendada
          </span>
          <span className="text-xs text-gray-500">
            {formatDate(campaign.scheduled_at)}
          </span>
        </div>
      )
    }

    const badges = {
      draft: 'badge-gray',
      scheduled: 'badge-info',
      running: 'badge-success',
      paused: 'badge-warning',
      completed: 'badge-info',
      cancelled: 'badge-danger'
    }
    const labels = {
      draft: 'Rascunho',
      scheduled: 'Agendada',
      running: 'Enviando',
      paused: 'Pausada',
      completed: 'Concluída',
      cancelled: 'Cancelada'
    }
    return (
      <span className={`badge ${badges[campaign.status] || 'badge-gray'}`}>
        {labels[campaign.status] || campaign.status}
      </span>
    )
  }

  const getProgress = (campaign) => {
    if (campaign.total_emails === 0) return 0
    return Math.round((campaign.sent_count / campaign.total_emails) * 100)
  }

  // Format date for display
  const formatDate = (dateStr) => {
    if (!dateStr) return '-'
    const date = new Date(dateStr)
    return date.toLocaleDateString('pt-BR', {
      day: '2-digit',
      month: '2-digit',
      year: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const toggleMenu = (e, id) => {
    e.stopPropagation()
    setActionMenu(actionMenu === id ? null : id)
  }

  // Check if campaign has active countdown
  const hasActiveCountdown = (campaign) => {
    const countdown = getCountdownSeconds(campaign.auto_start_at)
    return countdown !== null && countdown > 0 && campaign.status === 'draft'
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Campanhas</h1>
        <Link to="/campaigns/new" className="btn btn-primary flex items-center gap-2">
          <Plus className="w-4 h-4" />
          Nova Campanha
        </Link>
      </div>

      <div className="card">
        {loading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : campaigns.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <Send className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>Nenhuma campanha criada</p>
          </div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Campanha</th>
                <th>Lista</th>
                <th>Status</th>
                <th>Progresso</th>
                <th>Aberturas</th>
                <th>Cliques</th>
                <th>Criada</th>
                <th>Iniciada</th>
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((campaign) => (
                <tr
                  key={campaign.id}
                  onClick={() => navigate(`/campaigns/${campaign.id}/details`)}
                  className="cursor-pointer hover:bg-gray-50"
                >
                  <td>
                    <div>
                      <div className="font-medium">{campaign.name}</div>
                      <div className="text-sm text-gray-500">{campaign.subject}</div>
                    </div>
                  </td>
                  <td className="text-gray-600">{campaign.list_name || '-'}</td>
                  <td>{getStatusBadge(campaign)}</td>
                  <td>
                    <div className="w-32">
                      <div className="flex justify-between text-sm mb-1">
                        <span>{campaign.sent_count?.toLocaleString()}</span>
                        <span className="text-gray-500">{campaign.total_emails?.toLocaleString()}</span>
                      </div>
                      <div className="h-2 bg-gray-200 rounded-full overflow-hidden">
                        <div
                          className="h-full bg-blue-500 transition-all"
                          style={{ width: `${getProgress(campaign)}%` }}
                        />
                      </div>
                      {campaign.failed_count > 0 && (
                        <div className="text-xs text-red-500 mt-1">
                          {campaign.failed_count} falhos
                        </div>
                      )}
                    </div>
                  </td>
                  <td className="text-gray-600">
                    {campaign.open_count?.toLocaleString()}
                    {campaign.sent_count > 0 && (
                      <span className="text-xs text-gray-400 ml-1">
                        ({Math.round((campaign.open_count / campaign.sent_count) * 100)}%)
                      </span>
                    )}
                  </td>
                  <td className="text-gray-600">
                    {campaign.click_count?.toLocaleString()}
                    {campaign.open_count > 0 && (
                      <span className="text-xs text-gray-400 ml-1">
                        ({Math.round((campaign.click_count / campaign.open_count) * 100)}%)
                      </span>
                    )}
                  </td>
                  <td className="text-gray-500 text-sm whitespace-nowrap">
                    {formatDate(campaign.created_at)}
                  </td>
                  <td className="text-gray-500 text-sm whitespace-nowrap">
                    {formatDate(campaign.started_at)}
                  </td>
                  <td>
                    <div className="flex items-center gap-1">
                      {campaign.status === 'draft' && !hasActiveCountdown(campaign) && (
                        <button
                          onClick={(e) => { e.stopPropagation(); startCampaign(campaign.id) }}
                          className="p-2 text-green-600 hover:bg-green-50 rounded"
                          title="Iniciar"
                        >
                          <Play className="w-4 h-4" />
                        </button>
                      )}
                      {campaign.status === 'running' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); pauseCampaign(campaign.id) }}
                          className="p-2 text-yellow-600 hover:bg-yellow-50 rounded"
                          title="Pausar"
                        >
                          <Pause className="w-4 h-4" />
                        </button>
                      )}
                      {campaign.status === 'paused' && (
                        <>
                          <button
                            onClick={(e) => { e.stopPropagation(); resumeCampaign(campaign.id) }}
                            className="p-2 text-green-600 hover:bg-green-50 rounded"
                            title="Retomar"
                          >
                            <Play className="w-4 h-4" />
                          </button>
                          <button
                            onClick={(e) => { e.stopPropagation(); cancelCampaign(campaign.id) }}
                            className="p-2 text-red-600 hover:bg-red-50 rounded"
                            title="Cancelar"
                          >
                            <StopCircle className="w-4 h-4" />
                          </button>
                        </>
                      )}
                      <Link
                        to={`/campaigns/${campaign.id}`}
                        onClick={(e) => e.stopPropagation()}
                        className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                        title="Editar"
                      >
                        <Eye className="w-4 h-4" />
                      </Link>

                      {/* Dropdown menu for more actions */}
                      <div className="relative">
                        <button
                          onClick={(e) => toggleMenu(e, campaign.id)}
                          className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                          title="Mais opções"
                        >
                          <MoreVertical className="w-4 h-4" />
                        </button>

                        {actionMenu === campaign.id && (
                          <div className="absolute right-0 top-full mt-1 bg-white border rounded-lg shadow-lg py-1 z-10 min-w-[180px]" onClick={(e) => e.stopPropagation()}>
                            <button
                              onClick={() => cloneCampaign(campaign.id)}
                              className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                            >
                              <Copy className="w-4 h-4" />
                              Clonar
                            </button>

                            {campaign.status === 'draft' && !hasActiveCountdown(campaign) && (
                              <button
                                onClick={() => openScheduleModal(campaign)}
                                className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2 text-blue-600"
                              >
                                <Calendar className="w-4 h-4" />
                                Agendar
                              </button>
                            )}

                            {campaign.status === 'scheduled' && (
                              <button
                                onClick={() => cancelSchedule(campaign.id)}
                                className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2 text-orange-600"
                              >
                                <X className="w-4 h-4" />
                                Cancelar Agendamento
                              </button>
                            )}

                            {(campaign.status === 'completed' || campaign.status === 'cancelled') && (
                              <>
                                <button
                                  onClick={() => resendCampaign(campaign.id)}
                                  className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                                >
                                  <RefreshCw className="w-4 h-4" />
                                  Reenviar Todos
                                </button>

                                {campaign.failed_count > 0 && (
                                  <button
                                    onClick={() => resendToFailed(campaign.id)}
                                    className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2 text-red-600"
                                  >
                                    <AlertCircle className="w-4 h-4" />
                                    Reenviar Falhos ({campaign.failed_count})
                                  </button>
                                )}

                                <button
                                  onClick={() => resendToNonOpeners(campaign.id)}
                                  className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                                >
                                  <EyeOff className="w-4 h-4" />
                                  Reenviar Não Abertos
                                </button>
                              </>
                            )}

                            <hr className="my-1" />

                            <button
                              onClick={() => exportCSV(campaign.id, campaign.name)}
                              className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                            >
                              <Download className="w-4 h-4" />
                              Exportar CSV
                            </button>

                            <hr className="my-1" />

                            {(campaign.status === 'draft' || campaign.status === 'completed' || campaign.status === 'cancelled') && (
                              <button
                                onClick={() => deleteCampaign(campaign.id)}
                                className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2 text-red-600"
                              >
                                <Trash2 className="w-4 h-4" />
                                Excluir
                              </button>
                            )}
                          </div>
                        )}
                      </div>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Schedule Modal */}
      {scheduleModal.open && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-md">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">Agendar Campanha</h3>
              <button
                onClick={() => setScheduleModal({ open: false, campaignId: null, campaignName: '' })}
                className="text-gray-500 hover:text-gray-700"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <p className="text-gray-600 mb-4">
              Agendar: <strong>{scheduleModal.campaignName}</strong>
            </p>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Data
                </label>
                <input
                  type="date"
                  value={scheduleDate}
                  onChange={(e) => setScheduleDate(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Horário
                </label>
                <input
                  type="time"
                  value={scheduleTime}
                  onChange={(e) => setScheduleTime(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                />
              </div>
            </div>

            <div className="flex gap-3 mt-6">
              <button
                onClick={() => setScheduleModal({ open: false, campaignId: null, campaignName: '' })}
                className="flex-1 px-4 py-2 border border-gray-300 rounded-lg hover:bg-gray-50"
              >
                Cancelar
              </button>
              <button
                onClick={scheduleCampaign}
                className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 flex items-center justify-center gap-2"
              >
                <Calendar className="w-4 h-4" />
                Agendar
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default Campaigns
