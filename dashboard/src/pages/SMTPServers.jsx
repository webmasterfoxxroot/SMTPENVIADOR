import { useState, useEffect } from 'react'
import {
  Plus,
  Edit,
  Trash2,
  CheckCircle,
  XCircle,
  RefreshCw,
  TestTube,
  Loader2,
  Server,
  Send,
  X,
  Mail,
  Inbox,
  CheckCircle2,
  AlertCircle
} from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function SMTPModal({ smtp, onClose, onSave }) {
  const [form, setForm] = useState({
    name: '',
    host: '',
    port: 587,
    username: '',
    password: '',
    tls_mode: 'starttls',
    max_per_minute: 1000,
    max_per_hour: 50000,
    max_connections: 5,
    active: true,
    // IMAP configuration
    imap_host: '',
    imap_port: 993,
    imap_password: '',
    imap_tls_mode: 'tls',
    ...smtp
  })
  const [senders, setSenders] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadingSenders, setLoadingSenders] = useState(false)
  const [testingSmtp, setTestingSmtp] = useState(false)
  const [testingImap, setTestingImap] = useState(false)
  const [smtpTestResult, setSmtpTestResult] = useState(null)
  const [imapTestResult, setImapTestResult] = useState(null)

  // Load existing senders when editing
  useEffect(() => {
    if (smtp?.id) {
      loadSenders()
    }
  }, [smtp?.id])

  const loadSenders = async () => {
    try {
      const response = await api.get(`/smtp/${smtp.id}/senders`)
      const senderList = response.data.data || []
      const senderText = senderList.map(s => {
        if (s.name) return `${s.email}|${s.name}`
        return s.email
      }).join('\n')
      setSenders(senderText)

      // Load IMAP data from first sender (if exists)
      if (senderList.length > 0) {
        const firstSender = senderList[0]
        if (firstSender.imap_host) {
          setForm(prev => ({
            ...prev,
            imap_host: firstSender.imap_host || '',
            imap_port: firstSender.imap_port || 993,
            imap_password: '', // Don't show password for security
            imap_tls_mode: firstSender.imap_tls_mode || 'tls'
          }))
        }
      }
    } catch (error) {
      console.error('Error loading senders:', error)
    }
  }

  // Test SMTP connection
  const testSmtpConnection = async () => {
    if (!form.host || !form.username || !form.password) {
      toast.error('Preencha host, usuário e senha do SMTP')
      return
    }
    setTestingSmtp(true)
    setSmtpTestResult(null)
    try {
      await api.post('/smtp/test-connection', {
        host: form.host,
        port: form.port,
        username: form.username,
        password: form.password,
        tls_mode: form.tls_mode
      })
      setSmtpTestResult({ success: true, message: 'Conexão SMTP OK!' })
      toast.success('Conexão SMTP OK!')
    } catch (error) {
      const msg = error.response?.data?.details || error.response?.data?.error || 'Falha na conexão SMTP'
      setSmtpTestResult({ success: false, message: msg })
      toast.error(msg)
    } finally {
      setTestingSmtp(false)
    }
  }

  // Test IMAP connection
  const testImapConnection = async () => {
    if (!form.imap_host || !form.username) {
      toast.error('Preencha host IMAP e usuário')
      return
    }
    const imapPassword = form.imap_password || form.password
    if (!imapPassword) {
      toast.error('Preencha a senha IMAP ou senha SMTP')
      return
    }
    setTestingImap(true)
    setImapTestResult(null)
    try {
      const response = await api.post('/warmup/seeds/test-connection', {
        email: form.username,
        password: imapPassword,
        imap_host: form.imap_host,
        imap_port: form.imap_port || 993,
        imap_tls_mode: form.imap_tls_mode || 'tls',
        test_type: 'imap'
      })
      // Check response for IMAP result
      if (response.data?.imap?.success) {
        setImapTestResult({ success: true, message: 'Conexão IMAP OK!' })
        toast.success('Conexão IMAP OK!')
      } else {
        const msg = response.data?.imap?.error || 'Falha na conexão IMAP'
        setImapTestResult({ success: false, message: msg })
        toast.error(msg)
      }
    } catch (error) {
      const msg = error.response?.data?.details || error.response?.data?.error || 'Falha na conexão IMAP'
      setImapTestResult({ success: false, message: msg })
      toast.error(msg)
    } finally {
      setTestingImap(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      let smtpId = smtp?.id

      if (smtp?.id) {
        await api.put(`/smtp/${smtp.id}`, form)
      } else {
        const response = await api.post('/smtp', form)
        smtpId = response.data.id
      }

      // Save senders if we have an SMTP ID
      if (smtpId) {
        const senderLines = senders.trim().split('\n').filter(line => line.trim())
        const senderData = senderLines.map(line => {
          const parts = line.trim().split('|')
          return {
            email: parts[0].trim(),
            name: parts[1]?.trim() || '',
            // Include IMAP config for each sender
            imap_host: form.imap_host,
            imap_port: form.imap_port,
            imap_password: form.imap_password || form.password,
            imap_tls_mode: form.imap_tls_mode
          }
        })

        // Always call bulk endpoint with clear_existing to sync senders
        await api.post(`/smtp/${smtpId}/senders/bulk`, {
          senders: senderData,
          clear_existing: true
        })
      }

      toast.success(smtp?.id ? 'SMTP atualizado!' : 'SMTP criado!')
      onSave()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="text-xl font-bold mb-4 text-gray-900 dark:text-white">
          {smtp?.id ? 'Editar SMTP' : 'Novo SMTP'}
        </h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="label">Nome</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="input"
              placeholder="PMTA Principal"
              required
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Host</label>
              <input
                type="text"
                value={form.host}
                onChange={(e) => setForm({ ...form, host: e.target.value })}
                className="input"
                placeholder="mail.servidor.com"
                required
              />
            </div>
            <div>
              <label className="label">Porta</label>
              <input
                type="number"
                value={form.port}
                onChange={(e) => setForm({ ...form, port: parseInt(e.target.value) })}
                className="input"
                required
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Usuário</label>
              <input
                type="text"
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                className="input"
                required
              />
            </div>
            <div>
              <label className="label">Senha</label>
              <input
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                className="input"
                placeholder={smtp?.id ? '(manter atual)' : ''}
                required={!smtp?.id}
              />
            </div>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="label">Max/Minuto</label>
              <input
                type="number"
                value={form.max_per_minute}
                onChange={(e) => setForm({ ...form, max_per_minute: parseInt(e.target.value) })}
                className="input"
              />
            </div>
            <div>
              <label className="label">Max/Hora</label>
              <input
                type="number"
                value={form.max_per_hour}
                onChange={(e) => setForm({ ...form, max_per_hour: parseInt(e.target.value) })}
                className="input"
              />
            </div>
            <div>
              <label className="label">Conexões</label>
              <input
                type="number"
                value={form.max_connections}
                onChange={(e) => setForm({ ...form, max_connections: parseInt(e.target.value) })}
                className="input"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Modo TLS</label>
              <select
                value={form.tls_mode}
                onChange={(e) => setForm({ ...form, tls_mode: e.target.value })}
                className="input"
              >
                <option value="none">Nenhum (sem criptografia)</option>
                <option value="starttls">STARTTLS (porta 587/25)</option>
                <option value="tls">TLS Implícito (porta 465/outras)</option>
              </select>
            </div>
            <div className="flex items-end pb-2">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.active}
                  onChange={(e) => setForm({ ...form, active: e.target.checked })}
                  className="w-4 h-4"
                />
                Ativo
              </label>
            </div>
          </div>

          {/* Botão Testar SMTP */}
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={testSmtpConnection}
              disabled={testingSmtp}
              className="flex-1 py-2.5 px-4 border-2 border-blue-200 bg-blue-50 hover:bg-blue-100 text-blue-700 rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
            >
              {testingSmtp ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  Testando SMTP...
                </>
              ) : (
                <>
                  <Mail className="w-4 h-4" />
                  Testar Conexão SMTP
                </>
              )}
            </button>
            {smtpTestResult && (
              <div className={`flex items-center gap-1.5 text-sm ${smtpTestResult.success ? 'text-green-600' : 'text-red-600'}`}>
                {smtpTestResult.success ? (
                  <CheckCircle2 className="w-5 h-5" />
                ) : (
                  <AlertCircle className="w-5 h-5" />
                )}
                <span className="max-w-[150px] truncate">{smtpTestResult.message}</span>
              </div>
            )}
          </div>

          {/* Separador IMAP */}
          <div className="border-t border-gray-200 dark:border-gray-700 pt-4 mt-2">
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 flex items-center gap-2 mb-3">
              <Inbox className="w-4 h-4" />
              Configuração IMAP (para aquecimento interno)
            </h3>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Host IMAP</label>
              <input
                type="text"
                value={form.imap_host}
                onChange={(e) => setForm({ ...form, imap_host: e.target.value })}
                className="input"
                placeholder="imap.servidor.com"
              />
            </div>
            <div>
              <label className="label">Porta IMAP</label>
              <input
                type="number"
                value={form.imap_port}
                onChange={(e) => setForm({ ...form, imap_port: parseInt(e.target.value) })}
                className="input"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Senha IMAP</label>
              <input
                type="password"
                value={form.imap_password}
                onChange={(e) => setForm({ ...form, imap_password: e.target.value })}
                className="input"
                placeholder="(usar mesma senha SMTP)"
              />
              <p className="text-xs text-gray-500 mt-1">
                Deixe vazio para usar a senha SMTP
              </p>
            </div>
            <div>
              <label className="label">Modo TLS IMAP</label>
              <select
                value={form.imap_tls_mode}
                onChange={(e) => setForm({ ...form, imap_tls_mode: e.target.value })}
                className="input"
              >
                <option value="none">Nenhum (porta 143)</option>
                <option value="starttls">STARTTLS (porta 143)</option>
                <option value="tls">TLS Implícito (porta 993)</option>
              </select>
            </div>
          </div>

          {/* Botão Testar IMAP */}
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={testImapConnection}
              disabled={testingImap}
              className="flex-1 py-2.5 px-4 border-2 border-purple-200 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
            >
              {testingImap ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  Testando IMAP...
                </>
              ) : (
                <>
                  <Inbox className="w-4 h-4" />
                  Testar Conexão IMAP
                </>
              )}
            </button>
            {imapTestResult && (
              <div className={`flex items-center gap-1.5 text-sm ${imapTestResult.success ? 'text-green-600' : 'text-red-600'}`}>
                {imapTestResult.success ? (
                  <CheckCircle2 className="w-5 h-5" />
                ) : (
                  <AlertCircle className="w-5 h-5" />
                )}
                <span className="max-w-[150px] truncate">{imapTestResult.message}</span>
              </div>
            )}
          </div>

          {/* Separador Remetentes */}
          <div className="border-t border-gray-200 dark:border-gray-700 pt-4 mt-2">
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 flex items-center gap-2 mb-3">
              <Send className="w-4 h-4" />
              Remetentes
            </h3>
          </div>

          <div>
            <label className="label">Remetentes (um por linha)</label>
            <textarea
              value={senders}
              onChange={(e) => setSenders(e.target.value)}
              className="input min-h-[120px] font-mono text-sm"
              placeholder={"email@exemplo.com\nemail2@exemplo.com|Nome do Remetente\nemail3@exemplo.com|Outro Nome"}
              rows={5}
            />
            <p className="text-xs text-gray-500 mt-1">
              Formato: email ou email|nome (ex: contato@empresa.com|Empresa XYZ)
            </p>
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

