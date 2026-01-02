import { useState, useEffect } from 'react'
import { Settings as SettingsIcon, Save, RefreshCw, Globe, RotateCcw, AlertCircle, Server, Power, Upload, Database } from 'lucide-react'
import api from '../services/api'
import toast from 'react-hot-toast'

function Settings() {
  const [settings, setSettings] = useState({})
  const [serverInfo, setServerInfo] = useState({ ip: '' })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)

  useEffect(() => {
    fetchSettings()
    fetchServerInfo()
  }, [])

  const fetchSettings = async () => {
    try {
      setLoading(true)
      const response = await api.get('/settings')

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

  const fetchServerInfo = async () => {
    try {
      const response = await api.get('/settings/server-info')
      setServerInfo(response.data)
    } catch (error) {
      // Ignore error, IP will just not show
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

  const handleRestart = async () => {
    if (!confirm('Tem certeza que deseja reiniciar o servidor? As campanhas em andamento serao pausadas.')) {
      return
    }

    try {
      setRestarting(true)
      await api.post('/settings/restart')
      toast.success('Servidor reiniciado com sucesso!')
    } catch (error) {
      toast.error('Erro ao reiniciar servidor')
    } finally {
      setRestarting(false)
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

        {serverInfo.ip && (
          <div className="bg-gray-700 rounded-lg p-3 mb-4 flex items-center gap-3">
            <Server className="w-5 h-5 text-blue-400" />
            <div>
              <span className="text-gray-400 text-sm">IP do Servidor: </span>
              <span className="text-white font-mono font-bold">{serverInfo.ip}</span>
              <p className="text-gray-500 text-xs mt-1">
                Aponte seu dominio de tracking para este IP (registro A)
              </p>
            </div>
          </div>
        )}

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
          <RotateCcw className="w-5 h-5 text-blue-500" />
          <h2 className="text-lg font-semibold">Workers e Retentativas</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure a quantidade de workers e como o sistema deve lidar com falhas.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
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
              Workers paralelos (1-100)
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Tentativas de Reenvio
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
              Tentativas em caso de falha (0-10)
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Atraso entre Tentativas (seg)
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
              Segundos entre tentativas (10-3600)
            </p>
          </div>
        </div>
      </div>

      {/* Upload Settings */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Upload className="w-5 h-5 text-purple-500" />
          <h2 className="text-lg font-semibold">Configuracoes de Upload</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure o tamanho maximo permitido para upload de listas de emails.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Tamanho Maximo de Upload (MB)
            </label>
            <input
              type="number"
              value={settings.max_upload_size_mb || '500'}
              onChange={(e) => handleChange('max_upload_size_mb', e.target.value)}
              min="10"
              max="2000"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Maximo em megabytes (10-2000 MB)
            </p>
          </div>
          <div className="flex items-center">
            <div className="bg-gray-700 rounded-lg p-4 w-full">
              <p className="text-gray-300 text-sm">
                <span className="font-semibold text-purple-400">{settings.max_upload_size_mb || '500'} MB</span> = aproximadamente{' '}
                <span className="font-semibold text-green-400">
                  {Math.round((settings.max_upload_size_mb || 500) / 0.00003).toLocaleString()}
                </span>{' '}
                emails
              </p>
              <p className="text-gray-500 text-xs mt-1">
                Estimativa baseada em ~30 bytes por email
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Import Settings */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Database className="w-5 h-5 text-orange-500" />
          <h2 className="text-lg font-semibold">Processamento de Importacao</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure quantos emails sao processados por vez durante a importacao de listas.
          Valores maiores sao mais rapidos, mas usam mais memoria.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Emails por Lote (Batch Size)
            </label>
            <input
              type="number"
              value={settings.import_batch_size || '50000'}
              onChange={(e) => handleChange('import_batch_size', e.target.value)}
              min="1000"
              max="100000"
              step="1000"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Quantidade de emails por COPY (1000-100000)
            </p>
          </div>
          <div className="flex items-center">
            <div className="bg-gray-700 rounded-lg p-4 w-full">
              <p className="text-gray-300 text-sm">
                <span className="font-semibold text-orange-400">{settings.import_batch_size || '50000'}</span> emails por COPY
              </p>
              <p className="text-gray-500 text-xs mt-1">
                Recomendado: 50000 para melhor performance
              </p>
              <p className="text-gray-500 text-xs mt-1">
                5 milhoes de emails = ~{Math.ceil(5000000 / (settings.import_batch_size || 50000)).toLocaleString()} operacoes
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Action Buttons */}
      <div className="flex flex-col sm:flex-row gap-4">
        <button
          onClick={handleSave}
          disabled={saving}
          className="btn-primary flex items-center justify-center gap-2 px-6 py-3"
        >
          {saving ? (
            <RefreshCw className="w-5 h-5 animate-spin" />
          ) : (
            <Save className="w-5 h-5" />
          )}
          Salvar Configuracoes
        </button>

        <button
          onClick={handleRestart}
          disabled={restarting}
          className="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-lg flex items-center justify-center gap-2 transition-colors"
        >
          {restarting ? (
            <RefreshCw className="w-5 h-5 animate-spin" />
          ) : (
            <Power className="w-5 h-5" />
          )}
          Reiniciar Servidor
        </button>
      </div>

      {/* Info Box */}
      <div className="bg-blue-900/30 border border-blue-700 rounded-lg p-4 flex items-start gap-3">
        <AlertCircle className="w-5 h-5 text-blue-400 mt-0.5" />
        <div>
          <h3 className="font-medium text-blue-300">Importante</h3>
          <p className="text-blue-200 text-sm mt-1">
            Alteracoes na quantidade de workers requerem reinicializacao do servidor.
            O dominio de tracking sera aplicado imediatamente nas novas campanhas.
          </p>
        </div>
      </div>
    </div>
  )
}

export default Settings
