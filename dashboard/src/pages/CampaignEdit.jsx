import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Save, Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function CampaignEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [lists, setLists] = useState([])

  const [form, setForm] = useState({
    name: '',
    subject: '',
    html_content: '',
    text_content: '',
    list_id: '',
    send_rate: 0
  })

  useEffect(() => {
    fetchLists()
    if (id) {
      fetchCampaign()
    }
  }, [id])

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

  const handleSubmit = async (e) => {
    e.preventDefault()
    setSaving(true)

    try {
      if (id) {
        await api.put(`/campaigns/${id}`, form)
        toast.success('Campanha atualizada!')
      } else {
        await api.post('/campaigns', form)
        toast.success('Campanha criada!')
      }
      navigate('/campaigns')
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
              <p className="text-xs text-gray-500 mt-1">
                O remetente será usado automaticamente do SMTP que estiver enviando
              </p>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Conteúdo do Email</h2>

          <div className="space-y-4">
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
