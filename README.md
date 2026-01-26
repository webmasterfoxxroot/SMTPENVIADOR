# SMTPENVIADOR

Sistema de envio de emails em alto volume com suporte a múltiplos servidores SMTP (PowerMTA).

## Características

- **Engine de Alto Desempenho**: Escrito em Go com goroutines para máxima performance
- **Múltiplos SMTPs**: Distribui envios entre vários servidores PowerMTA
- **Dashboard Moderno**: Interface React com estatísticas em tempo real
- **Tracking Completo**: Abertura, cliques e descadastro
- **Personalização**: Variáveis dinâmicas nos emails
- **Rate Limiting**: Controle de velocidade por SMTP
- **Blacklist**: Gerenciamento automático de bounces

## Stack Tecnológica

- **Backend**: Go (Fiber framework)
- **Frontend**: React + Vite + TailwindCSS
- **Banco de Dados**: PostgreSQL
- **Fila**: Redis
- **Containerização**: Docker

## Instalação Rápida

### Instalação Automática (Recomendado)

**Uma linha só!** Instala Docker, configura tudo e inicia o sistema:

```bash
curl -fsSL https://raw.githubusercontent.com/webmasterfoxxroot/SMTPENVIADOR/main/install.sh | sudo bash
```

O instalador automaticamente:
- Instala Docker (se necessário)
- Clona o repositório em `/opt/smtpenviador`
- Gera senhas seguras aleatórias
- Configura o banco de dados
- Inicia todos os containers

### Instalação Manual

```bash
# Clone o repositório
git clone https://github.com/webmasterfoxxroot/SMTPENVIADOR.git
cd SMTPENVIADOR

# Copie o arquivo de ambiente
cp .env.example .env

# Edite as configurações
nano .env

# Inicie os containers
docker compose up -d
```

### Acesso

- **Dashboard**: http://localhost:3000
- **API**: http://localhost:8080
- **Login padrão**: admin@admin.com / admin123

## Configuração

### Variáveis de Ambiente (.env)

```env
# Database
DB_HOST=postgres
DB_PORT=5432
DB_USER=smtpenviador
DB_PASSWORD=sua_senha_segura
DB_NAME=smtpenviador

# Redis
REDIS_HOST=redis
REDIS_PORT=6379

# API
API_PORT=8080
API_SECRET=seu_jwt_secret_aqui

# Engine
WORKERS_COUNT=10
CONNECTIONS_PER_SMTP=5

# Tracking
TRACKING_DOMAIN=https://seu-dominio.com
```

## Uso

### 1. Adicionar Servidores SMTP

Acesse **Servidores SMTP** no menu e adicione seus PowerMTAs:

- Nome: Identificação do servidor
- Host: Endereço do servidor
- Porta: 587 (TLS) ou 25
- Usuário/Senha: Credenciais SMTP AUTH
- Max/Minuto: Limite de envio por minuto

### 2. Criar Lista de Emails

Acesse **Listas de Emails**:

1. Clique em "Nova Lista"
2. Dê um nome à lista
3. Clique em "Upload de Emails"
4. Faça upload de um arquivo CSV/TXT

Formato do arquivo:
```
email,nome,custom1,custom2
joao@email.com,João Silva,Empresa A,Cargo
maria@email.com,Maria Santos,Empresa B,Cargo
```

### 3. Criar Campanha

Acesse **Campanhas** > **Nova Campanha**:

1. Preencha nome e selecione a lista
2. Configure remetente (from name, from email)
3. Escreva o assunto e conteúdo HTML
4. Salve e clique em **Iniciar**

### Variáveis Disponíveis

Use no assunto e corpo do email:

| Variável | Descrição |
|----------|-----------|
| `{{nome}}` | Nome do destinatário |
| `{{email}}` | Email do destinatário |
| `{{data}}` | Data atual (DD/MM/AAAA) |
| `{{custom1}}` a `{{custom10}}` | Campos personalizados |
| `{{unsubscribe}}` | Link de descadastro |

## API REST

### Autenticação

```bash
POST /api/v1/auth/login
{
  "email": "admin@admin.com",
  "password": "admin123"
}
```

Resposta:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": "...", "email": "...", "name": "...", "role": "admin" }
}
```

Use o token no header:
```
Authorization: Bearer <token>
```

### Endpoints Principais

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | /api/v1/stats | Estatísticas do dashboard |
| GET | /api/v1/smtp | Listar SMTPs |
| POST | /api/v1/smtp | Criar SMTP |
| GET | /api/v1/lists | Listar listas |
| POST | /api/v1/lists/:id/upload | Upload de emails |
| GET | /api/v1/campaigns | Listar campanhas |
| POST | /api/v1/campaigns | Criar campanha |
| POST | /api/v1/campaigns/:id/start | Iniciar campanha |
| POST | /api/v1/campaigns/:id/pause | Pausar campanha |

## Performance

Estimativas com configuração padrão (10 workers):

| SMTPs | Emails/Minuto | Emails/Hora | Emails/Dia |
|-------|---------------|-------------|------------|
| 1 | 1.000 | 60.000 | 1.4M |
| 5 | 5.000 | 300.000 | 7.2M |
| 10 | 10.000 | 600.000 | 14.4M |

## Escalabilidade

Para aumentar o volume:

1. **Mais Workers**: Aumente `WORKERS_COUNT`
2. **Mais SMTPs**: Adicione mais servidores PowerMTA
3. **Múltiplas Instâncias**: Execute várias instâncias do engine

## Troubleshooting

### Emails não estão sendo enviados

1. Verifique se os SMTPs estão "Online" no dashboard
2. Teste a conexão com o botão "Testar"
3. Verifique os logs: `docker-compose logs api`

### Taxa de entrega baixa

1. Verifique a reputação dos IPs
2. Configure SPF/DKIM/DMARC nos domínios
3. Faça warmup dos IPs novos
4. Monitore bounces e remova emails inválidos

### Dashboard lento

1. Limpe dados antigos do Redis
2. Aumente recursos do container
3. Verifique conexão com PostgreSQL

## Licença

MIT License

## Suporte

- Issues: https://github.com/webmasterfoxxroot/SMTPENVIADOR/issues
- Email: suporte@smtpenviador.com