// Modal para enviar email de teste (estilo Mumara - simples)
function SendTestModal({ smtp, onClose }) {
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSendTest = async (e) => {
    e.preventDefault()
    if (!email.trim()) {
      toast.error('Digite um email de destino')
      return
    }

    setLoading(true)
    try {
      await api.post(`/smtp/${smtp.id}/send-test`, {
        to: email.trim()
      })
      toast.success('Email de teste enviado com sucesso!')
      onClose()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Falha ao enviar email de teste')
      if (error.response?.data?.details) {
        toast.error(error.response.data.details)
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white dark:bg-gray-800 rounded-2xl w-full max-w-md shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-gray-100 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-green-100 dark:bg-green-900/30 rounded-xl">
              <Send className="w-6 h-6 text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900 dark:text-white">Enviar Email de Teste</h2>
              <p className="text-sm text-gray-500 dark:text-gray-400">{smtp.name}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        <form onSubmit={handleSendTest} className="p-6">
          {/* Email de Destino */}
          <div className="mb-6">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Email de Destino
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-4 py-3 border border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white rounded-xl focus:ring-2 focus:ring-green-500 focus:border-transparent outline-none transition-all text-lg"
              placeholder="seu@email.com"
              autoFocus
              required
            />
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-2">
              Um email de teste sera enviado usando este servidor SMTP.
            </p>
          </div>

          {/* Info Box */}
          <div className="bg-gray-50 dark:bg-gray-700/50 rounded-xl p-4 mb-6">
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-500 dark:text-gray-400">Servidor:</span>
                <span className="font-medium text-gray-700 dark:text-gray-300">{smtp.host}:{smtp.port}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500 dark:text-gray-400">TLS:</span>
                <span className="font-medium text-gray-700 dark:text-gray-300">{smtp.tls_mode === 'tls' ? 'TLS' : smtp.tls_mode === 'starttls' ? 'STARTTLS' : 'Nenhum'}</span>
              </div>
            </div>
          </div>

          {/* Buttons */}
          <div className="flex gap-3">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 px-4 py-3 border border-gray-200 dark:border-gray-600 text-gray-700 dark:text-gray-300 rounded-xl font-medium hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={loading}
              className="flex-1 px-4 py-3 bg-green-600 text-white rounded-xl font-medium hover:bg-green-700 transition-colors flex items-center justify-center gap-2 disabled:opacity-50"
            >
              {loading ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin" />
                  Enviando...
                </>
              ) : (
                <>
                  <Send className="w-5 h-5" />
                  Enviar Teste
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function SMTPServers() {
  const [smtps, setSMTPs] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, smtp: null })
  const [testModal, setTestModal] = useState({ open: false, smtp: null })
  const [testing, setTesting] = useState(null)

  useEffect(() => {
    fetchSMTPs()
  }, [])

  const fetchSMTPs = async () => {
    try {
      const response = await api.get('/smtp')
      setSMTPs(response.data.data || [])
    } catch (error) {
      toast.error('Erro ao carregar SMTPs')
    } finally {
      setLoading(false)
    }
  }

  const testSMTP = async (id) => {
    setTesting(id)
    try {
      await api.post(`/smtp/${id}/test`)
      toast.success('Conexão OK!')
      fetchSMTPs()
    } catch (error) {
      toast.error(error.response?.data?.details || 'Falha na conexão')
    } finally {
      setTesting(null)
    }
  }

  const deleteSMTP = async (id) => {
    if (!confirm('Tem certeza que deseja excluir este SMTP?')) return

    try {
      await api.delete(`/smtp/${id}`)
      toast.success('SMTP excluído')
      fetchSMTPs()
    } catch (error) {
      toast.error('Erro ao excluir')
    }
  }

  const refreshSMTPs = async () => {
    try {
      await api.post('/smtp/refresh')
      toast.success('SMTPs recarregados no engine')
    } catch (error) {
      toast.error('Erro ao recarregar')
    }
  }

  const getStatusBadge = (status) => {
    switch (status) {
      case 'online':
        return <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-green-100 text-green-700"><span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>Online</span>
      case 'offline':
        return <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-red-100 text-red-700"><span className="w-1.5 h-1.5 bg-red-500 rounded-full"></span>Offline</span>
      case 'error':
        return <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-yellow-100 text-yellow-700"><span className="w-1.5 h-1.5 bg-yellow-500 rounded-full"></span>Erro</span>
      default:
        return <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-600"><span className="w-1.5 h-1.5 bg-gray-400 rounded-full"></span>Desconhecido</span>
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Servidores SMTP</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Gerencie seus servidores de envio de email</p>
        </div>
        <div className="flex gap-3">
          <button onClick={refreshSMTPs} className="btn btn-secondary flex items-center gap-2">
            <RefreshCw className="w-4 h-4" />
            Recarregar Engine
          </button>
          <button
            onClick={() => setModal({ open: true, smtp: null })}
            className="btn btn-primary flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Novo SMTP
          </button>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-2xl shadow-sm border border-gray-100 dark:border-gray-700">
        {loading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : smtps.length === 0 ? (
          <div className="text-center py-12 text-gray-500 dark:text-gray-400">
            <Server className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>Nenhum servidor SMTP cadastrado</p>
          </div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-100 dark:border-gray-700">
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Nome</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Host</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">TLS</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Status</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Limite</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Enviados</th>
                <th className="text-center py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Ativo</th>
                <th className="text-right py-4 px-6 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">Acoes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-50 dark:divide-gray-700">
              {smtps.map((smtp) => (
                <tr key={smtp.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
                  <td className="py-4 px-6">
                    <span className="font-medium text-gray-900 dark:text-white">{smtp.name}</span>
                  </td>
                  <td className="py-4 px-6 text-gray-600 dark:text-gray-400">
                    {smtp.host}:{smtp.port}
                  </td>
                  <td className="py-4 px-6 text-gray-600 dark:text-gray-400 text-sm">
                    {smtp.tls_mode === 'tls' ? 'TLS' : smtp.tls_mode === 'starttls' ? 'STARTTLS' : 'Nenhum'}
                  </td>
                  <td className="py-4 px-6">{getStatusBadge(smtp.status)}</td>
                  <td className="py-4 px-6 text-gray-600 dark:text-gray-400">
                    {smtp.max_per_minute}/min
                  </td>
                  <td className="py-4 px-6">
                    <span className="font-semibold text-gray-700 dark:text-gray-300">{smtp.total_sent?.toLocaleString()}</span>
                  </td>
                  <td className="py-4 px-6 text-center">
                    {smtp.active ? (
                      <CheckCircle className="w-5 h-5 text-green-500 mx-auto" />
                    ) : (
                      <XCircle className="w-5 h-5 text-gray-400 mx-auto" />
                    )}
                  </td>
                  <td className="py-4 px-6">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        onClick={() => testSMTP(smtp.id)}
                        disabled={testing === smtp.id}
                        className="p-2 text-gray-500 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors"
                        title="Testar conexao"
                      >
                        {testing === smtp.id ? (
                          <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                          <TestTube className="w-4 h-4" />
                        )}
                      </button>
                      <button
                        onClick={() => setTestModal({ open: true, smtp })}
                        className="p-2 text-gray-500 hover:text-green-600 hover:bg-green-50 rounded-lg transition-colors"
                        title="Enviar email de teste"
                      >
                        <Send className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => setModal({ open: true, smtp })}
                        className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg transition-colors"
                        title="Editar"
                      >
                        <Edit className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => deleteSMTP(smtp.id)}
                        className="p-2 text-gray-500 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
                        title="Excluir"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {modal.open && (
        <SMTPModal
          smtp={modal.smtp}
          onClose={() => setModal({ open: false, smtp: null })}
          onSave={() => {
            setModal({ open: false, smtp: null })
            fetchSMTPs()
          }}
        />
      )}

      {testModal.open && (
        <SendTestModal
          smtp={testModal.smtp}
          onClose={() => setTestModal({ open: false, smtp: null })}
        />
      )}
    </div>
  )
}

export default SMTPServers
