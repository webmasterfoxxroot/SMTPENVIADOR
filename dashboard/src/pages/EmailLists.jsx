import { useState, useEffect, useRef } from 'react'
import { Plus, Edit, Trash2, Upload, Users, Loader2, Mail, CheckCircle, XCircle, AlertTriangle, X, Split, Download, FolderOpen, ChevronDown, ChevronRight, Folder, FolderPlus, Eye, UserPlus, Search } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

// Modal para visualizar/editar/adicionar emails de uma lista
function EmailsModal({ list, onClose, onRefresh }) {
  const [emails, setEmails] = useState([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [editingEmail, setEditingEmail] = useState(null)
  const [addingEmail, setAddingEmail] = useState(false)
  const [form, setForm] = useState({ email: '', name: '', custom1: '', custom2: '', custom3: '', custom4: '', custom5: '' })
  const [saving, setSaving] = useState(false)
  const limit = 50

  useEffect(() => {
    fetchEmails()
  }, [page, search])

  const fetchEmails = async () => {
    setLoading(true)
    try {
      const response = await api.get(`/lists/${list.id}/emails`, {
        params: { page, limit, search }
      })
      setEmails(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Erro ao carregar emails')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (emailId) => {
    if (!confirm('Tem certeza que deseja excluir este email?')) return
    try {
      await api.delete(`/lists/${list.id}/emails/${emailId}`)
      toast.success('Email excluído')
      fetchEmails()
      onRefresh()
    } catch (error) {
      toast.error('Erro ao excluir email')
    }
  }

  const handleEdit = (email) => {
    setEditingEmail(email)
    setForm({
      email: email.email,
      name: email.name || '',
      custom1: email.custom1 || '',
      custom2: email.custom2 || '',
      custom3: email.custom3 || '',
      custom4: email.custom4 || '',
      custom5: email.custom5 || ''
    })
    setAddingEmail(false)
  }

  const handleAdd = () => {
    setEditingEmail(null)
    setForm({ email: '', name: '', custom1: '', custom2: '', custom3: '', custom4: '', custom5: '' })
    setAddingEmail(true)
  }

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      if (addingEmail) {
        await api.post(`/lists/${list.id}/emails`, form)
        toast.success('Email adicionado!')
      } else {
        await api.put(`/lists/${list.id}/emails/${editingEmail.id}`, form)
        toast.success('Email atualizado!')
      }
      setEditingEmail(null)
      setAddingEmail(false)
      setForm({ email: '', name: '', custom1: '', custom2: '', custom3: '', custom4: '', custom5: '' })
      fetchEmails()
      onRefresh()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  const totalPages = Math.ceil(total / limit)

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl w-full max-w-5xl max-h-[90vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b dark:border-gray-700">
          <div>
            <h2 className="text-xl font-bold text-gray-900 dark:text-white">Emails da Lista</h2>
            <p className="text-sm text-gray-500 dark:text-gray-400">{list.name} - {total.toLocaleString()} emails</p>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg text-gray-500 dark:text-gray-400">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Search & Add */}
        <div className="flex items-center gap-3 p-4 border-b dark:border-gray-700">
          <div className="flex-1 relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              value={search}
              onChange={(e) => { setSearch(e.target.value); setPage(1) }}
              placeholder="Buscar email..."
              className="input pl-10"
            />
          </div>
          <button onClick={handleAdd} className="btn btn-primary flex items-center gap-2">
            <UserPlus className="w-4 h-4" />
            Adicionar Email
          </button>
        </div>

        {/* Form for Add/Edit */}
        {(addingEmail || editingEmail) && (
          <div className="p-4 bg-blue-50 dark:bg-blue-900/30 border-b dark:border-gray-700">
            <form onSubmit={handleSave} className="space-y-3">
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <div>
                  <label className="label text-xs">Email *</label>
                  <input
                    type="email"
                    value={form.email}
                    onChange={(e) => setForm({ ...form, email: e.target.value })}
                    className="input"
                    required
                  />
                </div>
                <div>
                  <label className="label text-xs">Nome</label>
                  <input
                    type="text"
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label text-xs">Custom 1</label>
                  <input
                    type="text"
                    value={form.custom1}
                    onChange={(e) => setForm({ ...form, custom1: e.target.value })}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label text-xs">Custom 2</label>
                  <input
                    type="text"
                    value={form.custom2}
                    onChange={(e) => setForm({ ...form, custom2: e.target.value })}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label text-xs">Custom 3</label>
                  <input
                    type="text"
                    value={form.custom3}
                    onChange={(e) => setForm({ ...form, custom3: e.target.value })}
                    className="input"
                  />
                </div>
                <div>
                  <label className="label text-xs">Custom 4</label>
                  <input
                    type="text"
                    value={form.custom4}
                    onChange={(e) => setForm({ ...form, custom4: e.target.value })}
                    className="input"
                  />
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => { setEditingEmail(null); setAddingEmail(false) }}
                  className="btn btn-secondary"
                >
                  Cancelar
                </button>
                <button type="submit" disabled={saving} className="btn btn-primary">
                  {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : (addingEmail ? 'Adicionar' : 'Salvar')}
                </button>
              </div>
            </form>
          </div>
        )}

        {/* Email List */}
        <div className="flex-1 overflow-auto">
          {loading ? (
            <div className="flex justify-center py-12">
              <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
            </div>
          ) : emails.length === 0 ? (
            <div className="text-center py-12 text-gray-500 dark:text-gray-400">
              <Mail className="w-12 h-12 mx-auto mb-4 opacity-50" />
              <p>Nenhum email encontrado</p>
            </div>
          ) : (
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700 sticky top-0">
                <tr>
                  <th className="text-left p-3 text-xs font-semibold text-gray-600 dark:text-gray-300">EMAIL</th>
                  <th className="text-left p-3 text-xs font-semibold text-gray-600 dark:text-gray-300">NOME</th>
                  <th className="text-left p-3 text-xs font-semibold text-gray-600 dark:text-gray-300 hidden md:table-cell">CUSTOM1</th>
                  <th className="text-left p-3 text-xs font-semibold text-gray-600 dark:text-gray-300 hidden lg:table-cell">CUSTOM2</th>
                  <th className="text-center p-3 text-xs font-semibold text-gray-600 dark:text-gray-300">STATUS</th>
                  <th className="text-center p-3 text-xs font-semibold text-gray-600 dark:text-gray-300">AÇÕES</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
                {emails.map(email => (
                  <tr key={email.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="p-3 text-sm text-gray-900 dark:text-white">{email.email}</td>
                    <td className="p-3 text-sm text-gray-600 dark:text-gray-400">{email.name || '-'}</td>
                    <td className="p-3 text-sm text-gray-500 dark:text-gray-400 hidden md:table-cell">{email.custom1 || '-'}</td>
                    <td className="p-3 text-sm text-gray-500 dark:text-gray-400 hidden lg:table-cell">{email.custom2 || '-'}</td>
                    <td className="p-3 text-center">
                      {email.valid ? (
                        <span className="inline-flex items-center gap-1 text-green-600 text-xs">
                          <CheckCircle className="w-3 h-3" /> Válido
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-red-500 text-xs">
                          <XCircle className="w-3 h-3" /> Inválido
                        </span>
                      )}
                    </td>
                    <td className="p-3 text-center">
                      <div className="flex items-center justify-center gap-1">
                        <button
                          onClick={() => handleEdit(email)}
                          className="p-1.5 text-gray-500 dark:text-gray-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/30 rounded"
                          title="Editar"
                        >
                          <Edit className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handleDelete(email.id)}
                          className="p-1.5 text-gray-500 dark:text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 rounded"
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

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between p-4 border-t dark:border-gray-700">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              Página {page} de {totalPages}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn btn-secondary text-sm"
              >
                Anterior
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="btn btn-secondary text-sm"
              >
                Próxima
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// Modal para criar/editar grupo
function GroupModal({ group, onClose, onSave }) {
  const [form, setForm] = useState({
    name: group?.name || '',
    description: group?.description || '',
    color: group?.color || '#3B82F6'
  })
  const [loading, setLoading] = useState(false)

  const colors = ['#3B82F6', '#10B981', '#F59E0B', '#EF4444', '#8B5CF6', '#EC4899', '#06B6D4', '#84CC16']

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      if (group?.id) {
        await api.put(`/groups/${group.id}`, form)
        toast.success('Grupo atualizado!')
      } else {
        await api.post('/groups', form)
        toast.success('Grupo criado!')
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
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4 text-gray-900 dark:text-white">
          {group?.id ? 'Editar Grupo' : 'Novo Grupo'}
        </h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="label">Nome</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="input"
              placeholder="Ex: Listas Boas"
              required
            />
          </div>
          <div>
            <label className="label">Descricao</label>
            <input
              type="text"
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              className="input"
              placeholder="Opcional..."
            />
          </div>
          <div>
            <label className="label">Cor</label>
            <div className="flex gap-2 flex-wrap">
              {colors.map(c => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setForm({ ...form, color: c })}
                  className={`w-8 h-8 rounded-full border-2 ${form.color === c ? 'border-gray-800 dark:border-white scale-110' : 'border-transparent'}`}
                  style={{ backgroundColor: c }}
                />
              ))}
            </div>
          </div>
          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancelar</button>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Salvar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Modal para mover lista para grupo
function MoveToGroupModal({ list, groups, onClose, onSave }) {
  const [selectedGroup, setSelectedGroup] = useState(list.group_id || '')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      await api.put(`/lists/${list.id}/group`, { group_id: selectedGroup || null })
      toast.success('Lista movida!')
      onSave()
    } catch (error) {
      toast.error('Erro ao mover lista')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4 text-gray-900 dark:text-white">Mover "{list.name}"</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="label">Selecione o grupo</label>
            <select
              value={selectedGroup}
              onChange={(e) => setSelectedGroup(e.target.value)}
              className="input"
            >
              <option value="">Sem grupo</option>
              {groups.map(g => (
                <option key={g.id} value={g.id}>{g.name}</option>
              ))}
            </select>
          </div>
          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancelar</button>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : 'Mover'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Modal para upload com divisão em múltiplas listas
function SplitUploadModal({ groups, onClose, onUploadStarted, onCreateGroup }) {
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [hasHeader, setHasHeader] = useState(false)
  const [delimiter, setDelimiter] = useState(',')
  const [baseName, setBaseName] = useState('')
  const [numParts, setNumParts] = useState(5)
  const [groupId, setGroupId] = useState('')

  const handleUpload = async (e) => {
    e.preventDefault()
    if (!file || !baseName) return

    setUploading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('has_header', hasHeader)
    formData.append('delimiter', delimiter)
    formData.append('base_name', baseName)
    formData.append('num_parts', numParts)
    if (groupId) {
      formData.append('group_id', groupId)
    }

    try {
      const response = await api.post('/lists/upload-split', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 300000
      })

      toast.success(`${numParts} listas criadas! Importação iniciada...`)
      onUploadStarted(response.data.jobs || [])
      onClose()
    } catch (error) {
      console.error('Upload error:', error)
      const errorMsg = error.response?.data?.error || error.message || 'Erro no upload'
      toast.error(errorMsg)
      setUploading(false)
    }
  }

  const estimatedPerPart = file ? Math.ceil(file.size / numParts / 30) : 0

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-bold flex items-center gap-2 text-gray-900 dark:text-white">
            <Split className="w-5 h-5 text-blue-500" />
            Upload com Divisão
          </h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
            <X className="w-5 h-5" />
          </button>
        </div>

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
            {file && (
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                {file.name} ({(file.size / 1024 / 1024).toFixed(2)} MB)
              </p>
            )}
          </div>

          <div>
            <label className="label">Nome Base das Listas</label>
            <input
              type="text"
              value={baseName}
              onChange={(e) => setBaseName(e.target.value)}
              className="input"
              placeholder="Ex: MINHA LISTA"
              required
            />
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
              Será criado: {baseName || 'LISTA'} 01, {baseName || 'LISTA'} 02, ...
            </p>
          </div>

          <div>
            <label className="label">Grupo (opcional)</label>
            <div className="flex gap-2">
              <select
                value={groupId}
                onChange={(e) => setGroupId(e.target.value)}
                className="input flex-1"
              >
                <option value="">Sem grupo</option>
                {groups.map(g => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
              <button
                type="button"
                onClick={onCreateGroup}
                className="btn btn-secondary px-3"
                title="Criar novo grupo"
              >
                <FolderPlus className="w-4 h-4" />
              </button>
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
              Todas as listas criadas serão adicionadas a este grupo
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Dividir em</label>
              <input
                type="number"
                value={numParts}
                onChange={(e) => setNumParts(Math.max(2, Math.min(50, parseInt(e.target.value) || 2)))}
                className="input"
                min="2"
                max="50"
              />
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">partes (2-50)</p>
            </div>
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
                <option value=" ">Espaço</option>
              </select>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="hasHeader"
              checked={hasHeader}
              onChange={(e) => setHasHeader(e.target.checked)}
              className="w-4 h-4"
            />
            <label htmlFor="hasHeader" className="text-sm cursor-pointer text-gray-700 dark:text-gray-300">
              Arquivo tem cabeçalho
            </label>
          </div>

          {file && (
            <div className="p-3 bg-blue-50 dark:bg-blue-900/30 rounded-lg text-sm">
              <p className="font-medium text-blue-800 dark:text-blue-300 mb-2">Estimativa:</p>
              <ul className="text-blue-700 dark:text-blue-400 space-y-1">
                <li>~{estimatedPerPart.toLocaleString()} emails por lista</li>
                <li>{numParts} listas serão criadas</li>
                {groupId && <li>Grupo: {groups.find(g => g.id === groupId)?.name}</li>}
              </ul>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">
              Cancelar
            </button>
            <button type="submit" disabled={uploading || !file || !baseName} className="btn btn-primary">
              {uploading ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin mr-2" />
                  Dividindo...
                </>
              ) : (
                'Dividir e Importar'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function UploadModal({ listId, onClose, onUploadStarted }) {
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [hasHeader, setHasHeader] = useState(false)
  const [delimiter, setDelimiter] = useState(',')

  const handleUpload = async (e) => {
    e.preventDefault()
    if (!file) return

    setUploading(true)
    const formData = new FormData()
    formData.append('file', file)
    formData.append('has_header', hasHeader)
    formData.append('delimiter', delimiter)

    try {
      const response = await api.post(`/lists/${listId}/upload-async`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 300000
      })

      toast.success('Upload iniciado! Processando em background...')
      onUploadStarted(listId, response.data.job_id)
      onClose()
    } catch (error) {
      console.error('Upload error:', error)
      const errorMsg = error.response?.data?.error || error.message || 'Erro no upload'
      toast.error(errorMsg)
      setUploading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-bold text-gray-900 dark:text-white">Upload de Emails</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
            <X className="w-5 h-5" />
          </button>
        </div>

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
            {file && (
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                {file.name} ({(file.size / 1024 / 1024).toFixed(2)} MB)
              </p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Delimitador</label>
              <select
                value={delimiter}
                onChange={(e) => setDelimiter(e.target.value)}
                className="input"
              >
                <option value=",">Virgula (,)</option>
                <option value=";">Ponto e virgula (;)</option>
                <option value="\t">Tab</option>
                <option value=" ">Espaco</option>
              </select>
            </div>
            <div className="flex items-end">
              <label className="flex items-center gap-2 cursor-pointer pb-2 text-gray-700 dark:text-gray-300">
                <input
                  type="checkbox"
                  checked={hasHeader}
                  onChange={(e) => setHasHeader(e.target.checked)}
                  className="w-4 h-4"
                />
                Tem cabecalho
              </label>
            </div>
          </div>

          <div className="p-3 bg-blue-50 dark:bg-blue-900/30 rounded-lg text-sm text-blue-800 dark:text-blue-300">
            <p className="font-medium mb-1">Upload em background:</p>
            <ul className="list-disc list-inside space-y-1 text-blue-700 dark:text-blue-400">
              <li>Suporta arquivos grandes (milhoes de emails)</li>
              <li>Progresso mostrado na lista</li>
              <li>Valida e remove duplicados automaticamente</li>
            </ul>
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button type="button" onClick={onClose} className="btn btn-secondary">
              Cancelar
            </button>
            <button type="submit" disabled={uploading || !file} className="btn btn-primary">
              {uploading ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin mr-2" />
                  Enviando...
                </>
              ) : (
                'Iniciar Upload'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ListModal({ list, groups, onClose, onSave, onCreateGroup }) {
  const [form, setForm] = useState({
    name: '',
    description: '',
    group_id: '',
    ...list
  })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      const payload = {
        ...form,
        group_id: form.group_id || null
      }
      if (list?.id) {
        await api.put(`/lists/${list.id}`, payload)
        // If group changed, update it
        if (form.group_id !== list.group_id) {
          await api.put(`/lists/${list.id}/group`, { group_id: form.group_id || null })
        }
        toast.success('Lista atualizada!')
      } else {
        const response = await api.post('/lists', payload)
        // If group selected, move to group
        if (form.group_id && response.data.id) {
          await api.put(`/lists/${response.data.id}/group`, { group_id: form.group_id })
        }
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
      <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
        <h2 className="text-xl font-bold mb-4 text-gray-900 dark:text-white">
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

          <div>
            <label className="label">Grupo</label>
            <div className="flex gap-2">
              <select
                value={form.group_id || ''}
                onChange={(e) => setForm({ ...form, group_id: e.target.value })}
                className="input flex-1"
              >
                <option value="">Sem grupo</option>
                {groups.map(g => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
              <button
                type="button"
                onClick={onCreateGroup}
                className="btn btn-secondary px-3"
                title="Criar novo grupo"
              >
                <FolderPlus className="w-4 h-4" />
              </button>
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

// List Row - Horizontal compact layout
function ListRow({ list, importJob, onEdit, onDelete, onUpload, onCancelImport, onDownload, onMoveToGroup, onViewEmails }) {
  const isImporting = importJob && (importJob.status === 'pending' || importJob.status === 'processing')
  const isDeleting = list.status === 'deleting'

  // Calculate stats
  const totalEmails = list.total_emails || 0
  const validEmails = list.valid_emails || 0
  const invalidEmails = list.invalid_emails || 0
  const duplicates = importJob?.duplicates || 0
  const validPercent = totalEmails > 0 ? ((validEmails / totalEmails) * 100).toFixed(1) : 0

  return (
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg px-4 py-3 hover:shadow-md transition-shadow">
      {/* Main Row */}
      <div className="flex items-center gap-4">
        {/* Name & Description */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold text-gray-900 dark:text-white truncate">{list.name}</h3>
            {list.description && (
              <span className="text-xs text-gray-400 dark:text-gray-500 truncate hidden sm:inline">({list.description})</span>
            )}
          </div>
        </div>

        {/* Stats - hide when importing */}
        {!isImporting && (
          <div className="hidden md:flex items-center gap-4 text-sm">
            <div className="flex items-center gap-1 text-gray-600 dark:text-gray-400">
              <Users className="w-4 h-4" />
              <span className="font-medium">{totalEmails.toLocaleString()}</span>
            </div>
            {totalEmails > 0 && (
              <>
                <div className="flex items-center gap-1 text-green-600 dark:text-green-500" title="Válidos">
                  <CheckCircle className="w-4 h-4" />
                  <span>{validEmails.toLocaleString()}</span>
                  <span className="text-xs text-gray-400 dark:text-gray-500">({validPercent}%)</span>
                </div>
                {invalidEmails > 0 && (
                  <div className="flex items-center gap-1 text-red-500" title="Inválidos">
                    <XCircle className="w-4 h-4" />
                    <span>{invalidEmails.toLocaleString()}</span>
                  </div>
                )}
                {duplicates > 0 && (
                  <div className="flex items-center gap-1 text-yellow-600 dark:text-yellow-500" title="Duplicados">
                    <AlertTriangle className="w-4 h-4" />
                    <span>{duplicates.toLocaleString()}</span>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* Import Progress */}
        {isImporting && (
          <div className="flex-1 max-w-md">
            <div className="flex items-center gap-3">
              <div className="flex-1">
                <div className="w-full h-2 bg-blue-100 dark:bg-blue-900/50 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-blue-500 rounded-full transition-all duration-300"
                    style={{ width: `${importJob.progress || 0}%` }}
                  />
                </div>
              </div>
              <span className="text-sm font-medium text-blue-600 dark:text-blue-400 w-12">{importJob.progress || 0}%</span>
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {(importJob.processed || 0).toLocaleString()}/{(importJob.total_lines || 0).toLocaleString()}
              </div>
            </div>
          </div>
        )}

        {/* Status Badge */}
        <span className={`px-2 py-1 rounded-full text-xs font-medium whitespace-nowrap ${
          isImporting ? 'bg-blue-100 dark:bg-blue-900/50 text-blue-700 dark:text-blue-400' :
          isDeleting ? 'bg-red-100 dark:bg-red-900/50 text-red-700 dark:text-red-400' :
          list.status === 'ready' ? 'bg-green-100 dark:bg-green-900/50 text-green-700 dark:text-green-400' :
          list.status === 'pending' ? 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400' :
          'bg-yellow-100 dark:bg-yellow-900/50 text-yellow-700 dark:text-yellow-400'
        }`}>
          {isImporting ? 'Importando' :
           isDeleting ? 'Excluindo' :
           list.status === 'ready' ? 'Pronta' :
           list.status === 'pending' ? 'Pendente' :
           'Processando'}
        </span>

        {/* Actions */}
        <div className="flex items-center gap-1">
          <button
            onClick={onViewEmails}
            className="p-2 text-gray-500 dark:text-gray-400 hover:text-cyan-600 hover:bg-cyan-50 dark:hover:bg-cyan-900/30 rounded-lg transition-colors"
            disabled={isImporting || isDeleting}
            title="Ver/Editar Emails"
          >
            <Eye className="w-4 h-4" />
          </button>
          <button
            onClick={onEdit}
            className="p-2 text-gray-500 dark:text-gray-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/30 rounded-lg transition-colors"
            disabled={isImporting || isDeleting}
            title="Editar Lista"
          >
            <Edit className="w-4 h-4" />
          </button>
          <button
            onClick={onDelete}
            className="p-2 text-gray-500 dark:text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-lg transition-colors"
            disabled={isImporting || isDeleting}
            title="Excluir"
          >
            <Trash2 className="w-4 h-4" />
          </button>
          {isImporting ? (
            <button
              onClick={() => onCancelImport(importJob.id)}
              className="p-2 text-red-500 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-lg transition-colors"
              title="Cancelar importação"
            >
              <X className="w-4 h-4" />
            </button>
          ) : (
            <>
              <button
                onClick={onUpload}
                className="p-2 text-gray-500 dark:text-gray-400 hover:text-green-600 hover:bg-green-50 dark:hover:bg-green-900/30 rounded-lg transition-colors"
                disabled={isDeleting}
                title="Upload de emails"
              >
                <Upload className="w-4 h-4" />
              </button>
              <button
                onClick={() => {
                  const token = localStorage.getItem('smtpenviador_token')
                  window.open(`/api/v1/lists/${list.id}/download?format=csv&token=${token}`, '_blank')
                }}
                className="p-2 text-gray-500 dark:text-gray-400 hover:text-purple-600 hover:bg-purple-50 dark:hover:bg-purple-900/30 rounded-lg transition-colors"
                title="Download da lista"
              >
                <Download className="w-4 h-4" />
              </button>
              <button
                onClick={onMoveToGroup}
                className="p-2 text-gray-500 dark:text-gray-400 hover:text-orange-600 hover:bg-orange-50 dark:hover:bg-orange-900/30 rounded-lg transition-colors"
                disabled={isDeleting}
                title="Mover para grupo"
              >
                <FolderOpen className="w-4 h-4" />
              </button>
            </>
          )}
        </div>
      </div>

      {/* Mobile Stats - show below on small screens */}
      {!isImporting && totalEmails > 0 && (
        <div className="flex md:hidden items-center gap-3 mt-2 pt-2 border-t border-gray-100 dark:border-gray-700 text-xs">
          <span className="text-gray-600 dark:text-gray-400">{totalEmails.toLocaleString()} emails</span>
          <span className="text-green-600 dark:text-green-500">{validEmails.toLocaleString()} válidos</span>
          {invalidEmails > 0 && <span className="text-red-500">{invalidEmails.toLocaleString()} inválidos</span>}
          {duplicates > 0 && <span className="text-yellow-600 dark:text-yellow-500">{duplicates.toLocaleString()} duplicados</span>}
        </div>
      )}

      {/* Import details on small screens */}
      {isImporting && (
        <div className="flex md:hidden items-center gap-2 mt-2 pt-2 border-t border-gray-100 dark:border-gray-700 text-xs text-gray-500 dark:text-gray-400">
          <span className="text-green-600 dark:text-green-500">+{(importJob.valid || 0).toLocaleString()} válidos</span>
          {importJob.invalid > 0 && <span className="text-red-500">{importJob.invalid} inválidos</span>}
          {importJob.duplicates > 0 && <span className="text-yellow-600 dark:text-yellow-500">{importJob.duplicates} duplicados</span>}
        </div>
      )}
    </div>
  )
}

function EmailLists() {
  const [lists, setLists] = useState([])
  const [groups, setGroups] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, list: null })
  const [uploadModal, setUploadModal] = useState({ open: false, listId: null })
  const [splitModal, setSplitModal] = useState(false)
  const [groupModal, setGroupModal] = useState({ open: false, group: null })
  const [moveModal, setMoveModal] = useState({ open: false, list: null })
  const [emailsModal, setEmailsModal] = useState({ open: false, list: null })
  const [expandedGroups, setExpandedGroups] = useState({}) // { groupId: true/false }
  const [importJobs, setImportJobs] = useState({}) // { listId: jobStatus }
  const pollIntervals = useRef({})

  useEffect(() => {
    fetchListsAndCheckJobs()
    fetchGroups()
    return () => {
      // Cleanup all poll intervals
      Object.values(pollIntervals.current).forEach(clearInterval)
    }
  }, [])

  const fetchLists = async () => {
    try {
      const response = await api.get('/lists')
      setLists(response.data.data || [])
      return response.data.data || []
    } catch (error) {
      toast.error('Erro ao carregar listas')
      return []
    } finally {
      setLoading(false)
    }
  }

  const fetchGroups = async () => {
    try {
      const response = await api.get('/groups')
      setGroups(response.data.data || [])
      // Expand all groups by default
      const expanded = {}
      ;(response.data.data || []).forEach(g => { expanded[g.id] = true })
      expanded['ungrouped'] = true
      setExpandedGroups(expanded)
    } catch (error) {
      console.log('Error fetching groups:', error)
    }
  }

  const deleteGroup = async (id) => {
    if (!confirm('Excluir este grupo? As listas serao movidas para "Sem grupo".')) return
    try {
      await api.delete(`/groups/${id}`)
      toast.success('Grupo excluido')
      fetchGroups()
      fetchLists()
    } catch (error) {
      toast.error('Erro ao excluir grupo')
    }
  }

  const downloadList = async (listId, listName) => {
    try {
      // Use fetch for better blob handling
      const token = localStorage.getItem('smtpenviador_token')
      const response = await fetch(`/api/v1/lists/${listId}/download?format=csv`, {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      })

      if (!response.ok) {
        throw new Error('Download failed')
      }

      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `${listName}.csv`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(url)
      toast.success('Download concluído!')
    } catch (error) {
      console.error('Download error:', error)
      toast.error('Erro ao baixar lista')
    }
  }

  const toggleGroup = (groupId) => {
    setExpandedGroups(prev => ({ ...prev, [groupId]: !prev[groupId] }))
  }

  // Organize lists by group
  const getListsByGroup = () => {
    const grouped = {}
    const ungrouped = []

    lists.forEach(list => {
      if (list.group_id) {
        if (!grouped[list.group_id]) grouped[list.group_id] = []
        grouped[list.group_id].push(list)
      } else {
        ungrouped.push(list)
      }
    })

    return { grouped, ungrouped }
  }

  // Check for active import jobs on page load
  const fetchListsAndCheckJobs = async () => {
    const listsData = await fetchLists()

    // Check each list for active import jobs
    for (const list of listsData) {
      try {
        const response = await api.get(`/lists/${list.id}/import-jobs`)
        const jobs = response.data?.jobs || []

        // Find active job (pending or processing)
        const activeJob = jobs.find(j => j.status === 'pending' || j.status === 'processing')

        if (activeJob) {
          console.log(`Found active job for list ${list.id}:`, activeJob)
          // Restore job state and start polling
          setImportJobs(prev => ({
            ...prev,
            [list.id]: {
              status: activeJob.status,
              progress: activeJob.progress || 0,
              processed: activeJob.processed || 0,
              total_lines: activeJob.total_lines || 0,
              valid: activeJob.valid || 0,
              invalid: activeJob.invalid || 0,
              duplicates: activeJob.duplicates || 0
            }
          }))
          startJobPolling(list.id, activeJob.id)
        }
      } catch (error) {
        // Ignore errors for individual list job checks
        console.log(`No active jobs for list ${list.id}:`, error.message)
      }
    }
  }

  const deleteList = async (id) => {
    if (!confirm('Tem certeza? Todos os emails serao perdidos.')) return

    try {
      await api.delete(`/lists/${id}`)
      toast.success('Lista excluida')
      fetchLists()
    } catch (error) {
      toast.error('Erro ao excluir')
    }
  }

  const cancelImport = async (listId, jobId) => {
    if (!confirm('Tem certeza que deseja cancelar a importacao?')) return

    try {
      await api.post(`/import-cancel/${jobId}`)
      toast.success('Importacao cancelada')

      // Stop polling
      if (pollIntervals.current[listId]) {
        clearInterval(pollIntervals.current[listId])
        delete pollIntervals.current[listId]
      }

      // Remove from importJobs
      setImportJobs(prev => {
        const newJobs = { ...prev }
        delete newJobs[listId]
        return newJobs
      })

      fetchLists()
    } catch (error) {
      toast.error('Erro ao cancelar importacao')
    }
  }

  const startJobPolling = (listId, jobId) => {
    // Clear any existing poll for this list
    if (pollIntervals.current[listId]) {
      clearInterval(pollIntervals.current[listId])
    }

    // Start polling
    pollIntervals.current[listId] = setInterval(async () => {
      try {
        const response = await api.get(`/import-status/${jobId}`)
        const job = response.data

        setImportJobs(prev => ({
          ...prev,
          [listId]: job
        }))

        if (job.status === 'completed') {
          clearInterval(pollIntervals.current[listId])
          delete pollIntervals.current[listId]
          toast.success(`${job.valid.toLocaleString()} emails importados!`)
          fetchLists() // Refresh list to get updated count

          // Keep completed status for 10 seconds then clear
          setTimeout(() => {
            setImportJobs(prev => {
              const newJobs = { ...prev }
              delete newJobs[listId]
              return newJobs
            })
          }, 10000)
        } else if (job.status === 'failed') {
          clearInterval(pollIntervals.current[listId])
          delete pollIntervals.current[listId]
          toast.error(job.error || 'Erro na importacao')
          setImportJobs(prev => {
            const newJobs = { ...prev }
            delete newJobs[listId]
            return newJobs
          })
        }
      } catch (error) {
        console.error('Poll error:', error)
      }
    }, 1000)
  }

  const handleUploadStarted = (listId, jobId) => {
    setImportJobs(prev => ({
      ...prev,
      [listId]: { status: 'pending', progress: 0, valid: 0, invalid: 0, duplicates: 0 }
    }))
    startJobPolling(listId, jobId)
  }

  const handleSplitUploadStarted = (jobs) => {
    // Start polling for each job
    jobs.forEach(job => {
      setImportJobs(prev => ({
        ...prev,
        [job.list_id]: { status: 'pending', progress: 0, valid: 0, invalid: 0, duplicates: 0, id: job.job_id }
      }))
      startJobPolling(job.list_id, job.job_id)
    })
    fetchLists()
  }

  const { grouped, ungrouped } = getListsByGroup()

  const renderListRow = (list) => (
    <ListRow
      key={list.id}
      list={list}
      importJob={importJobs[list.id]}
      onEdit={() => setModal({ open: true, list })}
      onDelete={() => deleteList(list.id)}
      onUpload={() => setUploadModal({ open: true, listId: list.id })}
      onCancelImport={(jobId) => cancelImport(list.id, jobId)}
      onDownload={() => downloadList(list.id, list.name)}
      onMoveToGroup={() => setMoveModal({ open: true, list })}
      onViewEmails={() => setEmailsModal({ open: true, list })}
    />
  )

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Listas de Emails</h1>
        <div className="flex gap-2">
          <button
            onClick={() => setGroupModal({ open: true, group: null })}
            className="btn btn-secondary flex items-center gap-2"
          >
            <FolderPlus className="w-4 h-4" />
            Novo Grupo
          </button>
          <button
            onClick={() => setSplitModal(true)}
            className="btn btn-secondary flex items-center gap-2"
          >
            <Split className="w-4 h-4" />
            Upload com Divisao
          </button>
          <button
            onClick={() => setModal({ open: true, list: null })}
            className="btn btn-primary flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Nova Lista
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
        </div>
      ) : lists.length === 0 ? (
        <div className="text-center py-12 text-gray-500 dark:text-gray-400">
          <Mail className="w-12 h-12 mx-auto mb-4 opacity-50" />
          <p>Nenhuma lista criada</p>
        </div>
      ) : (
        <div className="space-y-4">
          {/* Groups */}
          {groups.map(group => {
            const groupLists = grouped[group.id] || []
            const isExpanded = expandedGroups[group.id]

            return (
              <div key={group.id} className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
                {/* Group Header */}
                <div
                  className="flex items-center justify-between px-4 py-3 bg-gray-50 dark:bg-gray-700/50 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700"
                  onClick={() => toggleGroup(group.id)}
                >
                  <div className="flex items-center gap-3">
                    {isExpanded ? <ChevronDown className="w-5 h-5 text-gray-500 dark:text-gray-400" /> : <ChevronRight className="w-5 h-5 text-gray-500 dark:text-gray-400" />}
                    <div className="w-3 h-3 rounded-full" style={{ backgroundColor: group.color }} />
                    <span className="font-semibold text-gray-900 dark:text-white">{group.name}</span>
                    <span className="text-sm text-gray-500 dark:text-gray-400">({groupLists.length} listas)</span>
                  </div>
                  <div className="flex items-center gap-1" onClick={e => e.stopPropagation()}>
                    <button
                      onClick={() => setGroupModal({ open: true, group })}
                      className="p-2 text-gray-500 dark:text-gray-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/30 rounded-lg"
                      title="Editar grupo"
                    >
                      <Edit className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => deleteGroup(group.id)}
                      className="p-2 text-gray-500 dark:text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-lg"
                      title="Excluir grupo"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                {/* Group Lists */}
                {isExpanded && groupLists.length > 0 && (
                  <div className="p-2 space-y-2 bg-white dark:bg-gray-800">
                    {groupLists.map(renderListRow)}
                  </div>
                )}
                {isExpanded && groupLists.length === 0 && (
                  <div className="p-4 text-center text-gray-400 dark:text-gray-500 text-sm bg-white dark:bg-gray-800">
                    Nenhuma lista neste grupo
                  </div>
                )}
              </div>
            )
          })}

          {/* Ungrouped Lists */}
          {ungrouped.length > 0 && (
            <div className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
              <div
                className="flex items-center justify-between px-4 py-3 bg-gray-50 dark:bg-gray-700/50 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700"
                onClick={() => toggleGroup('ungrouped')}
              >
                <div className="flex items-center gap-3">
                  {expandedGroups['ungrouped'] ? <ChevronDown className="w-5 h-5 text-gray-500 dark:text-gray-400" /> : <ChevronRight className="w-5 h-5 text-gray-500 dark:text-gray-400" />}
                  <Folder className="w-5 h-5 text-gray-400 dark:text-gray-500" />
                  <span className="font-semibold text-gray-600 dark:text-gray-300">Sem Grupo</span>
                  <span className="text-sm text-gray-500 dark:text-gray-400">({ungrouped.length} listas)</span>
                </div>
              </div>
              {expandedGroups['ungrouped'] && (
                <div className="p-2 space-y-2 bg-white dark:bg-gray-800">
                  {ungrouped.map(renderListRow)}
                </div>
              )}
            </div>
          )}

          {/* No groups - show all lists flat */}
          {groups.length === 0 && ungrouped.length === 0 && lists.length > 0 && (
            <div className="space-y-2">
              {lists.map(renderListRow)}
            </div>
          )}
        </div>
      )}

      {modal.open && (
        <ListModal
          list={modal.list}
          groups={groups}
          onClose={() => setModal({ open: false, list: null })}
          onSave={() => {
            setModal({ open: false, list: null })
            fetchLists()
            fetchGroups()
          }}
          onCreateGroup={() => {
            setModal({ open: false, list: null })
            setGroupModal({ open: true, group: null, returnToList: modal.list })
          }}
        />
      )}

      {uploadModal.open && (
        <UploadModal
          listId={uploadModal.listId}
          onClose={() => setUploadModal({ open: false, listId: null })}
          onUploadStarted={handleUploadStarted}
        />
      )}

      {splitModal && (
        <SplitUploadModal
          groups={groups}
          onClose={() => setSplitModal(false)}
          onUploadStarted={handleSplitUploadStarted}
          onCreateGroup={() => {
            setSplitModal(false)
            setGroupModal({ open: true, group: null, returnToSplit: true })
          }}
        />
      )}

      {groupModal.open && (
        <GroupModal
          group={groupModal.group}
          onClose={() => {
            const returnToList = groupModal.returnToList
            const returnToSplit = groupModal.returnToSplit
            setGroupModal({ open: false, group: null })
            if (returnToList !== undefined) {
              setModal({ open: true, list: returnToList })
            } else if (returnToSplit) {
              setSplitModal(true)
            }
          }}
          onSave={() => {
            const returnToList = groupModal.returnToList
            const returnToSplit = groupModal.returnToSplit
            setGroupModal({ open: false, group: null })
            fetchGroups().then(() => {
              if (returnToList !== undefined) {
                setModal({ open: true, list: returnToList })
              } else if (returnToSplit) {
                setSplitModal(true)
              }
            })
          }}
        />
      )}

      {moveModal.open && (
        <MoveToGroupModal
          list={moveModal.list}
          groups={groups}
          onClose={() => setMoveModal({ open: false, list: null })}
          onSave={() => {
            setMoveModal({ open: false, list: null })
            fetchLists()
          }}
        />
      )}

      {emailsModal.open && (
        <EmailsModal
          list={emailsModal.list}
          onClose={() => setEmailsModal({ open: false, list: null })}
          onRefresh={fetchLists}
        />
      )}
    </div>
  )
}

export default EmailLists
