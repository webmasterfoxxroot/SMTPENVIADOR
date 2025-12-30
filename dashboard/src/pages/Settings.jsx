import { useState, useEffect } from 'react'
import { Settings as SettingsIcon, Save, RefreshCw, Globe, Users, Link, RotateCcw, Clock, AlertCircle } from 'lucide-react'
import api from '../services/api'
import toast from 'react-hot-toast'

function Settings() {
  const [settings, setSettings] = useState({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    fetchSettings()
  }, [])

  const fetchSettings = async () => {
    try {
      setLoading(true)
      const response = await api.get('/settings')

      // Convert settings object to simple key-value for form
      const formValues = {}
      Object.keys(response.data).forEach(key => {
        formValues[key] = response.data[key].value
      })
      setSettings(formValues)
    } catch (error) {
      toast.error('Erro ao carregar configuracoes')
    } finally {
      setLoading(false)
    }
  }

  const handleChange = (key, value) => {
    setSettings(prev => ({
      ...prev,
      [key]: value
    }))
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      await api.put('/settings', settings)
      toast.success('Configuracoes salvas com sucesso!')
    } catch (error) {
      toast.error('Erro ao salvar configuracoes')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="w-8 h-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <SettingsIcon className="w-8 h-8 text-blue-500" />
          <h1 className="text-2xl font-bold">Configuracoes</h1>
        </div>
        <button
          onClick={handleSave}
          disabled={saving}
          className="btn-primary flex items-center gap-2"
        >
          {saving ? (
            <RefreshCw className="w-4 h-4 animate-spin" />
          ) : (
            <Save className="w-4 h-4" />
          )}
          Salvar Configuracoes
        </button>
      </div>

      {/* Tracking Settings */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Globe className="w-5 h-5 text-green-500" />
          <h2 className="text-lg font-semibold">Dominio de Rastreamento</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure o dominio usado para rastrear aberturas e cliques nos emails.
          Este dominio deve apontar para este servidor.
        </p>
        <div>
          <label className="block text-sm font-medium mb-1">
            Dominio de Tracking
          </label>
          <input
            type="text"
            value={settings.tracking_domain || ''}
            onChange={(e) => handleChange('tracking_domain', e.target.value)}
            placeholder="https://track.seudominio.com"
            className="input w-full max-w-lg"
          />
          <p className="text-gray-500 text-xs mt-1">
            Exemplo: https://track.seudominio.com (sem barra no final)
          </p>
        </div>
      </div>

      {/* Workers Settings */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Users className="w-5 h-5 text-blue-500" />
          <h2 className="text-lg font-semibold">Workers de Envio</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure a quantidade de workers e conexoes para envio de emails.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Quantidade de Workers
            </label>
            <input
              type="number"
              value={settings.workers_count || '10'}
              onChange={(e) => handleChange('workers_count', e.target.value)}
              min="1"
              max="100"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Numero de workers paralelos (1-100)
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Conexoes por SMTP
            </label>
            <input
              type="number"
              value={settings.connections_per_smtp || '5'}
              onChange={(e) => handleChange('connections_per_smtp', e.target.value)}
              min="1"
              max="50"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Conexoes simultaneas por servidor SMTP (1-50)
            </p>
          </div>
        </div>
      </div>

      {/* Rate Limiting */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Link className="w-5 h-5 text-yellow-500" />
          <h2 className="text-lg font-semibold">Taxa de Envio</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure a taxa padrao de envio de emails.
        </p>
        <div>
          <label className="block text-sm font-medium mb-1">
            Taxa Padrao (emails/minuto)
          </label>
          <input
            type="number"
            value={settings.default_send_rate || '0'}
            onChange={(e) => handleChange('default_send_rate', e.target.value)}
            min="0"
            className="input w-full max-w-xs"
          />
          <p className="text-gray-500 text-xs mt-1">
            0 = sem limite (usa velocidade maxima)
          </p>
        </div>
      </div>

      {/* Retry Settings */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <RotateCcw className="w-5 h-5 text-orange-500" />
          <h2 className="text-lg font-semibold">Tentativas de Reenvio</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure como o sistema deve lidar com falhas de envio.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Numero de Tentativas
            </label>
            <input
              type="number"
              value={settings.retry_attempts || '3'}
              onChange={(e) => handleChange('retry_attempts', e.target.value)}
              min="0"
              max="10"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Quantas vezes tentar reenviar (0-10)
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Atraso entre Tentativas (segundos)
            </label>
            <input
              type="number"
              value={settings.retry_delay || '60'}
              onChange={(e) => handleChange('retry_delay', e.target.value)}
              min="10"
              max="3600"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Tempo de espera entre tentativas (10-3600)
            </p>
          </div>
        </div>
      </div>

      {/* Info Box */}
      <div className="bg-blue-900/30 border border-blue-700 rounded-lg p-4 flex items-start gap-3">
        <AlertCircle className="w-5 h-5 text-blue-400 mt-0.5" />
        <div>
          <h3 className="font-medium text-blue-300">Importante</h3>
          <p className="text-blue-200 text-sm mt-1">
            Algumas configuracoes podem requerer reinicializacao do sistema para entrar em vigor.
            As alteracoes no dominio de rastreamento serao aplicadas imediatamente nas novas campanhas.
          </p>
        </div>
      </div>
    </div>
  )
}

export default Settings
