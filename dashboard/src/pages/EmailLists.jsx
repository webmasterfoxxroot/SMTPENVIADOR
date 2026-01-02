import { useState, useEffect, useRef } from 'react'
import { Plus, Edit, Trash2, Upload, Users, Loader2, Mail, CheckCircle, XCircle, AlertTriangle, X } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

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
      <div className="bg-white rounded-xl p-6 w-full max-w-md">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-bold">Upload de Emails</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
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
              <p className="text-xs text-gray-500 mt-1">
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
              <label className="flex items-center gap-2 cursor-pointer pb-2">
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

          <div className="p-3 bg-blue-50 rounded-lg text-sm text-blue-800">
            <p className="font-medium mb-1">Upload em background:</p>
            <ul className="list-disc list-inside space-y-1 text-blue-700">
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

// List Card with import progress
function ListCard({ list, importJob, onEdit, onDelete, onUpload }) {
  const isImporting = importJob && (importJob.status === 'pending' || importJob.status === 'processing')
  const importCompleted = importJob && importJob.status === 'completed'

  return (
    <div className="card">
      <div className="flex items-start justify-between mb-4">
        <div>
          <h3 className="font-semibold text-lg">{list.name}</h3>
          {list.description && (
            <p className="text-sm text-gray-500 mt-1">{list.description}</p>
          )}
        </div>
        <div className="flex gap-1">
          <button
            onClick={onEdit}
            className="p-2 text-gray-600 hover:bg-gray-100 rounded"
            disabled={isImporting}
          >
            <Edit className="w-4 h-4" />
          </button>
          <button
            onClick={onDelete}
            className="p-2 text-red-600 hover:bg-red-50 rounded"
            disabled={isImporting || list.status === 'deleting'}
          >
            <Trash2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Import Progress */}
      {isImporting && (
        <div className="mb-4 p-3 bg-blue-50 rounded-lg">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm font-medium text-blue-800">Importando...</span>
            <span className="text-sm text-blue-600">{importJob.progress || 0}%</span>
          </div>
          <div className="w-full h-2 bg-blue-200 rounded-full overflow-hidden">
            <div
              className="h-full bg-blue-500 rounded-full transition-all duration-300"
              style={{ width: `${importJob.progress || 0}%` }}
            />
          </div>
          <div className="flex justify-between mt-2 text-xs">
            <span className="text-blue-600">{(importJob.processed || 0).toLocaleString()} / {(importJob.total_lines || 0).toLocaleString()}</span>
            <div className="flex gap-3">
              <span className="text-green-600">+{(importJob.valid || 0).toLocaleString()} válidos</span>
              {importJob.invalid > 0 && (
                <span className="text-red-500">{(importJob.invalid || 0).toLocaleString()} inválidos</span>
              )}
              {importJob.duplicates > 0 && (
                <span className="text-yellow-600">{(importJob.duplicates || 0).toLocaleString()} duplicados</span>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Import Completed Summary */}
      {importCompleted && (
        <div className="mb-4 p-3 bg-green-50 rounded-lg">
          <div className="flex items-center gap-2 mb-2">
            <CheckCircle className="w-4 h-4 text-green-600" />
            <span className="text-sm font-medium text-green-800">Importacao concluida!</span>
          </div>
          <div className="grid grid-cols-3 gap-2 text-xs">
            <div className="text-green-600">
              <span className="font-bold">{(importJob.valid || 0).toLocaleString()}</span> validos
            </div>
            {importJob.invalid > 0 && (
              <div className="text-red-600">
                <span className="font-bold">{(importJob.invalid || 0).toLocaleString()}</span> invalidos
              </div>
            )}
            {importJob.duplicates > 0 && (
              <div className="text-yellow-600">
                <span className="font-bold">{(importJob.duplicates || 0).toLocaleString()}</span> duplicados
              </div>
            )}
          </div>
        </div>
      )}

      <div className="flex items-center gap-6 text-sm text-gray-600 mb-4">
        <div className="flex items-center gap-2">
          <Users className="w-4 h-4" />
          {(list.valid_emails || 0).toLocaleString()} emails
        </div>
        <span className={`badge ${
          isImporting ? 'badge-warning' :
          list.status === 'deleting' ? 'badge-error' :
          list.status === 'ready' ? 'badge-success' : 'badge-warning'
        }`}>
          {isImporting ? 'Importando' :
           list.status === 'deleting' ? 'Excluindo...' :
           list.status === 'ready' ? 'Pronta' : 'Processando'}
        </span>
      </div>

      <button
        onClick={onUpload}
        disabled={isImporting}
        className="btn btn-secondary w-full flex items-center justify-center gap-2"
      >
        {isImporting ? (
          <>
            <Loader2 className="w-4 h-4 animate-spin" />
            Importando...
          </>
        ) : (
          <>
            <Upload className="w-4 h-4" />
            Upload de Emails
          </>
        )}
      </button>
    </div>
  )
}

function EmailLists() {
  const [lists, setLists] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, list: null })
  const [uploadModal, setUploadModal] = useState({ open: false, listId: null })
  const [importJobs, setImportJobs] = useState({}) // { listId: jobStatus }
  const pollIntervals = useRef({})

  useEffect(() => {
    fetchListsAndCheckJobs()
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
            <ListCard
              key={list.id}
              list={list}
              importJob={importJobs[list.id]}
              onEdit={() => setModal({ open: true, list })}
              onDelete={() => deleteList(list.id)}
              onUpload={() => setUploadModal({ open: true, listId: list.id })}
            />
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
          onUploadStarted={handleUploadStarted}
        />
      )}
    </div>
  )
}

export default EmailLists
