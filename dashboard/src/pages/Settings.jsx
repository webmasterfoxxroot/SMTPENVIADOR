import { useState, useEffect } from 'react'
import { Settings as SettingsIcon, Save, RefreshCw, Globe, RotateCcw, AlertCircle, Server, Power, Upload, Database, Shield, Zap } from 'lucide-react'
import api from '../services/api'
import toast from 'react-hot-toast'

function Settings() {
  const [settings, setSettings] = useState({})
  const [serverInfo, setServerInfo] = useState({ ip: '' })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [testingProxy, setTestingProxy] = useState(false)
  const [proxyTestResult, setProxyTestResult] = useState(null)

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

  const handleTestProxy = async () => {
    if (!settings.proxy_host || !settings.proxy_port || !settings.proxy_user || !settings.proxy_pass) {
      toast.error('Preencha todos os campos do proxy')
      return
    }

    try {
      setTestingProxy(true)
      setProxyTestResult(null)
      const response = await api.post('/settings/test-proxy', {
        host: settings.proxy_host,
        port: settings.proxy_port,
        username: settings.proxy_user,
        password: settings.proxy_pass
      })
      setProxyTestResult({
        success: true,
        ip: response.data.ip,
        region: response.data.region
      })
      toast.success('Proxy funcionando! IP: ' + response.data.ip)
    } catch (error) {
      setProxyTestResult({
        success: false,
        error: error.response?.data?.error || 'Erro ao testar proxy'
      })
      toast.error('Erro ao testar proxy: ' + (error.response?.data?.error || 'Falha na conexao'))
    } finally {
      setTestingProxy(false)
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

      {/* Workers Settings - Separated Queues */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <RotateCcw className="w-5 h-5 text-blue-500" />
          <h2 className="text-lg font-semibold">Workers Dedicados</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure workers separados para campanhas e warmup. Isso garante que campanhas
          nunca sejam afetadas pelo warmup e vice-versa.
        </p>

        {/* Visual explanation */}
        <div className="bg-gray-700 rounded-lg p-4 mb-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="text-center">
              <div className="text-2xl mb-1">📧</div>
              <div className="text-green-400 font-bold">{settings.campaign_workers || '30'} Workers</div>
              <div className="text-gray-400 text-xs">Campanhas (Prioridade Alta)</div>
            </div>
            <div className="text-center">
              <div className="text-2xl mb-1">🔥</div>
              <div className="text-orange-400 font-bold">{settings.warmup_workers || '5'} Workers</div>
              <div className="text-gray-400 text-xs">Warmup (Isolado)</div>
            </div>
          </div>
          <div className="mt-3 text-center text-gray-500 text-xs">
            Total: {(parseInt(settings.campaign_workers) || 30) + (parseInt(settings.warmup_workers) || 5)} workers ativos
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              <span className="text-green-400">📧</span> Workers para Campanhas
            </label>
            <input
              type="number"
              value={settings.campaign_workers || '30'}
              onChange={(e) => handleChange('campaign_workers', e.target.value)}
              min="5"
              max="100"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Workers dedicados para envio de campanhas (5-100)
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              <span className="text-orange-400">🔥</span> Workers para Warmup
            </label>
            <input
              type="number"
              value={settings.warmup_workers || '5'}
              onChange={(e) => handleChange('warmup_workers', e.target.value)}
              min="1"
              max="20"
              className="input w-full"
            />
            <p className="text-gray-500 text-xs mt-1">
              Workers dedicados para aquecimento (1-20)
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
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

      {/* Proxy Settings for Warmup Seeds */}
      <div className="card">
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5 text-cyan-500" />
          <h2 className="text-lg font-semibold">Proxy Residencial (Seed→SMTP)</h2>
        </div>
        <p className="text-gray-400 text-sm mb-4">
          Configure proxy residencial para envios Seed→SMTP. Cada envio usa um IP diferente automaticamente.
        </p>

        {/* Enable/Disable Toggle */}
        <div className="bg-gray-700 rounded-lg p-4 mb-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className={`w-12 h-6 rounded-full p-1 cursor-pointer transition-colors ${settings.proxy_enabled === 'true' ? 'bg-cyan-500' : 'bg-gray-600'}`}
                onClick={() => handleChange('proxy_enabled', settings.proxy_enabled === 'true' ? 'false' : 'true')}>
                <div className={`w-4 h-4 rounded-full bg-white transition-transform ${settings.proxy_enabled === 'true' ? 'translate-x-6' : 'translate-x-0'}`} />
              </div>
              <div>
                <span className="font-medium">Proxy {settings.proxy_enabled === 'true' ? 'Ativado' : 'Desativado'}</span>
                <p className="text-gray-400 text-xs">
                  {settings.proxy_enabled === 'true' ? 'Seeds usam proxy residencial para enviar' : 'Seeds enviam diretamente sem proxy'}
                </p>
              </div>
            </div>
            {settings.proxy_enabled === 'true' && settings.proxy_host && settings.proxy_user && (
              <div className="text-cyan-400 text-sm">
                ✓ Configurado
              </div>
            )}
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Host
            </label>
            <input
              type="text"
              value={settings.proxy_host || ''}
              onChange={(e) => handleChange('proxy_host', e.target.value)}
              placeholder="prem.digiproxy.cc"
              className="input w-full font-mono"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Porta
            </label>
            <input
              type="text"
              value={settings.proxy_port || ''}
              onChange={(e) => handleChange('proxy_port', e.target.value)}
              placeholder="8000"
              className="input w-full font-mono"
            />
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-1">
              Usuário
            </label>
            <input
              type="text"
              value={settings.proxy_user || ''}
              onChange={(e) => handleChange('proxy_user', e.target.value)}
              placeholder="seu_usuario"
              className="input w-full font-mono"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">
              Senha
            </label>
            <input
              type="text"
              value={settings.proxy_pass || ''}
              onChange={(e) => handleChange('proxy_pass', e.target.value)}
              placeholder="sua_senha"
              className="input w-full font-mono"
            />
          </div>
        </div>

        {/* Test Proxy Button */}
        <div className="mb-4">
          <button
            onClick={handleTestProxy}
            disabled={testingProxy || !settings.proxy_host || !settings.proxy_port || !settings.proxy_user || !settings.proxy_pass}
            className="bg-cyan-600 hover:bg-cyan-700 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg flex items-center gap-2 transition-colors"
          >
            {testingProxy ? (
              <RefreshCw className="w-4 h-4 animate-spin" />
            ) : (
              <Zap className="w-4 h-4" />
            )}
            {testingProxy ? 'Testando...' : 'Testar Conexão'}
          </button>

          {proxyTestResult && (
            <div className={`mt-3 p-3 rounded-lg ${proxyTestResult.success ? 'bg-green-900/50 border border-green-700' : 'bg-red-900/50 border border-red-700'}`}>
              {proxyTestResult.success ? (
                <div className="flex items-center gap-2 text-green-400">
                  <span className="text-lg">✓</span>
                  <div>
                    <p className="font-medium">Proxy funcionando!</p>
                    <p className="text-sm text-green-300">IP: <span className="font-mono">{proxyTestResult.ip}</span> | Região: {proxyTestResult.region}</p>
                  </div>
                </div>
              ) : (
                <div className="flex items-center gap-2 text-red-400">
                  <span className="text-lg">✕</span>
                  <div>
                    <p className="font-medium">Erro no proxy</p>
                    <p className="text-sm text-red-300">{proxyTestResult.error}</p>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="bg-gray-700/50 rounded-lg p-3 text-sm">
          <div className="flex items-start gap-2">
            <AlertCircle className="w-4 h-4 text-cyan-400 mt-0.5" />
            <div className="text-gray-300">
              <strong>Formato:</strong> user:pass@host:port — Cada email Seed→SMTP usa um IP diferente (rotativo).
            </div>
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
          <h3 className="font-medium text-blue-300">Importante - Workers Isolados</h3>
          <p className="text-blue-200 text-sm mt-1">
            Os workers de campanha e warmup sao <strong>completamente isolados</strong>.
            Campanhas usam uma fila dedicada e nunca sao afetadas pelo warmup.
          </p>
          <p className="text-blue-200 text-sm mt-2">
            Alteracoes requerem reinicializacao do servidor para aplicar.
          </p>
        </div>
      </div>
    </div>
  )
}

export default Settings
