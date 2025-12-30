import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Loader2, CheckCircle, XCircle, Clock, Eye, MousePointer } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function CampaignDetails() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [campaign, setCampaign] = useState(null)
  const [emails, setEmails] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const limit = 50

  useEffect(() => {
    fetchCampaign()
  }, [id])

  useEffect(() => {
    fetchDetails()
  }, [id, filter, page])

  const fetchCampaign = async () => {
    try {
      const response = await api.get(`/campaigns/${id}`)
      setCampaign(response.data)
    } catch (error) {
      toast.error('Erro ao carregar campanha')
      navigate('/campaigns')
    }
  }

  const fetchDetails = async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ page, limit })
      if (filter) params.append('status', filter)

      const response = await api.get(`/campaigns/${id}/details?${params}`)
      setEmails(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Erro ao carregar detalhes')
    } finally {
      setLoading(false)
    }
  }

  const getStatusIcon = (status) => {
    switch (status) {
      case 'sent':
        return <CheckCircle className="w-4 h-4 text-green-500" />
      case 'failed':
        return <XCircle className="w-4 h-4 text-red-500" />
      case 'queued':
      case 'sending':
        return <Clock className="w-4 h-4 text-yellow-500" />
      default:
        return <Clock className="w-4 h-4 text-gray-400" />
    }
  }

  const getStatusLabel = (status) => {
    const labels = {
      queued: 'Na fila',
      sending: 'Enviando',
      sent: 'Enviado',
      failed: 'Falhou',
      bounced: 'Bounce'
    }
    return labels[status] || status
  }

  const totalPages = Math.ceil(total / limit)

  return (
    <div>
      <div className="flex items-center gap-4 mb-6">
        <button
          onClick={() => navigate('/campaigns')}
          className="p-2 hover:bg-gray-100 rounded"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div>
          <h1 className="text-2xl font-bold">Detalhes da Campanha</h1>
          {campaign && (
            <p className="text-gray-500">{campaign.name}</p>
          )}
        </div>
      </div>

      {/* Stats cards */}
      {campaign && (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-6">
          <div className="card text-center">
            <div className="text-2xl font-bold text-blue-600">{campaign.total_emails?.toLocaleString()}</div>
            <div className="text-sm text-gray-500">Total</div>
          </div>
          <div className="card text-center">
            <div className="text-2xl font-bold text-green-600">{campaign.sent_count?.toLocaleString()}</div>
            <div className="text-sm text-gray-500">Enviados</div>
          </div>
          <div className="card text-center">
            <div className="text-2xl font-bold text-red-600">{campaign.failed_count?.toLocaleString()}</div>
            <div className="text-sm text-gray-500">Falhos</div>
          </div>
          <div className="card text-center">
            <div className="text-2xl font-bold text-purple-600">{campaign.open_count?.toLocaleString()}</div>
            <div className="text-sm text-gray-500">Aberturas</div>
          </div>
          <div className="card text-center">
            <div className="text-2xl font-bold text-orange-600">{campaign.click_count?.toLocaleString()}</div>
            <div className="text-sm text-gray-500">Cliques</div>
          </div>
        </div>
      )}

      {/* Filter buttons */}
      <div className="flex gap-2 mb-4">
        <button
          onClick={() => { setFilter(''); setPage(1) }}
          className={`px-4 py-2 rounded ${filter === '' ? 'bg-blue-500 text-white' : 'bg-gray-100 hover:bg-gray-200'}`}
        >
          Todos
        </button>
        <button
          onClick={() => { setFilter('sent'); setPage(1) }}
          className={`px-4 py-2 rounded ${filter === 'sent' ? 'bg-green-500 text-white' : 'bg-gray-100 hover:bg-gray-200'}`}
        >
          Enviados
        </button>
        <button
          onClick={() => { setFilter('failed'); setPage(1) }}
          className={`px-4 py-2 rounded ${filter === 'failed' ? 'bg-red-500 text-white' : 'bg-gray-100 hover:bg-gray-200'}`}
        >
          Falhos
        </button>
        <button
          onClick={() => { setFilter('queued'); setPage(1) }}
          className={`px-4 py-2 rounded ${filter === 'queued' ? 'bg-yellow-500 text-white' : 'bg-gray-100 hover:bg-gray-200'}`}
        >
          Na fila
        </button>
      </div>

      {/* Emails table */}
      <div className="card">
        {loading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : emails.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            Nenhum email encontrado
          </div>
        ) : (
          <>
            <table className="table">
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Nome</th>
                  <th>Status</th>
                  <th>Enviado</th>
                  <th>Aberto</th>
                  <th>Clicou</th>
                  <th>Erro</th>
                </tr>
              </thead>
              <tbody>
                {emails.map((email, index) => (
                  <tr key={index}>
                    <td className="font-mono text-sm">{email.email}</td>
                    <td className="text-gray-600">{email.name || '-'}</td>
                    <td>
                      <div className="flex items-center gap-2">
                        {getStatusIcon(email.status)}
                        <span>{getStatusLabel(email.status)}</span>
                      </div>
                    </td>
                    <td className="text-sm text-gray-500">
                      {email.sent_at ? new Date(email.sent_at).toLocaleString('pt-BR') : '-'}
                    </td>
                    <td>
                      {email.opened_at ? (
                        <div className="flex items-center gap-1 text-purple-600">
                          <Eye className="w-4 h-4" />
                          <span className="text-xs">{new Date(email.opened_at).toLocaleString('pt-BR')}</span>
                        </div>
                      ) : '-'}
                    </td>
                    <td>
                      {email.clicked_at ? (
                        <div className="flex items-center gap-1 text-orange-600">
                          <MousePointer className="w-4 h-4" />
                          <span className="text-xs">{new Date(email.clicked_at).toLocaleString('pt-BR')}</span>
                        </div>
                      ) : '-'}
                    </td>
                    <td className="text-sm text-red-500 max-w-xs truncate" title={email.error}>
                      {email.error || '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between mt-4 pt-4 border-t">
                <div className="text-sm text-gray-500">
                  Mostrando {((page - 1) * limit) + 1} - {Math.min(page * limit, total)} de {total}
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => setPage(p => Math.max(1, p - 1))}
                    disabled={page === 1}
                    className="px-3 py-1 rounded bg-gray-100 hover:bg-gray-200 disabled:opacity-50"
                  >
                    Anterior
                  </button>
                  <span className="px-3 py-1">
                    {page} / {totalPages}
                  </span>
                  <button
                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                    disabled={page === totalPages}
                    className="px-3 py-1 rounded bg-gray-100 hover:bg-gray-200 disabled:opacity-50"
                  >
                    Próxima
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

export default CampaignDetails
