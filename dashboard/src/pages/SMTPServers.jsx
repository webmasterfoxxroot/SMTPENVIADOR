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
  X
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
    ...smtp
  })
  const [senders, setSenders] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadingSenders, setLoadingSenders] = useState(false)

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
    } catch (error) {
      console.error('Error loading senders:', error)
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
            name: parts[1]?.trim() || ''
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
      <div className="bg-white rounded-xl p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="text-xl font-bold mb-4">
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

// Modal para enviar email de teste (estilo Mumara)
function SendTestModal({ smtp, onClose }) {
  const [form, setForm] = useState({
    to: '',
    from_name: 'Teste SMTP',
    subject: 'Email de Teste - Verificacao do Servidor SMTP',
    body: `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
</head>
<body style="font-family: Arial, sans-serif; background: #f5f5f5; padding: 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: white; border-radius: 10px; padding: 40px; box-shadow: 0 2px 10px rgba(0,0,0,0.1);">
        <h1 style="color: #3b82f6; text-align: center;">Teste de SMTP</h1>
        <div style="background: #10b981; color: white; padding: 15px 25px; border-radius: 8px; text-align: center; font-size: 18px; margin: 20px 0;">
            ✓ Email de teste enviado com sucesso!
        </div>
        <p style="text-align: center; color: #64748b;">
            Este email confirma que seu servidor SMTP esta configurado corretamente e pronto para envios.
        </p>
        <p style="text-align: center; color: #94a3b8; font-size: 12px; margin-top: 30px;">
            SMTP Enviador - Sistema de Email Marketing
        </p>
    </div>
</body>
</html>`
  })
  const [loading, setLoading] = useState(false)

  const handleSendTest = async (e) => {
    e.preventDefault()
    if (!form.to.trim()) {
      toast.error('Digite um email de destino')
      return
    }

    setLoading(true)
    try {
      await api.post(`/smtp/${smtp.id}/send-test`, {
        to: form.to.trim(),
        from_name: form.from_name.trim(),
        subject: form.subject.trim(),
        body: form.body
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
      <div className="bg-white rounded-2xl w-full max-w-2xl shadow-xl max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-gray-100">
          <div className="flex items-center gap-3">
            <div className="p-3 bg-blue-100 rounded-xl">
              <Send className="w-6 h-6 text-blue-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-gray-900">Enviar Email de Teste</h2>
              <p className="text-sm text-gray-500">{smtp.name} - {smtp.host}:{smtp.port}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-100 rounded-lg transition-colors"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        <form onSubmit={handleSendTest} className="p-6 space-y-4">
          {/* Email de Destino */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Email de Destino *
            </label>
            <input
              type="email"
              value={form.to}
              onChange={(e) => setForm({ ...form, to: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              placeholder="seu@email.com"
              autoFocus
              required
            />
          </div>

          {/* Nome do Remetente */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Nome do Remetente
            </label>
            <input
              type="text"
              value={form.from_name}
              onChange={(e) => setForm({ ...form, from_name: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              placeholder="Nome que aparecera no email"
            />
            <p className="text-xs text-gray-400 mt-1">Email remetente: {smtp.username}</p>
          </div>

          {/* Assunto */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Assunto *
            </label>
            <input
              type="text"
              value={form.subject}
              onChange={(e) => setForm({ ...form, subject: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              placeholder="Assunto do email de teste"
              required
            />
          </div>

          {/* Corpo do Email */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Corpo do Email (HTML)
            </label>
            <textarea
              value={form.body}
              onChange={(e) => setForm({ ...form, body: e.target.value })}
              className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all font-mono text-sm"
              rows={8}
              placeholder="<html>...</html>"
            />
          </div>

          {/* Info Box */}
          <div className="bg-blue-50 rounded-xl p-4">
            <h3 className="text-xs font-semibold text-blue-700 uppercase mb-2">Informacoes do Envio</h3>
            <div className="space-y-1 text-sm text-blue-600">
              <p><span className="opacity-70">Servidor:</span> <span className="font-medium">{smtp.host}:{smtp.port}</span></p>
              <p><span className="opacity-70">Usuario:</span> <span className="font-medium">{smtp.username}</span></p>
              <p><span className="opacity-70">TLS:</span> <span className="font-medium">{smtp.tls_mode === 'tls' ? 'TLS Implicito' : smtp.tls_mode === 'starttls' ? 'STARTTLS' : 'Sem criptografia'}</span></p>
            </div>
          </div>

          {/* Buttons */}
          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 px-4 py-3 border border-gray-200 text-gray-700 rounded-xl font-medium hover:bg-gray-50 transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={loading}
              className="flex-1 px-4 py-3 bg-blue-600 text-white rounded-xl font-medium hover:bg-blue-700 transition-colors flex items-center justify-center gap-2 disabled:opacity-50"
            >
              {loading ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin" />
                  Enviando...
                </>
              ) : (
                <>
                  <Send className="w-5 h-5" />
                  Enviar Email de Teste
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
          <h1 className="text-2xl font-bold text-gray-900">Servidores SMTP</h1>
          <p className="text-sm text-gray-500 mt-1">Gerencie seus servidores de envio de email</p>
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

      <div className="bg-white rounded-2xl shadow-sm border border-gray-100">
        {loading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : smtps.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <Server className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>Nenhum servidor SMTP cadastrado</p>
          </div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-100">
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Nome</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Host</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">TLS</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Status</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Limite</th>
                <th className="text-left py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Enviados</th>
                <th className="text-center py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Ativo</th>
                <th className="text-right py-4 px-6 text-xs font-semibold text-gray-500 uppercase tracking-wider">Acoes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-50">
              {smtps.map((smtp) => (
                <tr key={smtp.id} className="hover:bg-gray-50 transition-colors">
                  <td className="py-4 px-6">
                    <span className="font-medium text-gray-900">{smtp.name}</span>
                  </td>
                  <td className="py-4 px-6 text-gray-600">
                    {smtp.host}:{smtp.port}
                  </td>
                  <td className="py-4 px-6 text-gray-600 text-sm">
                    {smtp.tls_mode === 'tls' ? 'TLS' : smtp.tls_mode === 'starttls' ? 'STARTTLS' : 'Nenhum'}
                  </td>
                  <td className="py-4 px-6">{getStatusBadge(smtp.status)}</td>
                  <td className="py-4 px-6 text-gray-600">
                    {smtp.max_per_minute}/min
                  </td>
                  <td className="py-4 px-6">
                    <span className="font-semibold text-gray-700">{smtp.total_sent?.toLocaleString()}</span>
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
