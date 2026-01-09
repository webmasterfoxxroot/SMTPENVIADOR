import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Save, Loader2, ChevronDown, ChevronRight, Folder, Users, CheckCircle, FileText } from 'lucide-react'
import toast from 'react-hot-toast'
import api from '../services/api'

function CampaignEdit() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [lists, setLists] = useState([])
  const [groups, setGroups] = useState([])
  const [expandedGroups, setExpandedGroups] = useState({})
  const [templates, setTemplates] = useState([])
  const [selectedTemplate, setSelectedTemplate] = useState('')

  const [form, setForm] = useState({
    name: '',
    from_name: '',
    subject: '',
    html_content: '',
    text_content: '',
    list_ids: [],
    send_rate: 0,
    track_opens: true,
    track_clicks: true
  })

  useEffect(() => {
    fetchLists()
    fetchGroups()
    fetchTemplates()
    if (id) {
      fetchCampaign()
    }
  }, [id])

  const fetchLists = async () => {
    try {
      const response = await api.get('/lists')
      setLists(response.data.data || [])
    } catch (error) {
      console.error('Failed to fetch lists:', error)
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
      console.log('No groups:', error)
    }
  }

  const fetchTemplates = async () => {
    try {
      const response = await api.get('/templates')
      setTemplates(response.data.data || [])
    } catch (error) {
      console.log('No templates:', error)
    }
  }

  const handleTemplateSelect = async (templateId) => {
    if (!templateId) {
      setSelectedTemplate('')
      return
    }

    try {
      const response = await api.get(`/templates/${templateId}`)
      const template = response.data
      setSelectedTemplate(templateId)

      // Fill in the form with template data
      setForm(prev => ({
        ...prev,
        from_name: template.from_name || prev.from_name,
        subject: template.subject || prev.subject,
        html_content: template.html_content || prev.html_content,
        text_content: template.text_content || prev.text_content
      }))

      toast.success(`Template "${template.name}" aplicado!`)
    } catch (error) {
      toast.error('Erro ao carregar template')
    }
  }

  const fetchCampaign = async () => {
    setLoading(true)
    try {
      const response = await api.get(`/campaigns/${id}`)
      const data = response.data
      // Handle both list_ids array and legacy list_id
      const listIds = data.list_ids || (data.list_id ? [data.list_id] : [])
      setForm({ ...data, list_ids: listIds })
    } catch (error) {
      toast.error('Erro ao carregar campanha')
      navigate('/campaigns')
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()

    if (form.list_ids.length === 0) {
      toast.error('Selecione pelo menos uma lista')
      return
    }

    setSaving(true)

    try {
      if (id) {
        await api.put(`/campaigns/${id}`, form)
        toast.success('Campanha atualizada!')
        navigate('/campaigns')
      } else {
        const response = await api.post('/campaigns', form)
        toast.success('Campanha criada! Countdown iniciado.')
        navigate('/campaigns', {
          state: {
            newCampaignId: response.data.id,
            autoStart: response.data.auto_start
          }
        })
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  const toggleList = (listId) => {
    setForm(prev => {
      const isSelected = prev.list_ids.includes(listId)
      if (isSelected) {
        return { ...prev, list_ids: prev.list_ids.filter(id => id !== listId) }
      } else {
        return { ...prev, list_ids: [...prev.list_ids, listId] }
      }
    })
  }

  const toggleGroup = (groupId) => {
    setExpandedGroups(prev => ({ ...prev, [groupId]: !prev[groupId] }))
  }

  const selectAllInGroup = (groupLists) => {
    const allIds = groupLists.map(l => l.id)
    const allSelected = allIds.every(id => form.list_ids.includes(id))

    if (allSelected) {
      // Deselect all
      setForm(prev => ({
        ...prev,
        list_ids: prev.list_ids.filter(id => !allIds.includes(id))
      }))
    } else {
      // Select all
      setForm(prev => ({
        ...prev,
        list_ids: [...new Set([...prev.list_ids, ...allIds])]
      }))
    }
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

  // Calculate totals
  const selectedLists = lists.filter(l => form.list_ids.includes(l.id))
  const totalEmails = selectedLists.reduce((sum, l) => sum + (l.valid_emails || 0), 0)

  const { grouped, ungrouped } = getListsByGroup()

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center gap-4 mb-6">
        <button
          onClick={() => navigate('/campaigns')}
          className="p-2 hover:bg-gray-100 rounded"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <h1 className="text-2xl font-bold">
          {id ? 'Editar Campanha' : 'Nova Campanha'}
        </h1>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Informacoes Basicas</h2>

          {/* Template Selector - only show for new campaigns */}
          {!id && templates.length > 0 && (
            <div className="mb-4 p-4 bg-blue-50 border border-blue-200 rounded-lg">
              <div className="flex items-center gap-2 mb-2">
                <FileText className="w-5 h-5 text-blue-600" />
                <label className="font-medium text-blue-800">Usar Template</label>
              </div>
              <select
                value={selectedTemplate}
                onChange={(e) => handleTemplateSelect(e.target.value)}
                className="input"
              >
                <option value="">-- Selecione um template (opcional) --</option>
                {templates.map(template => (
                  <option key={template.id} value={template.id}>
                    {template.name} - {template.subject}
                  </option>
                ))}
              </select>
              <p className="text-xs text-blue-600 mt-1">
                Ao selecionar um template, os campos Nome do Remetente, Assunto e Conteúdo serão preenchidos automaticamente.
              </p>
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="label">Nome da Campanha</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input"
                placeholder="Black Friday 2024"
                required
              />
            </div>
            <div>
              <label className="label">Taxa de Envio (emails/min, 0 = ilimitado)</label>
              <input
                type="number"
                value={form.send_rate}
                onChange={(e) => setForm({ ...form, send_rate: parseInt(e.target.value) || 0 })}
                className="input"
                min="0"
              />
            </div>
          </div>

          {/* List Selection */}
          <div className="mt-6">
            <div className="flex items-center justify-between mb-3">
              <label className="label mb-0">Listas de Emails</label>
              {form.list_ids.length > 0 && (
                <span className="text-sm text-blue-600 font-medium">
                  {form.list_ids.length} lista(s) selecionada(s) - {totalEmails.toLocaleString()} emails
                </span>
              )}
            </div>

            <div className="border border-gray-200 rounded-lg max-h-80 overflow-y-auto">
              {/* Groups */}
              {groups.map(group => {
                const groupLists = grouped[group.id] || []
                if (groupLists.length === 0) return null

                const isExpanded = expandedGroups[group.id]
                const selectedCount = groupLists.filter(l => form.list_ids.includes(l.id)).length
                const allSelected = selectedCount === groupLists.length && groupLists.length > 0

                return (
                  <div key={group.id} className="border-b border-gray-100 last:border-b-0">
                    <div
                      className="flex items-center justify-between px-4 py-2 bg-gray-50 cursor-pointer hover:bg-gray-100"
                      onClick={() => toggleGroup(group.id)}
                    >
                      <div className="flex items-center gap-3">
                        {isExpanded ? <ChevronDown className="w-4 h-4 text-gray-500" /> : <ChevronRight className="w-4 h-4 text-gray-500" />}
                        <div className="w-3 h-3 rounded-full" style={{ backgroundColor: group.color }} />
                        <span className="font-medium text-sm">{group.name}</span>
                        <span className="text-xs text-gray-500">({groupLists.length} listas)</span>
                        {selectedCount > 0 && (
                          <span className="text-xs bg-blue-100 text-blue-700 px-2 py-0.5 rounded-full">
                            {selectedCount} selecionadas
                          </span>
                        )}
                      </div>
                      <button
                        type="button"
                        onClick={(e) => { e.stopPropagation(); selectAllInGroup(groupLists) }}
                        className={`text-xs px-2 py-1 rounded ${allSelected ? 'bg-blue-500 text-white' : 'bg-gray-200 text-gray-700 hover:bg-gray-300'}`}
                      >
                        {allSelected ? 'Desmarcar todas' : 'Selecionar todas'}
                      </button>
                    </div>
                    {isExpanded && (
                      <div className="bg-white">
                        {groupLists.map(list => (
                          <label
                            key={list.id}
                            className="flex items-center gap-3 px-4 py-2 pl-10 hover:bg-gray-50 cursor-pointer"
                          >
                            <input
                              type="checkbox"
                              checked={form.list_ids.includes(list.id)}
                              onChange={() => toggleList(list.id)}
                              className="w-4 h-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                            />
                            <span className="flex-1 text-sm">{list.name}</span>
                            <span className="text-xs text-gray-500 flex items-center gap-1">
                              <Users className="w-3 h-3" />
                              {(list.valid_emails || 0).toLocaleString()}
                            </span>
                            {list.status === 'ready' && (
                              <CheckCircle className="w-4 h-4 text-green-500" />
                            )}
                          </label>
                        ))}
                      </div>
                    )}
                  </div>
                )
              })}

              {/* Ungrouped Lists */}
              {ungrouped.length > 0 && (
                <div className="border-b border-gray-100 last:border-b-0">
                  <div
                    className="flex items-center justify-between px-4 py-2 bg-gray-50 cursor-pointer hover:bg-gray-100"
                    onClick={() => toggleGroup('ungrouped')}
                  >
                    <div className="flex items-center gap-3">
                      {expandedGroups['ungrouped'] ? <ChevronDown className="w-4 h-4 text-gray-500" /> : <ChevronRight className="w-4 h-4 text-gray-500" />}
                      <Folder className="w-4 h-4 text-gray-400" />
                      <span className="font-medium text-sm text-gray-600">Sem Grupo</span>
                      <span className="text-xs text-gray-500">({ungrouped.length} listas)</span>
                    </div>
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); selectAllInGroup(ungrouped) }}
                      className="text-xs px-2 py-1 rounded bg-gray-200 text-gray-700 hover:bg-gray-300"
                    >
                      Selecionar todas
                    </button>
                  </div>
                  {expandedGroups['ungrouped'] && (
                    <div className="bg-white">
                      {ungrouped.map(list => (
                        <label
                          key={list.id}
                          className="flex items-center gap-3 px-4 py-2 pl-10 hover:bg-gray-50 cursor-pointer"
                        >
                          <input
                            type="checkbox"
                            checked={form.list_ids.includes(list.id)}
                            onChange={() => toggleList(list.id)}
                            className="w-4 h-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                          />
                          <span className="flex-1 text-sm">{list.name}</span>
                          <span className="text-xs text-gray-500 flex items-center gap-1">
                            <Users className="w-3 h-3" />
                            {(list.valid_emails || 0).toLocaleString()}
                          </span>
                          {list.status === 'ready' && (
                            <CheckCircle className="w-4 h-4 text-green-500" />
                          )}
                        </label>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {lists.length === 0 && (
                <div className="p-4 text-center text-gray-500 text-sm">
                  Nenhuma lista disponivel
                </div>
              )}
            </div>
          </div>

          <div className="flex gap-6 mt-4">
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="track_opens"
                checked={form.track_opens}
                onChange={(e) => setForm({ ...form, track_opens: e.target.checked })}
                className="w-5 h-5 rounded border-gray-300 text-blue-500 focus:ring-blue-500"
              />
              <label htmlFor="track_opens" className="text-sm">
                Rastrear aberturas
              </label>
            </div>
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="track_clicks"
                checked={form.track_clicks}
                onChange={(e) => setForm({ ...form, track_clicks: e.target.checked })}
                className="w-5 h-5 rounded border-gray-300 text-blue-500 focus:ring-blue-500"
              />
              <label htmlFor="track_clicks" className="text-sm">
                Rastrear cliques
              </label>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Conteudo do Email</h2>

          <div className="space-y-4">
            <div>
              <label className="label">Nome do Remetente</label>
              <input
                type="text"
                value={form.from_name}
                onChange={(e) => setForm({ ...form, from_name: e.target.value })}
                className="input"
                placeholder="Empresa XYZ"
                required
              />
              <p className="text-xs text-gray-500 mt-1">
                Nome que aparece no campo "De:" do email. O email sera do SMTP.
              </p>
            </div>

            <div>
              <label className="label">Assunto</label>
              <input
                type="text"
                value={form.subject}
                onChange={(e) => setForm({ ...form, subject: e.target.value })}
                className="input"
                placeholder="Ola {{nome}}, confira nossa oferta!"
                required
              />
              <p className="text-xs text-gray-500 mt-1">
                Use variaveis: {'{{nome}}'}, {'{{email}}'}, {'{{custom1}}'}, etc.
              </p>
            </div>

            <div>
              <label className="label">Conteudo HTML</label>
              <textarea
                value={form.html_content}
                onChange={(e) => setForm({ ...form, html_content: e.target.value })}
                className="input font-mono text-sm"
                rows={15}
                placeholder="<html><body>Ola {{nome}}!</body></html>"
                required
              />
            </div>

            <div>
              <label className="label">Conteudo Texto (opcional)</label>
              <textarea
                value={form.text_content}
                onChange={(e) => setForm({ ...form, text_content: e.target.value })}
                className="input"
                rows={5}
                placeholder="Versao texto do email..."
              />
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-3">
          <button
            type="button"
            onClick={() => navigate('/campaigns')}
            className="btn btn-secondary"
          >
            Cancelar
          </button>
          <button type="submit" disabled={saving} className="btn btn-primary flex items-center gap-2">
            {saving ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <Save className="w-5 h-5" />
            )}
            Salvar Campanha
          </button>
        </div>
      </form>
    </div>
  )
}

export default CampaignEdit
