import { useState, useEffect } from 'react'
import { Plus, Edit, Trash2, Copy, FileText, Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function TemplateModal({ template, onClose, onSave }) {
  const [form, setForm] = useState({
    name: '',
    from_name: '',
    subject: '',
    html_content: '',
    text_content: '',
    ...template
  })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    try {
      if (template?.id) {
        await api.put(`/templates/${template.id}`, form)
        toast.success('Template atualizado!')
      } else {
        await api.post('/templates', form)
        toast.success('Template criado!')
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
      <div className="bg-white rounded-xl p-6 w-full max-w-3xl max-h-[90vh] overflow-y-auto">
        <h2 className="text-xl font-bold mb-4">
          {template?.id ? 'Editar Template' : 'Novo Template'}
        </h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Nome do Template</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input"
                placeholder="Template Promocional"
                required
              />
            </div>
            <div>
              <label className="label">Nome do Remetente</label>
              <input
                type="text"
                value={form.from_name}
                onChange={(e) => setForm({ ...form, from_name: e.target.value })}
                className="input"
                placeholder="Empresa XYZ"
              />
              <p className="text-xs text-gray-500 mt-1">Nome que aparece no "De:"</p>
            </div>
          </div>

          <div>
            <label className="label">Assunto</label>
            <input
              type="text"
              value={form.subject}
              onChange={(e) => setForm({ ...form, subject: e.target.value })}
              className="input"
              placeholder="Olá {{nome}}!"
              required
            />
          </div>

          <div>
            <label className="label">Conteúdo HTML</label>
            <textarea
              value={form.html_content}
              onChange={(e) => setForm({ ...form, html_content: e.target.value })}
              className="input font-mono text-sm"
              rows={12}
              placeholder="<html><body>...</body></html>"
              required
            />
          </div>

          <div>
            <label className="label">Conteúdo Texto (opcional)</label>
            <textarea
              value={form.text_content}
              onChange={(e) => setForm({ ...form, text_content: e.target.value })}
              className="input"
              rows={4}
            />
          </div>

          <div className="bg-gray-50 p-4 rounded-lg">
            <p className="text-sm font-medium text-gray-700 mb-2">Variáveis disponíveis:</p>
            <div className="flex flex-wrap gap-2">
              {['{{nome}}', '{{email}}', '{{data}}', '{{custom1}}', '{{custom2}}', '{{custom3}}', '{{unsubscribe}}'].map(v => (
                <code key={v} className="px-2 py-1 bg-gray-200 rounded text-sm">{v}</code>
              ))}
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

function Templates() {
  const [templates, setTemplates] = useState([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState({ open: false, template: null })

  useEffect(() => {
    fetchTemplates()
  }, [])

  const fetchTemplates = async () => {
    try {
      const response = await api.get('/templates')
      setTemplates(response.data.data || [])
    } catch (error) {
      toast.error('Erro ao carregar templates')
    } finally {
      setLoading(false)
    }
  }

  const deleteTemplate = async (id) => {
    if (!confirm('Tem certeza que deseja excluir este template?')) return

    try {
      await api.delete(`/templates/${id}`)
      toast.success('Template excluído')
      fetchTemplates()
    } catch (error) {
      toast.error('Erro ao excluir')
    }
  }

  const duplicateTemplate = async (template) => {
    try {
      await api.post('/templates', {
        name: template.name + ' (cópia)',
        from_name: template.from_name,
        subject: template.subject,
        html_content: template.html_content,
        text_content: template.text_content
      })
      toast.success('Template duplicado!')
      fetchTemplates()
    } catch (error) {
      toast.error('Erro ao duplicar')
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Templates</h1>
        <button
          onClick={() => setModal({ open: true, template: null })}
          className="btn btn-primary flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          Novo Template
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {loading ? (
          <div className="col-span-full flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
          </div>
        ) : templates.length === 0 ? (
          <div className="col-span-full text-center py-12 text-gray-500">
            <FileText className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>Nenhum template criado</p>
          </div>
        ) : (
          templates.map((template) => (
            <div key={template.id} className="card">
              <div className="flex items-start justify-between mb-3">
                <h3 className="font-semibold">{template.name}</h3>
                <div className="flex gap-1">
                  <button
                    onClick={() => duplicateTemplate(template)}
                    className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                    title="Duplicar"
                  >
                    <Copy className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => setModal({ open: true, template })}
                    className="p-2 text-gray-600 hover:bg-gray-100 rounded"
                    title="Editar"
                  >
                    <Edit className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => deleteTemplate(template.id)}
                    className="p-2 text-red-600 hover:bg-red-50 rounded"
                    title="Excluir"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
              <p className="text-sm text-gray-500 mb-3">{template.subject}</p>
              <div className="text-xs text-gray-400">
                Criado em {new Date(template.created_at).toLocaleDateString('pt-BR')}
              </div>
            </div>
          ))
        )}
      </div>

      {modal.open && (
        <TemplateModal
          template={modal.template}
          onClose={() => setModal({ open: false, template: null })}
          onSave={() => {
            setModal({ open: false, template: null })
            fetchTemplates()
          }}
        />
      )}
    </div>
  )
}

export default Templates
