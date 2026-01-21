import { useState, useEffect } from 'react'
import {
  Globe,
  Plus,
  Trash2,
  Play,
  Pause,
  Settings,
  Mail,
  CheckCircle,
  XCircle,
  AlertCircle,
  Loader2,
  TestTube,
  RefreshCw,
  Eye,
  EyeOff,
  Chrome,
  Monitor,
  Upload,
  Send,
  Zap
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function WebWarmup() {
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [showAddModal, setShowAddModal] = useState(false)
  const [showImportModal, setShowImportModal] = useState(false)
  const [testingAccount, setTestingAccount] = useState(null)
  const [sendingTestEmail, setSendingTestEmail] = useState(null)
  const [importText, setImportText] = useState('')
  const [importing, setImporting] = useState(false)
  const [stats, setStats] = useState({
    total_accounts: 0,
    active_accounts: 0,
    emails_today: 0,
    success_rate: 0
  })
  const [settings, setSettings] = useState({
    enabled: false,
    emails_per_day: 10,
    delay_between_emails: 60,
    use_proxy: false,
    proxy_host: '',
    proxy_port: '',
    proxy_user: '',
    proxy_pass: ''
  })
  const [newAccount, setNewAccount] = useState({
    email: '',
    password: '',
    refresh_token: '',
    client_id: '',
    provider: 'outlook'
  })

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      setLoading(true)
      const [accountsRes, statsRes, settingsRes] = await Promise.all([
        api.get('/web-warmup/accounts').catch(() => ({ data: [] })),
        api.get('/web-warmup/stats').catch(() => ({ data: {} })),
        api.get('/web-warmup/settings').catch(() => ({ data: {} }))
      ])
      setAccounts(accountsRes.data || [])
      setStats(statsRes.data || {})
      if (settingsRes.data) {
        setSettings(prev => ({ ...prev, ...settingsRes.data }))
      }
    } catch (error) {
      console.error('Error loading data:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleAddAccount = async () => {
    if (!newAccount.email || !newAccount.password) {
      toast.error('Preencha email e senha')
      return
    }
    try {
      await api.post('/web-warmup/accounts', newAccount)
      toast.success('Conta adicionada!')
      setShowAddModal(false)
      setNewAccount({ email: '', password: '', refresh_token: '', client_id: '', provider: 'outlook' })
      loadData()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao adicionar conta')
    }
  }

  const handleImportAccounts = async () => {
    if (!importText.trim()) {
      toast.error('Cole as contas para importar')
      return
    }
    setImporting(true)
    try {
      const res = await api.post('/web-warmup/accounts/import', {
        accounts: importText,
        provider: 'outlook'
      })
      toast.success(`${res.data.imported} contas importadas!`)
      if (res.data.errors && res.data.errors.length > 0) {
        toast.error(`${res.data.errors.length} erros na importação`)
      }
      setShowImportModal(false)
      setImportText('')
      loadData()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao importar')
    } finally {
      setImporting(false)
    }
  }

  const handleSendTestEmail = async (account) => {
    const toEmail = prompt('Email destinatário para teste:')
    if (!toEmail) return

    setSendingTestEmail(account.id)
    try {
      const res = await api.post(`/web-warmup/accounts/${account.id}/send-test-email`, {
        to_email: toEmail
      })
      if (res.data.success) {
        toast.success(`Email enviado via ${res.data.method}! IP: ${res.data.ip || 'N/A'}`)
      } else {
        toast.error(res.data.error || 'Falha ao enviar')
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao enviar')
    } finally {
      setSendingTestEmail(null)
    }
  }

  const handleDeleteAccount = async (id) => {
    if (!confirm('Remover esta conta?')) return
    try {
      await api.delete(`/web-warmup/accounts/${id}`)
      toast.success('Conta removida')
      loadData()
    } catch (error) {
      toast.error('Erro ao remover')
    }
  }

  const handleTestAccount = async (account) => {
    setTestingAccount(account.id)
    try {
      const res = await api.post(`/web-warmup/accounts/${account.id}/test`)
      if (res.data.success) {
        toast.success(`Login OK! IP: ${res.data.ip || 'N/A'}`)
      } else {
        toast.error(res.data.error || 'Falha no login')
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro no teste')
    } finally {
      setTestingAccount(null)
    }
  }

  const handleToggleEnabled = async () => {
    try {
      const newEnabled = !settings.enabled
      await api.put('/web-warmup/settings', { ...settings, enabled: newEnabled })
      setSettings(prev => ({ ...prev, enabled: newEnabled }))
      toast.success(newEnabled ? 'Web Warmup ativado!' : 'Web Warmup pausado')
    } catch (error) {
      toast.error('Erro ao alterar status')
    }
  }

  const handleSaveSettings = async () => {
    try {
      await api.put('/web-warmup/settings', settings)
      toast.success('Configurações salvas!')
    } catch (error) {
      toast.error('Erro ao salvar')
    }
  }

  const handleToggleAccount = async (account) => {
    try {
      const newStatus = account.status === 'active' ? 'paused' : 'active'
      await api.put(`/web-warmup/accounts/${account.id}`, { status: newStatus })
      toast.success(newStatus === 'active' ? 'Conta ativada' : 'Conta pausada')
      loadData()
    } catch (error) {
      toast.error('Erro ao alterar status')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-3">
            <Globe className="w-8 h-8 text-blue-500" />
            Web Warmup
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            Aquecimento via automação web (browser) - funciona com proxy HTTP/HTTPS
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={handleToggleEnabled}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors ${
              settings.enabled
                ? 'bg-red-100 text-red-700 hover:bg-red-200'
                : 'bg-green-100 text-green-700 hover:bg-green-200'
            }`}
          >
            {settings.enabled ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
            {settings.enabled ? 'Pausar' : 'Iniciar'}
          </button>
          <button
            onClick={() => setShowImportModal(true)}
            className="flex items-center gap-2 px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors"
          >
            <Upload className="w-4 h-4" />
            Importar
          </button>
          <button
            onClick={() => setShowAddModal(true)}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
          >
            <Plus className="w-4 h-4" />
            Adicionar
          </button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
              <Monitor className="w-5 h-5 text-blue-600" />
            </div>
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Total Contas</p>
              <p className="text-xl font-bold text-gray-900 dark:text-white">{stats.total_accounts || 0}</p>
            </div>
          </div>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-green-100 dark:bg-green-900/30 rounded-lg">
              <CheckCircle className="w-5 h-5 text-green-600" />
            </div>
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Contas Ativas</p>
              <p className="text-xl font-bold text-gray-900 dark:text-white">{stats.active_accounts || 0}</p>
            </div>
          </div>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-purple-100 dark:bg-purple-900/30 rounded-lg">
              <Mail className="w-5 h-5 text-purple-600" />
            </div>
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Emails Hoje</p>
              <p className="text-xl font-bold text-gray-900 dark:text-white">{stats.emails_today || 0}</p>
            </div>
          </div>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-orange-100 dark:bg-orange-900/30 rounded-lg">
              <Globe className="w-5 h-5 text-orange-600" />
            </div>
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Taxa Sucesso</p>
              <p className="text-xl font-bold text-gray-900 dark:text-white">{stats.success_rate || 0}%</p>
            </div>
          </div>
        </div>
      </div>

      {/* Info Banner */}
      <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-xl p-4">
        <div className="flex items-start gap-3">
          <Zap className="w-6 h-6 text-blue-600 flex-shrink-0 mt-0.5" />
          <div>
            <h3 className="font-medium text-blue-900 dark:text-blue-100">Como funciona o Web Warmup?</h3>
            <p className="text-sm text-blue-700 dark:text-blue-300 mt-1">
              <strong>Microsoft Graph API:</strong> Usa tokens OAuth2 para enviar emails via HTTPS (porta 443).
              Funciona com TODOS os proxies pois não usa portas SMTP bloqueadas.
            </p>
            <p className="text-sm text-blue-700 dark:text-blue-300 mt-2">
              <strong>Formato de importação:</strong> <code className="bg-blue-100 dark:bg-blue-800 px-1 rounded">email TAB senha TAB token TAB client_id</code>
            </p>
          </div>
        </div>
      </div>

      {/* Settings */}
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
          <Settings className="w-5 h-5" />
          Configurações
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Emails por dia (por conta)
            </label>
            <input
              type="number"
              value={settings.emails_per_day}
              onChange={(e) => setSettings(prev => ({ ...prev, emails_per_day: parseInt(e.target.value) || 10 }))}
              className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Delay entre emails (segundos)
            </label>
            <input
              type="number"
              value={settings.delay_between_emails}
              onChange={(e) => setSettings(prev => ({ ...prev, delay_between_emails: parseInt(e.target.value) || 60 }))}
              className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
            />
          </div>
        </div>

        {/* Proxy Settings */}
        <div className="mt-6 pt-4 border-t border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3 mb-4">
            <input
              type="checkbox"
              id="use_proxy"
              checked={settings.use_proxy}
              onChange={(e) => setSettings(prev => ({ ...prev, use_proxy: e.target.checked }))}
              className="w-4 h-4 rounded border-gray-300"
            />
            <label htmlFor="use_proxy" className="font-medium text-gray-700 dark:text-gray-300">
              Usar Proxy (Bright Data, etc)
            </label>
          </div>

          {settings.use_proxy && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Host</label>
                <input
                  type="text"
                  value={settings.proxy_host}
                  onChange={(e) => setSettings(prev => ({ ...prev, proxy_host: e.target.value }))}
                  placeholder="brd.superproxy.io"
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Porta</label>
                <input
                  type="text"
                  value={settings.proxy_port}
                  onChange={(e) => setSettings(prev => ({ ...prev, proxy_port: e.target.value }))}
                  placeholder="22225"
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Usuário</label>
                <input
                  type="text"
                  value={settings.proxy_user}
                  onChange={(e) => setSettings(prev => ({ ...prev, proxy_user: e.target.value }))}
                  placeholder="brd-customer-xxx-zone-yyy"
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Senha</label>
                <input
                  type="password"
                  value={settings.proxy_pass}
                  onChange={(e) => setSettings(prev => ({ ...prev, proxy_pass: e.target.value }))}
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
            </div>
          )}
        </div>

        <div className="mt-4 flex justify-end">
          <button
            onClick={handleSaveSettings}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
          >
            Salvar Configurações
          </button>
        </div>
      </div>

      {/* Accounts List */}
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
          Contas Web ({accounts.length})
        </h2>

        {accounts.length === 0 ? (
          <div className="text-center py-12 text-gray-500 dark:text-gray-400">
            <Globe className="w-16 h-16 mx-auto mb-4 opacity-30" />
            <p>Nenhuma conta adicionada</p>
            <p className="text-sm mt-1">Adicione contas Outlook/Gmail para começar o Web Warmup</p>
          </div>
        ) : (
          <div className="space-y-3">
            {accounts.map(account => (
              <div
                key={account.id}
                className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-700/50 rounded-xl"
              >
                <div className="flex items-center gap-4">
                  <div className={`p-2 rounded-lg ${
                    account.status === 'active' ? 'bg-green-100 dark:bg-green-900/30' : 'bg-gray-200 dark:bg-gray-600'
                  }`}>
                    <Mail className={`w-5 h-5 ${
                      account.status === 'active' ? 'text-green-600' : 'text-gray-400'
                    }`} />
                  </div>
                  <div>
                    <p className="font-medium text-gray-900 dark:text-white">{account.email}</p>
                    <p className="text-sm text-gray-500 dark:text-gray-400">
                      {account.provider === 'outlook' ? 'Outlook/Hotmail' : 'Gmail'} •
                      Enviados: {account.emails_sent || 0} •
                      Último: {account.last_activity ? new Date(account.last_activity).toLocaleString('pt-BR') : 'Nunca'}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => handleTestAccount(account)}
                    disabled={testingAccount === account.id}
                    className="p-2 text-blue-600 hover:bg-blue-100 dark:hover:bg-blue-900/30 rounded-lg transition-colors"
                    title="Testar Conexão"
                  >
                    {testingAccount === account.id ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <TestTube className="w-4 h-4" />
                    )}
                  </button>
                  <button
                    onClick={() => handleSendTestEmail(account)}
                    disabled={sendingTestEmail === account.id}
                    className="p-2 text-purple-600 hover:bg-purple-100 dark:hover:bg-purple-900/30 rounded-lg transition-colors"
                    title="Enviar Email Teste"
                  >
                    {sendingTestEmail === account.id ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <Send className="w-4 h-4" />
                    )}
                  </button>
                  <button
                    onClick={() => handleToggleAccount(account)}
                    className={`p-2 rounded-lg transition-colors ${
                      account.status === 'active'
                        ? 'text-orange-600 hover:bg-orange-100 dark:hover:bg-orange-900/30'
                        : 'text-green-600 hover:bg-green-100 dark:hover:bg-green-900/30'
                    }`}
                    title={account.status === 'active' ? 'Pausar' : 'Ativar'}
                  >
                    {account.status === 'active' ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
                  </button>
                  <button
                    onClick={() => handleDeleteAccount(account.id)}
                    className="p-2 text-red-600 hover:bg-red-100 dark:hover:bg-red-900/30 rounded-lg transition-colors"
                    title="Remover"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Add Account Modal */}
      {showAddModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 w-full max-w-md mx-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
              Adicionar Conta Web
            </h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Provedor
                </label>
                <select
                  value={newAccount.provider}
                  onChange={(e) => setNewAccount(prev => ({ ...prev, provider: e.target.value }))}
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                >
                  <option value="outlook">Outlook / Hotmail</option>
                  <option value="gmail">Gmail (em breve)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Email
                </label>
                <input
                  type="email"
                  value={newAccount.email}
                  onChange={(e) => setNewAccount(prev => ({ ...prev, email: e.target.value }))}
                  placeholder="exemplo@outlook.com"
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Senha
                </label>
                <input
                  type="password"
                  value={newAccount.password}
                  onChange={(e) => setNewAccount(prev => ({ ...prev, password: e.target.value }))}
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Refresh Token (OAuth2)
                </label>
                <input
                  type="text"
                  value={newAccount.refresh_token}
                  onChange={(e) => setNewAccount(prev => ({ ...prev, refresh_token: e.target.value }))}
                  placeholder="M.C548_BAY.0.U.-ChP..."
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-xs"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Client ID (OAuth2)
                </label>
                <input
                  type="text"
                  value={newAccount.client_id}
                  onChange={(e) => setNewAccount(prev => ({ ...prev, client_id: e.target.value }))}
                  placeholder="9e5f94bc-e8a4-4e73-b8be-..."
                  className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-xs"
                />
              </div>
            </div>
            <div className="flex justify-end gap-3 mt-6">
              <button
                onClick={() => setShowAddModal(false)}
                className="px-4 py-2 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              >
                Cancelar
              </button>
              <button
                onClick={handleAddAccount}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
              >
                Adicionar
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Import Modal */}
      {showImportModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-2xl p-6 w-full max-w-2xl mx-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
              Importar Contas Web
            </h3>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
              Cole as contas no formato: <code className="bg-gray-100 dark:bg-gray-700 px-2 py-1 rounded">email TAB senha TAB token TAB client_id</code>
            </p>
            <textarea
              value={importText}
              onChange={(e) => setImportText(e.target.value)}
              placeholder="exemplo@outlook.com&#9;senha123&#9;M.C548_BAY.0.U.-ChP...&#9;9e5f94bc-e8a4-..."
              rows={10}
              className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg font-mono text-sm"
            />
            <p className="text-xs text-gray-400 mt-2">
              Uma conta por linha. Use TAB para separar os campos.
            </p>
            <div className="flex justify-end gap-3 mt-6">
              <button
                onClick={() => { setShowImportModal(false); setImportText(''); }}
                className="px-4 py-2 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              >
                Cancelar
              </button>
              <button
                onClick={handleImportAccounts}
                disabled={importing}
                className="px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors disabled:opacity-50 flex items-center gap-2"
              >
                {importing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
                {importing ? 'Importando...' : 'Importar'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default WebWarmup
