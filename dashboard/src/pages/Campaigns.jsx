import { useState, useEffect, useRef } from 'react'
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
  Clock
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function Campaigns() {
  const [campaigns, setCampaigns] = useState([])
  const [loading, setLoading] = useState(true)
  const [actionMenu, setActionMenu] = useState(null)
  const navigate = useNavigate()
  const location = useLocation()

  // Countdown state - tracks countdown per campaign ID
  const [countdowns, setCountdowns] = useState({})

  // Check if we came from creating a new campaign
  useEffect(() => {
    if (location.state?.newCampaignId && location.state?.autoStart) {
      setCountdowns(prev => ({
        ...prev,
        [location.state.newCampaignId]: 60
      }))
      // Clear the state to prevent re-triggering on page refresh
      window.history.replaceState({}, document.title)
    }
  }, [location.state])

  useEffect(() => {
    fetchCampaigns()
    const interval = setInterval(fetchCampaigns, 10000)
    return () => clearInterval(interval)
  }, [])

  // Close menu when clicking outside
  useEffect(() => {
    const handleClick = () => setActionMenu(null)
    document.addEventListener('click', handleClick)
    return () => document.removeEventListener('click', handleClick)
  }, [])

  // Countdown timer - runs every second
  useEffect(() => {
    const interval = setInterval(() => {
      setCountdowns(prev => {
        const updated = { ...prev }
        let hasChanges = false

        Object.keys(updated).forEach(campaignId => {
          if (updated[campaignId] > 0) {
            updated[campaignId] = updated[campaignId] - 1
            hasChanges = true

            // When countdown reaches 0, start the campaign
            if (updated[campaignId] === 0) {
              startCampaignAfterCountdown(campaignId)
              delete updated[campaignId]
            }
          }
        })

        return hasChanges ? updated : prev
      })
    }, 1000)

    return () => clearInterval(interval)
  }, [])

  const fetchCampaigns = async () => {
    try {
      const response = await api.get('/campaigns')
      setCampaigns(response.data.data || [])
    } catch (error) {
      toast.error('Erro ao carregar campanhas')
    } finally {
      setLoading(false)
    }
  }

  const startCampaignAfterCountdown = async (id) => {
    try {
      const response = await api.post(`/campaigns/${id}/start`)
      toast.success(`Campanha iniciada! ${response.data.emails_queued} emails na fila`)
      fetchCampaigns()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar')
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

  const cancelCountdown = (campaignId) => {
    setCountdowns(prev => {
      const updated = { ...prev }
      delete updated[campaignId]
      return updated
    })
    toast.info('Início cancelado. A campanha permanece como rascunho.')
  }

  const forceStartNow = (campaignId) => {
    setCountdowns(prev => {
      const updated = { ...prev }
      delete updated[campaignId]
      return updated
    })
    startCampaign(campaignId)
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
      const response = await api.post(`/campaigns/${id}/clone`)
      toast.success('Campanha clonada!')

      // Start countdown for the new campaign
      if (response.data.auto_start) {
        setCountdowns(prev => ({
          ...prev,
          [response.data.id]: 60 // 60 second countdown
        }))
        fetchCampaigns()
      } else {
        navigate(`/campaigns/${response.data.id}`)
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao clonar campanha')
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

  const getStatusBadge = (campaign) => {
    // Check if campaign has active countdown
    if (countdowns[campaign.id] !== undefined) {
      return (
        <div className="flex flex-col items-start gap-1">
          <div className="flex items-center gap-2 text-blue-600">
            <Clock className="w-4 h-4 animate-pulse" />
            <span className="font-bold text-lg">{countdowns[campaign.id]}s</span>
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
                cancelCountdown(campaign.id)
              }}
              className="text-xs px-2 py-1 bg-gray-500 text-white rounded hover:bg-gray-600"
            >
              Cancelar
            </button>
          </div>
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

  const toggleMenu = (e, id) => {
    e.stopPropagation()
    setActionMenu(actionMenu === id ? null : id)
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
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((campaign) => (
                <tr key={campaign.id}>
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
                  <td>
                    <div className="flex items-center gap-1">
                      {campaign.status === 'draft' && !countdowns[campaign.id] && (
                        <button
                          onClick={() => startCampaign(campaign.id)}
                          className="p-2 text-green-600 hover:bg-green-50 rounded"
                          title="Iniciar"
                        >
                          <Play className="w-4 h-4" />
                        </button>
                      )}
                      {campaign.status === 'running' && (
                        <button
                          onClick={() => pauseCampaign(campaign.id)}
                          className="p-2 text-yellow-600 hover:bg-yellow-50 rounded"
                          title="Pausar"
                        >
                          <Pause className="w-4 h-4" />
                        </button>
                      )}
                      {campaign.status === 'paused' && (
                        <>
                          <button
                            onClick={() => resumeCampaign(campaign.id)}
                            className="p-2 text-green-600 hover:bg-green-50 rounded"
                            title="Retomar"
                          >
                            <Play className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => cancelCampaign(campaign.id)}
                            className="p-2 text-red-600 hover:bg-red-50 rounded"
                            title="Cancelar"
                          >
                            <StopCircle className="w-4 h-4" />
                          </button>
                        </>
                      )}
                      <Link
                        to={`/campaigns/${campaign.id}`}
                        className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                        title="Ver/Editar"
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
                          <div className="absolute right-0 top-full mt-1 bg-white border rounded-lg shadow-lg py-1 z-10 min-w-[180px]">
                            <button
                              onClick={() => cloneCampaign(campaign.id)}
                              className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                            >
                              <Copy className="w-4 h-4" />
                              Clonar
                            </button>

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

                            <Link
                              to={`/campaigns/${campaign.id}/details`}
                              className="w-full px-4 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-2"
                            >
                              <List className="w-4 h-4" />
                              Ver Detalhes
                            </Link>

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
    </div>
  )
}

export default Campaigns
