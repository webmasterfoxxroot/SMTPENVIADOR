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
  Server
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
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      if (smtp?.id) {
        await api.put(`/smtp/${smtp.id}`, form)
        toast.success('SMTP atualizado!')
      } else {
        await api.post('/smtp', form)
        toast.success('SMTP criado!')
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

function SMTPServers() {
  const [smtps, setSMTPs] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, smtp: null })
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
        return <span className="badge badge-success">Online</span>
      case 'offline':
        return <span className="badge badge-danger">Offline</span>
      case 'error':
        return <span className="badge badge-warning">Erro</span>
      default:
        return <span className="badge badge-gray">Desconhecido</span>
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Servidores SMTP</h1>
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

      <div className="card">
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
          <table className="table">
            <thead>
              <tr>
                <th>Nome</th>
                <th>Host</th>
                <th>TLS</th>
                <th>Status</th>
                <th>Limite</th>
                <th>Enviados</th>
                <th>Ativo</th>
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              {smtps.map((smtp) => (
                <tr key={smtp.id}>
                  <td className="font-medium">{smtp.name}</td>
                  <td className="text-gray-600">
                    {smtp.host}:{smtp.port}
                  </td>
                  <td className="text-gray-600 text-sm">
                    {smtp.tls_mode === 'tls' ? 'TLS' : smtp.tls_mode === 'starttls' ? 'STARTTLS' : 'Nenhum'}
                  </td>
                  <td>{getStatusBadge(smtp.status)}</td>
                  <td className="text-gray-600">
                    {smtp.max_per_minute}/min
                  </td>
                  <td className="text-gray-600">
                    {smtp.total_sent?.toLocaleString()}
                  </td>
                  <td>
                    {smtp.active ? (
                      <CheckCircle className="w-5 h-5 text-green-500" />
                    ) : (
                      <XCircle className="w-5 h-5 text-gray-400" />
                    )}
                  </td>
                  <td>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => testSMTP(smtp.id)}
                        disabled={testing === smtp.id}
                        className="p-2 text-blue-600 hover:bg-blue-50 rounded"
                        title="Testar conexão"
                      >
                        {testing === smtp.id ? (
                          <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                          <TestTube className="w-4 h-4" />
                        )}
                      </button>
                      <button
                        onClick={() => setModal({ open: true, smtp })}
                        className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                        title="Editar"
                      >
                        <Edit className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => deleteSMTP(smtp.id)}
                        className="p-2 text-red-600 hover:bg-red-50 rounded"
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
    </div>
  )
}

export default SMTPServers
