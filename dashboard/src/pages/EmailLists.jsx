import { useState, useEffect } from 'react'
import { Plus, Edit, Trash2, Upload, Users, Loader2, Mail } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function UploadModal({ listId, onClose, onSuccess }) {
  const [file, setFile] = useState(null)
  const [loading, setLoading] = useState(false)
  const [hasHeader, setHasHeader] = useState(true)
  const [delimiter, setDelimiter] = useState(',')

  const handleUpload = async (e) => {
    e.preventDefault()
    if (!file) return

    setLoading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('has_header', hasHeader)
    formData.append('delimiter', delimiter)
    formData.append('email_column', '0')
    formData.append('name_column', '1')

    try {
      const response = await api.post(`/lists/${listId}/upload`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      toast.success(`Importados: ${response.data.valid} emails`)
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
        <h2 className="text-xl font-bold mb-4">Upload de Emails</h2>

        <form onSubmit={handleUpload} className="space-y-4">
          <div>
            <label className="label">Arquivo (CSV/TXT)</label>
            <input
              type="file"
              accept=".csv,.txt"
              onChange={(e) => setFile(e.target.files[0])}
              className="input"
              required
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Delimitador</label>
              <select
                value={delimiter}
                onChange={(e) => setDelimiter(e.target.value)}
                className="input"
              >
                <option value=",">Vírgula (,)</option>
                <option value=";">Ponto e vírgula (;)</option>
                <option value="\t">Tab</option>
              </select>
            </div>
            <div className="flex items-end">
              <label className="flex items-center gap-2 cursor-pointer pb-2">
                <input
                  type="checkbox"
                  checked={hasHeader}
                  onChange={(e) => setHasHeader(e.target.checked)}
                  className="w-4 h-4"
                />
                Tem cabeçalho
              </label>
            </div>
          </div>

          <p className="text-sm text-gray-500">
            Formato esperado: email,nome (primeira coluna é email)
          </p>

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

function ListModal({ list, onClose, onSave }) {
  const [form, setForm] = useState({
    name: '',
    description: '',
    ...list
  })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      if (list?.id) {
        await api.put(`/lists/${list.id}`, form)
        toast.success('Lista atualizada!')
      } else {
        await api.post('/lists', form)
        toast.success('Lista criada!')
      }
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4">
          {list?.id ? 'Editar Lista' : 'Nova Lista'}
        </h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="label">Nome</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="input"
              placeholder="Minha Lista"
              required
            />
          </div>

          <div>
            <label className="label">Descrição</label>
            <textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              className="input"
              rows={3}
              placeholder="Descrição opcional..."
            />
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">
              Cancelar
            </button>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Salvar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function EmailLists() {
  const [lists, setLists] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, list: null })
  const [uploadModal, setUploadModal] = useState({ open: false, listId: null })

  useEffect(() => {
    fetchLists()
  }, [])

  const fetchLists = async () => {
    try {
      const response = await api.get('/lists')
      setLists(response.data.data || [])
    } catch (error) {
      toast.error('Erro ao carregar listas')
    } finally {
      setLoading(false)
    }
  }

  const deleteList = async (id) => {
    if (!confirm('Tem certeza? Todos os emails serão perdidos.')) return

    try {
      await api.delete(`/lists/${id}`)
      toast.success('Lista excluída')
      fetchLists()
    } catch (error) {
      toast.error('Erro ao excluir')
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Listas de Emails</h1>
        <button
          onClick={() => setModal({ open: true, list: null })}
          className="btn btn-primary flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          Nova Lista
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {loading ? (
          <div className="col-span-full flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : lists.length === 0 ? (
          <div className="col-span-full text-center py-12 text-gray-500">
            <Mail className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>Nenhuma lista criada</p>
          </div>
        ) : (
          lists.map((list) => (
            <div key={list.id} className="card">
              <div className="flex items-start justify-between mb-4">
                <div>
                  <h3 className="font-semibold text-lg">{list.name}</h3>
                  {list.description && (
                    <p className="text-sm text-gray-500 mt-1">{list.description}</p>
                  )}
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => setModal({ open: true, list })}
                    className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                  >
                    <Edit className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => deleteList(list.id)}
                    className="p-2 text-red-600 hover:bg-red-50 rounded"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div className="flex items-center gap-6 text-sm text-gray-600 mb-4">
                <div className="flex items-center gap-2">
                  <Users className="w-4 h-4" />
                  {list.valid_emails?.toLocaleString()} emails
                </div>
                <span className={`badge ${list.status === 'ready' ? 'badge-success' : 'badge-warning'}`}>
                  {list.status === 'ready' ? 'Pronta' : 'Processando'}
                </span>
              </div>

              <button
                onClick={() => setUploadModal({ open: true, listId: list.id })}
                className="btn btn-secondary w-full flex items-center justify-center gap-2"
              >
                <Upload className="w-4 h-4" />
                Upload de Emails
              </button>
            </div>
          ))
        )}
      </div>

      {modal.open && (
        <ListModal
          list={modal.list}
          onClose={() => setModal({ open: false, list: null })}
          onSave={() => {
            setModal({ open: false, list: null })
            fetchLists()
          }}
        />
      )}

      {uploadModal.open && (
        <UploadModal
          listId={uploadModal.listId}
          onClose={() => setUploadModal({ open: false, listId: null })}
          onSuccess={() => {
            setUploadModal({ open: false, listId: null })
            fetchLists()
          }}
        />
      )}
    </div>
  )
}

export default EmailLists
