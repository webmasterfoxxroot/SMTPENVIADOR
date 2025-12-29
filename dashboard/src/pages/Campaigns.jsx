import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Plus,
  Play,
  Pause,
  StopCircle,
  Trash2,
  Eye,
  Send,
  Loader2
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function Campaigns() {
  const [campaigns, setCampaigns] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchCampaigns()
    const interval = setInterval(fetchCampaigns, 10000)
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

  const startCampaign = async (id) => {
    try {
      const response = await api.post(`/campaigns/${id}/start`)
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

  const getStatusBadge = (status) => {
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
      <span className={`badge ${badges[status] || 'badge-gray'}`}>
        {labels[status] || status}
      </span>
    )
  }

  const getProgress = (campaign) => {
    if (campaign.total_emails === 0) return 0
    return Math.round((campaign.sent_count / campaign.total_emails) * 100)
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
                  <td>{getStatusBadge(campaign.status)}</td>
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
                      {campaign.status === 'draft' && (
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
                      {(campaign.status === 'draft' || campaign.status === 'completed' || campaign.status === 'cancelled') && (
                        <button
                          onClick={() => deleteCampaign(campaign.id)}
                          className="p-2 text-red-600 hover:bg-red-50 rounded"
                          title="Excluir"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
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
