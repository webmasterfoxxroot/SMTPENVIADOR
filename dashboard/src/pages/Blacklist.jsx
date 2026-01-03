import { useState, useEffect, useRef } from 'react'
import { Plus, Trash2, Upload, Search, Ban, Loader2, Database, CheckCircle2, AlertCircle, X } from 'lucide-react'
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

function DatabaseMigrationModal({ onClose, onStarted }) {
  const [loading, setLoading] = useState(false)
  const [file, setFile] = useState(null)
  const [dbType, setDbType] = useState('mailwizz')

  const databaseTypes = [
    { value: 'mailwizz', name: 'MailWizz', table: 'mw_email_blacklist' },
    { value: 'newapp', name: 'NewApp', table: 'email_banned_emails' },
    { value: 'mumara', name: 'Mumara', table: 'suppression_list' }
  ]

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
      toast.success('Importacao iniciada em segundo plano!')
      onStarted(response.data.job_id)
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao iniciar importacao')
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 w-full max-w-lg">
        <h2 className="text-xl font-bold mb-4 flex items-center gap-2">
          <Database className="w-5 h-5" />
          Importar Blacklist de SQL
        </h2>

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
            <p>Voce pode fechar este modal e mudar de pagina. O progresso aparecera nesta pagina.</p>
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
      </div>
    </div>
  )
}

// Progress bar component shown on the main page
function ImportProgressBar({ job, onCancel, onDismiss }) {
  const progress = job.total_emails > 0 ? Math.round((job.processed / job.total_emails) * 100) : 0

  const isCompleted = job.status === 'completed'
  const isFailed = job.status === 'failed'
  const isProcessing = job.status === 'processing' || job.status === 'pending'

  return (
    <div className={`mb-4 p-4 rounded-lg border ${
      isCompleted ? 'bg-green-50 border-green-200' :
      isFailed ? 'bg-red-50 border-red-200' :
      'bg-blue-50 border-blue-200'
    }`}>
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          {isProcessing && <Loader2 className="w-5 h-5 animate-spin text-blue-500" />}
          {isCompleted && <CheckCircle2 className="w-5 h-5 text-green-500" />}
          {isFailed && <AlertCircle className="w-5 h-5 text-red-500" />}
          <span className="font-medium">
            {isProcessing && `Importando SQL (${job.db_type})...`}
            {isCompleted && 'Importacao concluida!'}
            {isFailed && 'Erro na importacao'}
          </span>
        </div>
        <button
          onClick={isProcessing ? onCancel : onDismiss}
          className="text-gray-400 hover:text-gray-600"
          title={isProcessing ? 'Cancelar' : 'Fechar'}
        >
          <X className="w-5 h-5" />
        </button>
      </div>

      {isProcessing && (
        <>
          <div className="w-full bg-gray-200 rounded-full h-2 mb-2">
            <div
              className="bg-blue-500 h-2 rounded-full transition-all duration-300"
              style={{ width: `${progress}%` }}
            />
          </div>
          <div className="flex justify-between text-sm text-gray-600">
            <span>{progress}% - {job.processed?.toLocaleString()}/{job.total_emails?.toLocaleString()} emails</span>
            <span>
              <span className="text-green-600">{job.imported?.toLocaleString()} importados</span>
              {' | '}
              <span className="text-yellow-600">{job.duplicates?.toLocaleString()} duplicados</span>
            </span>
          </div>
        </>
      )}

      {isCompleted && (
        <div className="text-sm text-green-700">
          <strong>{job.imported?.toLocaleString()}</strong> emails importados,
          <strong> {job.duplicates?.toLocaleString()}</strong> duplicados
          {job.errors > 0 && <>, <strong className="text-red-600">{job.errors?.toLocaleString()}</strong> erros</>}
        </div>
      )}

      {isFailed && (
        <div className="text-sm text-red-600">
          {job.error_message || 'Erro desconhecido'}
        </div>
      )}
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
  const [activeJob, setActiveJob] = useState(null)
  const pollInterval = useRef(null)

  // Initial load - fetch blacklist and check for active jobs
  useEffect(() => {
    fetchBlacklist()
    checkActiveJobs()

    return () => {
      if (pollInterval.current) {
        clearInterval(pollInterval.current)
      }
    }
  }, [])

  // Fetch on page/search change
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

  // Check for active import jobs on page load
  const checkActiveJobs = async () => {
    try {
      const response = await api.get('/blacklist/migration/jobs')
      const jobs = response.data?.data || []

      // Find active job (pending or processing)
      const active = jobs.find(j => j.status === 'pending' || j.status === 'processing')

      if (active) {
        console.log('Found active blacklist import job:', active)
        setActiveJob(active)
        startPolling(active.id)
      }
    } catch (error) {
      console.log('No active import jobs')
    }
  }

  const startPolling = (jobId) => {
    // Clear existing poll
    if (pollInterval.current) {
      clearInterval(pollInterval.current)
    }

    // Poll immediately then every second
    pollJobStatus(jobId)
    pollInterval.current = setInterval(() => pollJobStatus(jobId), 1000)
  }

  const pollJobStatus = async (jobId) => {
    try {
      const response = await api.get(`/blacklist/migration/status/${jobId}`)
      const job = response.data

      setActiveJob(job)

      if (job.status === 'completed') {
        clearInterval(pollInterval.current)
        pollInterval.current = null
        toast.success(`${job.imported?.toLocaleString()} emails importados!`)
        fetchBlacklist()

        // Keep showing completed status for 30 seconds
        setTimeout(() => {
          setActiveJob(null)
        }, 30000)
      } else if (job.status === 'failed') {
        clearInterval(pollInterval.current)
        pollInterval.current = null
        toast.error(job.error_message || 'Erro na importacao')
      }
    } catch (error) {
      console.error('Poll error:', error)
    }
  }

  const handleImportStarted = (jobId) => {
    setMigrationModal(false)
    setActiveJob({ id: jobId, status: 'pending', progress: 0 })
    startPolling(jobId)
  }

  const dismissJob = () => {
    if (pollInterval.current) {
      clearInterval(pollInterval.current)
      pollInterval.current = null
    }
    setActiveJob(null)
    fetchBlacklist()
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
            disabled={activeJob && (activeJob.status === 'pending' || activeJob.status === 'processing')}
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

      {/* Active Import Progress */}
      {activeJob && (
        <ImportProgressBar
          job={activeJob}
          onCancel={dismissJob}
          onDismiss={dismissJob}
        />
      )}

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
          onStarted={handleImportStarted}
        />
      )}
    </div>
  )
}

export default Blacklist
