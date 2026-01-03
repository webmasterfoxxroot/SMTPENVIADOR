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
  const [step, setStep] = useState(1) // 1: upload, 2: preview, 3: importing
  const [loading, setLoading] = useState(false)
  const [file, setFile] = useState(null)
  const [dbType, setDbType] = useState('mailwizz')
  const [previewResult, setPreviewResult] = useState(null)
  const [importResult, setImportResult] = useState(null)

  const databaseTypes = [
    { value: 'mailwizz', name: 'MailWizz', table: 'mw_email_blacklist' },
    { value: 'newapp', name: 'NewApp', table: 'email_banned_emails' },
    { value: 'mumara', name: 'Mumara', table: 'suppression_list' }
  ]

  const handlePreview = async () => {
    if (!file) {
      toast.error('Selecione um arquivo .sql')
      return
    }

    setLoading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('type', dbType)

    try {
      const response = await api.post('/blacklist/migration/preview', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      setPreviewResult(response.data)
      setStep(2)
      toast.success('Arquivo analisado!')
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao analisar arquivo')
      setPreviewResult({ error: error.response?.data?.error || 'Erro desconhecido' })
      setStep(2)
    } finally {
      setLoading(false)
    }
  }

  const handleImport = async () => {
    setLoading(true)
    setStep(3)

    const formData = new FormData()
    formData.append('file', file)
    formData.append('type', dbType)

    try {
      const response = await api.post('/blacklist/migration/import', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      setImportResult(response.data)
      toast.success(`Importacao concluida! ${response.data.imported} emails importados`)
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro na importacao')
      setImportResult({ error: error.response?.data?.error || 'Erro desconhecido' })
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (importResult && !importResult.error) {
      onSuccess()
    } else {
      onClose()
    }
  }

  const resetForm = () => {
    setStep(1)
    setFile(null)
    setPreviewResult(null)
    setImportResult(null)
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
                accept=".sql"
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
              <p className="font-medium mb-1">Como exportar o arquivo SQL:</p>
              <p>Use phpMyAdmin ou mysqldump para exportar a tabela de blacklist do seu sistema antigo.</p>
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <button type="button" onClick={onClose} className="btn btn-secondary">
                Cancelar
              </button>
              <button onClick={handlePreview} disabled={loading || !file} className="btn btn-primary">
                {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Analisar Arquivo'}
              </button>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-4">
            {previewResult?.error ? (
              <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
                <div className="flex items-center gap-2 text-red-700 font-medium mb-2">
                  <AlertCircle className="w-5 h-5" />
                  Erro ao Analisar
                </div>
                <p className="text-red-600 text-sm">{previewResult.error}</p>
              </div>
            ) : (
              <>
                <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
                  <div className="flex items-center gap-2 text-green-700 font-medium mb-2">
                    <CheckCircle2 className="w-5 h-5" />
                    Arquivo Analisado!
                  </div>
                  <div className="text-sm text-green-700 space-y-1">
                    <p><strong>Tabela encontrada:</strong> {previewResult.table}</p>
                    <p><strong>Total de Emails:</strong> {previewResult.total_emails?.toLocaleString()}</p>
                  </div>
                </div>

                {previewResult.samples && previewResult.samples.length > 0 && (
                  <div className="p-4 bg-gray-50 border border-gray-200 rounded-lg">
                    <p className="text-sm font-medium text-gray-700 mb-2">Amostra de Emails:</p>
                    <ul className="text-sm text-gray-600 font-mono space-y-1">
                      {previewResult.samples.map((email, i) => (
                        <li key={i}>{email}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={resetForm} className="btn btn-secondary">
                Voltar
              </button>
              {!previewResult?.error && (
                <button onClick={handleImport} disabled={loading} className="btn btn-primary">
                  {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : `Importar ${previewResult?.total_emails?.toLocaleString()} Emails`}
                </button>
              )}
            </div>
          </div>
        )}

        {step === 3 && (
          <div className="space-y-4">
            {loading ? (
              <div className="text-center py-8">
                <Loader2 className="w-12 h-12 animate-spin mx-auto text-blue-500 mb-4" />
                <p className="text-gray-600">Importando emails...</p>
                <p className="text-sm text-gray-500">Isso pode levar alguns minutos</p>
              </div>
            ) : importResult?.error ? (
              <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
                <div className="flex items-center gap-2 text-red-700 font-medium mb-2">
                  <AlertCircle className="w-5 h-5" />
                  Erro na Importacao
                </div>
                <p className="text-red-600 text-sm">{importResult.error}</p>
              </div>
            ) : (
              <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
                <div className="flex items-center gap-2 text-green-700 font-medium mb-2">
                  <CheckCircle2 className="w-5 h-5" />
                  Importacao Concluida!
                </div>
                <div className="text-sm text-green-700 space-y-1">
                  <p><strong>Fonte:</strong> {importResult.source}</p>
                  <p><strong>Tabela:</strong> {importResult.table}</p>
                  <p><strong>Importados:</strong> {importResult.imported?.toLocaleString()}</p>
                  <p><strong>Duplicados (ja existiam):</strong> {importResult.duplicates?.toLocaleString()}</p>
                  <p className="pt-2 border-t border-green-200 mt-2">
                    <strong>Total processado:</strong> {importResult.total?.toLocaleString()}
                  </p>
                </div>
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={handleClose} className="btn btn-primary">
                {importResult?.error ? 'Voltar' : 'Fechar'}
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
