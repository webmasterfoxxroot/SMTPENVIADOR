import { useState, useEffect } from 'react'
import { Plus, Trash2, Upload, Search, Ban, Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function AddModal({ onClose, onSave }) {
  const [email, setEmail] = useState('')
  const [reason, setReason] = useState('manual')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      await api.post('/blacklist', { email, reason })
      toast.success('Email adicionado à blacklist')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao adicionar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4">Adicionar à Blacklist</h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="label">Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="input"
              placeholder="email@exemplo.com"
              required
            />
          </div>

          <div>
            <label className="label">Motivo</label>
            <select
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="input"
            >
              <option value="manual">Manual</option>
              <option value="bounce">Bounce</option>
              <option value="complaint">Reclamação</option>
              <option value="unsubscribe">Descadastro</option>
            </select>
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">
              Cancelar
            </button>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Adicionar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ImportModal({ onClose, onSuccess }) {
  const [file, setFile] = useState(null)
  const [reason, setReason] = useState('import')
  const [loading, setLoading] = useState(false)

  const handleUpload = async (e) => {
    e.preventDefault()
    if (!file) return

    setLoading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('reason', reason)

    try {
      const response = await api.post('/blacklist/import', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      toast.success(`Importados: ${response.data.imported} emails`)
      onSuccess()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro no upload')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4">Importar Blacklist</h2>

        <form onSubmit={handleUpload} className="space-y-4">
          <div>
            <label className="label">Arquivo (um email por linha)</label>
            <input
              type="file"
              accept=".csv,.txt"
              onChange={(e) => setFile(e.target.files[0])}
              className="input"
              required
            />
          </div>

          <div>
            <label className="label">Motivo</label>
            <select
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="input"
            >
              <option value="import">Importação</option>
              <option value="bounce">Bounces</option>
              <option value="complaint">Reclamações</option>
            </select>
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">
              Cancelar
            </button>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Importar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function Blacklist() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [addModal, setAddModal] = useState(false)
  const [importModal, setImportModal] = useState(false)

  useEffect(() => {
    fetchBlacklist()
  }, [page, search])

  const fetchBlacklist = async () => {
    try {
      const response = await api.get('/blacklist', {
        params: { page, limit: 50, search }
      })
      setItems(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Erro ao carregar blacklist')
    } finally {
      setLoading(false)
    }
  }

  const removeFromBlacklist = async (id) => {
    if (!confirm('Remover este email da blacklist?')) return

    try {
      await api.delete(`/blacklist/${id}`)
      toast.success('Email removido da blacklist')
      fetchBlacklist()
    } catch (error) {
      toast.error('Erro ao remover')
    }
  }

  const getReasonLabel = (reason) => {
    const labels = {
      manual: 'Manual',
      bounce: 'Bounce',
      complaint: 'Reclamação',
      unsubscribe: 'Descadastro',
      import: 'Importação'
    }
    return labels[reason] || reason
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Blacklist</h1>
        <div className="flex gap-3">
          <button
            onClick={() => setImportModal(true)}
            className="btn btn-secondary flex items-center gap-2"
          >
            <Upload className="w-4 h-4" />
            Importar
          </button>
          <button
            onClick={() => setAddModal(true)}
            className="btn btn-primary flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Adicionar
          </button>
        </div>
      </div>

      <div className="card">
        <div className="flex items-center gap-4 mb-4">
          <div className="relative flex-1 max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
            <input
              type="text"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
              className="input pl-10"
              placeholder="Buscar email..."
            />
          </div>
          <span className="text-sm text-gray-500">
            {total.toLocaleString()} emails na blacklist
          </span>
        </div>

        {loading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : items.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <Ban className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>{search ? 'Nenhum email encontrado' : 'Blacklist vazia'}</p>
          </div>
        ) : (
          <>
            <table className="table">
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Motivo</th>
                  <th>Data</th>
                  <th>Ações</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <td className="font-mono">{item.email}</td>
                    <td>
                      <span className="badge badge-gray">
                        {getReasonLabel(item.reason)}
                      </span>
                    </td>
                    <td className="text-gray-500">
                      {new Date(item.created_at).toLocaleDateString('pt-BR')}
                    </td>
                    <td>
                      <button
                        onClick={() => removeFromBlacklist(item.id)}
                        className="p-2 text-red-600 hover:bg-red-50 rounded"
                        title="Remover"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            {total > 50 && (
              <div className="flex items-center justify-between mt-4 pt-4 border-t">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="btn btn-secondary"
                >
                  Anterior
                </button>
                <span className="text-sm text-gray-500">
                  Página {page} de {Math.ceil(total / 50)}
                </span>
                <button
                  onClick={() => setPage(p => p + 1)}
                  disabled={page >= Math.ceil(total / 50)}
                  className="btn btn-secondary"
                >
                  Próxima
                </button>
              </div>
            )}
          </>
        )}
      </div>

      {addModal && (
        <AddModal
          onClose={() => setAddModal(false)}
          onSave={() => {
            setAddModal(false)
            fetchBlacklist()
          }}
        />
      )}

      {importModal && (
        <ImportModal
          onClose={() => setImportModal(false)}
          onSuccess={() => {
            setImportModal(false)
            fetchBlacklist()
          }}
        />
      )}
    </div>
  )
}

export default Blacklist
