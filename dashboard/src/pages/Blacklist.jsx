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
  const [step, setStep] = useState(1) // 1: config, 2: test result, 3: migrating
  const [loading, setLoading] = useState(false)
  const [testResult, setTestResult] = useState(null)
  const [migrationResult, setMigrationResult] = useState(null)
  const [config, setConfig] = useState({
    host: '',
    port: 3306,
    user: '',
    password: '',
    database: '',
    type: 'mailwizz'
  })

  const databaseTypes = [
    { value: 'mailwizz', name: 'MailWizz', table: 'mw_email_blacklist' },
    { value: 'newapp', name: 'NewApp', table: 'email_banned_emails' },
    { value: 'mumara', name: 'Mumara', table: 'suppression_list' }
  ]

  const handleTest = async () => {
    if (!config.host || !config.user || !config.database) {
      toast.error('Preencha host, usuario e banco de dados')
      return
    }

    setLoading(true)
    try {
      const response = await api.post('/blacklist/migration/test', config)
      setTestResult(response.data)
      setStep(2)
      toast.success('Conexao bem sucedida!')
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao testar conexao')
      setTestResult({ error: error.response?.data?.error || 'Erro desconhecido' })
    } finally {
      setLoading(false)
    }
  }

  const handleMigrate = async () => {
    setLoading(true)
    setStep(3)
    try {
      const response = await api.post('/blacklist/migration/import', config)
      setMigrationResult(response.data)
      toast.success(`Migracao concluida! ${response.data.imported} emails importados`)
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro na migracao')
      setMigrationResult({ error: error.response?.data?.error || 'Erro desconhecido' })
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (migrationResult && !migrationResult.error) {
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
          Migrar de Banco de Dados
        </h2>

        {step === 1 && (
          <div className="space-y-4">
            <p className="text-sm text-gray-600 mb-4">
              Importe emails de blacklists de outros sistemas (MailWizz, NewApp, Mumara)
            </p>

            <div>
              <label className="label">Tipo de Banco</label>
              <select
                value={config.type}
                onChange={(e) => setConfig({ ...config, type: e.target.value })}
                className="input"
              >
                {databaseTypes.map((db) => (
                  <option key={db.value} value={db.value}>
                    {db.name} ({db.table})
                  </option>
                ))}
              </select>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="label">Host</label>
                <input
                  type="text"
                  value={config.host}
                  onChange={(e) => setConfig({ ...config, host: e.target.value })}
                  className="input"
                  placeholder="localhost ou IP"
                />
              </div>
              <div>
                <label className="label">Porta</label>
                <input
                  type="number"
                  value={config.port}
                  onChange={(e) => setConfig({ ...config, port: parseInt(e.target.value) || 3306 })}
                  className="input"
                  placeholder="3306"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="label">Usuario</label>
                <input
                  type="text"
                  value={config.user}
                  onChange={(e) => setConfig({ ...config, user: e.target.value })}
                  className="input"
                  placeholder="root"
                />
              </div>
              <div>
                <label className="label">Senha</label>
                <input
                  type="password"
                  value={config.password}
                  onChange={(e) => setConfig({ ...config, password: e.target.value })}
                  className="input"
                  placeholder="********"
                />
              </div>
            </div>

            <div>
              <label className="label">Nome do Banco de Dados</label>
              <input
                type="text"
                value={config.database}
                onChange={(e) => setConfig({ ...config, database: e.target.value })}
                className="input"
                placeholder="mailwizz, newapp, mumara..."
              />
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <button type="button" onClick={onClose} className="btn btn-secondary">
                Cancelar
              </button>
              <button onClick={handleTest} disabled={loading} className="btn btn-primary">
                {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Testar Conexao'}
              </button>
            </div>
          </div>
        )}

        {step === 2 && testResult && (
          <div className="space-y-4">
            {testResult.error ? (
              <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
                <div className="flex items-center gap-2 text-red-700 font-medium mb-2">
                  <AlertCircle className="w-5 h-5" />
                  Erro na Conexao
                </div>
                <p className="text-red-600 text-sm">{testResult.error}</p>
              </div>
            ) : (
              <>
                <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
                  <div className="flex items-center gap-2 text-green-700 font-medium mb-2">
                    <CheckCircle2 className="w-5 h-5" />
                    Conexao Bem Sucedida!
                  </div>
                  <div className="text-sm text-green-700 space-y-1">
                    <p><strong>Tabela:</strong> {testResult.table}</p>
                    <p><strong>Coluna de Email:</strong> {testResult.email_column}</p>
                    <p><strong>Total de Emails:</strong> {testResult.total_emails?.toLocaleString()}</p>
                  </div>
                </div>

                {testResult.samples && testResult.samples.length > 0 && (
                  <div className="p-4 bg-gray-50 border border-gray-200 rounded-lg">
                    <p className="text-sm font-medium text-gray-700 mb-2">Amostra de Emails:</p>
                    <ul className="text-sm text-gray-600 font-mono space-y-1">
                      {testResult.samples.map((email, i) => (
                        <li key={i}>{email}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={() => { setStep(1); setTestResult(null) }} className="btn btn-secondary">
                Voltar
              </button>
              {!testResult.error && (
                <button onClick={handleMigrate} disabled={loading} className="btn btn-primary">
                  {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : `Importar ${testResult.total_emails?.toLocaleString()} Emails`}
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
                <p className="text-gray-600">Migrando emails...</p>
                <p className="text-sm text-gray-500">Isso pode levar alguns minutos</p>
              </div>
            ) : migrationResult?.error ? (
              <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
                <div className="flex items-center gap-2 text-red-700 font-medium mb-2">
                  <AlertCircle className="w-5 h-5" />
                  Erro na Migracao
                </div>
                <p className="text-red-600 text-sm">{migrationResult.error}</p>
              </div>
            ) : (
              <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
                <div className="flex items-center gap-2 text-green-700 font-medium mb-2">
                  <CheckCircle2 className="w-5 h-5" />
                  Migracao Concluida!
                </div>
                <div className="text-sm text-green-700 space-y-1">
                  <p><strong>Fonte:</strong> {migrationResult.source}</p>
                  <p><strong>Importados:</strong> {migrationResult.imported?.toLocaleString()}</p>
                  <p><strong>Duplicados (ja existiam):</strong> {migrationResult.duplicates?.toLocaleString()}</p>
                  {migrationResult.errors > 0 && (
                    <p><strong>Erros:</strong> {migrationResult.errors?.toLocaleString()}</p>
                  )}
                  <p className="pt-2 border-t border-green-200 mt-2">
                    <strong>Total processado:</strong> {migrationResult.total?.toLocaleString()}
                  </p>
                </div>
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <button onClick={handleClose} className="btn btn-primary">
                {migrationResult?.error ? 'Voltar' : 'Fechar'}
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
            Migrar de Banco
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
