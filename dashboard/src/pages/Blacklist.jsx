import { useState, useEffect } from 'react'
import { Plus, Trash2, Upload, Search, Ban, Loader2, Database, CheckCircle2, AlertCircle } from 'lucide-react'
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

function DatabaseMigrationModal({ onClose, onSuccess }) {
  const [step, setStep] = useState(1) // 1: upload, 2: importing/progress, 3: completed
  const [loading, setLoading] = useState(false)
  const [file, setFile] = useState(null)
  const [dbType, setDbType] = useState('mailwizz')
  const [jobId, setJobId] = useState(null)
  const [jobStatus, setJobStatus] = useState(null)

  const databaseTypes = [
    { value: 'mailwizz', name: 'MailWizz', table: 'mw_email_blacklist' },
    { value: 'newapp', name: 'NewApp', table: 'email_banned_emails' },
    { value: 'mumara', name: 'Mumara', table: 'suppression_list' }
  ]

  // Poll job status
  useEffect(() => {
    if (!jobId || step !== 2) return

    const pollStatus = async () => {
      try {
        const response = await api.get(`/blacklist/migration/status/${jobId}`)
        setJobStatus(response.data)

        if (response.data.status === 'completed') {
          setStep(3)
          toast.success(`Importacao concluida! ${response.data.imported} emails importados`)
        } else if (response.data.status === 'failed') {
          setStep(3)
          toast.error(response.data.error_message || 'Erro na importacao')
        }
      } catch (error) {
        console.error('Error polling status:', error)
      }
    }

    pollStatus()
    const interval = setInterval(pollStatus, 1000)
    return () => clearInterval(interval)
  }, [jobId, step])

  const handleStartImport = async () => {
    if (!file) {
      toast.error('Selecione um arquivo .sql')
      return
    }

    setLoading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('type', dbType)

    try {
      const response = await api.post('/blacklist/migration/import', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      setJobId(response.data.job_id)
      setStep(2)
      toast.success('Importacao iniciada!')
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar importacao')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (jobStatus?.status === 'completed') {
      onSuccess()
    } else {
      onClose()
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 w-full max-w-lg">
        <h2 className="text-xl font-bold mb-4 flex items-center gap-2">
          <Database className="w-5 h-5" />
          Importar Blacklist de SQL
        </h2>

        {step === 1 && (
          <div className="space-y-4">
            <p className="text-sm text-gray-600 mb-4">
              Carregue o arquivo .sql exportado do banco de dados (MailWizz, NewApp, Mumara)
            </p>

            <div>
              <label className="label">Tipo de Banco de Origem</label>
              <select
                value={dbType}
                onChange={(e) => setDbType(e.target.value)}
                className="input"
              >
                {databaseTypes.map((db) => (
                  <option key={db.value} value={db.value}>
                    {db.name} (tabela: {db.table})
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="label">Arquivo SQL</label>
              <input
                type="file"
                accept=".sql,.txt,*/*"
                onChange={(e) => setFile(e.target.files[0])}
                className="input"
              />
              {file && (
                <p className="text-sm text-gray-500 mt-1">
                  Arquivo: {file.name} ({(file.size / 1024 / 1024).toFixed(2)} MB)
                </p>
              )}
            </div>

            <div className="p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-700">
              <p className="font-medium mb-1">A importacao roda em segundo plano</p>
              <p>Voce pode fechar este modal e continuar usando o sistema. A importacao continuara.</p>
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <button type="button" onClick={onClose} className="btn btn-secondary">
                Cancelar
              </button>
              <button onClick={handleStartImport} disabled={loading || !file} className="btn btn-primary">
                {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Iniciar Importacao'}
              </button>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-4">
            <div className="text-center py-4">
              <Loader2 className="w-12 h-12 animate-spin mx-auto text-blue-500 mb-4" />
              <p className="text-gray-600 font-medium">Importando emails...</p>
              <p className="text-sm text-gray-500 mb-4">Voce pode fechar este modal - a importacao continuara em segundo plano</p>
            </div>

            {jobStatus && (
              <div className="space-y-3">
                <div className="flex justify-between text-sm">
                  <span className="text-gray-600">Progresso:</span>
                  <span className="font-medium">{jobStatus.progress}%</span>
                </div>
                <div className="w-full bg-gray-200 rounded-full h-3">
                  <div
                    className="bg-blue-500 h-3 rounded-full transition-all duration-300"
                    style={{ width: `${jobStatus.progress}%` }}
                  />
                </div>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div className="p-2 bg-gray-50 rounded">
                    <span className="text-gray-500">Processados:</span>
                    <span className="font-medium ml-1">{jobStatus.processed?.toLocaleString()}/{jobStatus.total_emails?.toLocaleString()}</span>
                  </div>
                  <div className="p-2 bg-green-50 rounded">
                    <span className="text-gray-500">Importados:</span>
                    <span className="font-medium ml-1 text-green-600">{jobStatus.imported?.toLocaleString()}</span>
                  </div>
                  <div className="p-2 bg-yellow-50 rounded">
                    <span className="text-gray-500">Duplicados:</span>
                    <span className="font-medium ml-1 text-yellow-600">{jobStatus.duplicates?.toLocaleString()}</span>
                  </div>
                  <div className="p-2 bg-red-50 rounded">
                    <span className="text-gray-500">Erros:</span>
                    <span className="font-medium ml-1 text-red-600">{jobStatus.errors?.toLocaleString()}</span>
                  </div>
                </div>
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={onClose} className="btn btn-secondary">
                Fechar (continua em segundo plano)
              </button>
            </div>
          </div>
        )}

        {step === 3 && (
          <div className="space-y-4">
            {jobStatus?.status === 'failed' ? (
              <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
                <div className="flex items-center gap-2 text-red-700 font-medium mb-2">
                  <AlertCircle className="w-5 h-5" />
                  Erro na Importacao
                </div>
                <p className="text-red-600 text-sm">{jobStatus.error_message}</p>
              </div>
            ) : (
              <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
                <div className="flex items-center gap-2 text-green-700 font-medium mb-2">
                  <CheckCircle2 className="w-5 h-5" />
                  Importacao Concluida!
                </div>
                <div className="text-sm text-green-700 space-y-1">
                  <p><strong>Fonte:</strong> {jobStatus?.db_type}</p>
                  <p><strong>Total processado:</strong> {jobStatus?.total_emails?.toLocaleString()}</p>
                  <p><strong>Importados:</strong> {jobStatus?.imported?.toLocaleString()}</p>
                  <p><strong>Duplicados:</strong> {jobStatus?.duplicates?.toLocaleString()}</p>
                  {jobStatus?.errors > 0 && (
                    <p><strong>Erros:</strong> {jobStatus?.errors?.toLocaleString()}</p>
                  )}
                </div>
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={handleClose} className="btn btn-primary">
                Fechar
              </button>
            </div>
          </div>
        )}
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
  const [migrationModal, setMigrationModal] = useState(false)

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

  const clearAllBlacklist = async () => {
    if (!confirm(`Tem certeza que deseja APAGAR TODOS os ${total.toLocaleString()} emails da blacklist?\n\nEssa ação não pode ser desfeita!`)) return

    try {
      const response = await api.delete('/blacklist')
      toast.success(`${response.data.deleted.toLocaleString()} emails removidos da blacklist`)
      fetchBlacklist()
    } catch (error) {
      toast.error('Erro ao limpar blacklist')
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
          {total > 0 && (
            <button
              onClick={clearAllBlacklist}
              className="btn btn-danger flex items-center gap-2"
            >
              <Trash2 className="w-4 h-4" />
              Limpar Tudo
            </button>
          )}
          <button
            onClick={() => setMigrationModal(true)}
            className="btn btn-secondary flex items-center gap-2"
          >
            <Database className="w-4 h-4" />
            Importar SQL
          </button>
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

      {migrationModal && (
        <DatabaseMigrationModal
          onClose={() => setMigrationModal(false)}
          onSuccess={() => {
            setMigrationModal(false)
            fetchBlacklist()
          }}
        />
      )}
    </div>
  )
}

export default Blacklist
