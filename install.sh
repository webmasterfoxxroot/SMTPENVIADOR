#!/bin/bash

###########################################
# SMTPENVIADOR - Instalador Automatico
# Uso: curl -fsSL https://raw.githubusercontent.com/webmasterfoxxroot/SMTPENVIADOR/main/install.sh | bash
###########################################

set -e

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_banner() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                                                           ║"
    echo "║   ███████╗███╗   ███╗████████╗██████╗                     ║"
    echo "║   ██╔════╝████╗ ████║╚══██╔══╝██╔══██╗                    ║"
    echo "║   ███████╗██╔████╔██║   ██║   ██████╔╝                    ║"
    echo "║   ╚════██║██║╚██╔╝██║   ██║   ██╔═══╝                     ║"
    echo "║   ███████║██║ ╚═╝ ██║   ██║   ██║                         ║"
    echo "║   ╚══════╝╚═╝     ╚═╝   ╚═╝   ╚═╝   ENVIADOR              ║"
    echo "║                                                           ║"
    echo "║   Sistema de Envio de Emails em Alto Volume               ║"
    echo "║                                                           ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[AVISO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERRO]${NC} $1"
}

# Detectar IP publico
get_public_ip() {
    curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null || echo "localhost"
}

# Verificar se esta rodando como root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "Este script precisa ser executado como root"
        log_info "Execute: sudo bash install.sh"
        exit 1
    fi
}

# Instalar Docker
install_docker() {
    if command -v docker &> /dev/null; then
        log_info "Docker ja esta instalado"
        return 0
    fi

    log_info "Instalando Docker..."

    # Atualizar pacotes
    apt-get update -qq

    # Instalar dependencias
    apt-get install -y -qq ca-certificates curl gnupg lsb-release

    # Adicionar chave GPG do Docker
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    # Adicionar repositorio
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    # Instalar Docker
    apt-get update -qq
    apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    # Iniciar Docker
    systemctl start docker
    systemctl enable docker

    log_info "Docker instalado com sucesso!"
}

# Instalar Git
install_git() {
    if command -v git &> /dev/null; then
        log_info "Git ja esta instalado"
        return 0
    fi

    log_info "Instalando Git..."
    apt-get install -y -qq git
}

# Clonar repositorio
clone_repo() {
    INSTALL_DIR="/opt/smtpenviador"

    if [ -d "$INSTALL_DIR" ]; then
        log_warn "Diretorio $INSTALL_DIR ja existe"
        read -p "Deseja remover e reinstalar? (s/n): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Ss]$ ]]; then
            rm -rf "$INSTALL_DIR"
        else
            log_info "Usando instalacao existente..."
            cd "$INSTALL_DIR"
            git pull origin main 2>/dev/null || true
            return 0
        fi
    fi

    log_info "Clonando repositorio..."
    git clone https://github.com/webmasterfoxxroot/SMTPENVIADOR.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
}

# Configurar ambiente
configure_env() {
    cd "$INSTALL_DIR"

    PUBLIC_IP=$(get_public_ip)
    TRACKING_DOMAIN="http://${PUBLIC_IP//./-}.$(hostname -d 2>/dev/null || echo 'server')"

    # Gerar senha aleatoria
    RANDOM_PASSWORD=$(openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16)
    JWT_SECRET=$(openssl rand -base64 32 | tr -dc 'a-zA-Z0-9' | head -c 32)

    log_info "Configurando variaveis de ambiente..."

    cat > .env << EOF
# Database
DB_HOST=postgres
DB_PORT=5432
DB_USER=smtpenviador
DB_PASSWORD=${RANDOM_PASSWORD}
DB_NAME=smtpenviador

# Redis
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=

# ClickHouse
CLICKHOUSE_HOST=clickhouse
CLICKHOUSE_PORT=9000
CLICKHOUSE_USER=smtpenviador
CLICKHOUSE_PASSWORD=${RANDOM_PASSWORD}
CLICKHOUSE_DB=smtpenviador

# API
API_PORT=80
API_SECRET=${JWT_SECRET}

# Engine
WORKERS_COUNT=30
CONNECTIONS_PER_SMTP=20

# Tracking (seu IP publico)
TRACKING_DOMAIN=http://${PUBLIC_IP}
EOF

    log_info "Arquivo .env criado com senhas aleatorias"
}

# Iniciar containers
start_containers() {
    cd "$INSTALL_DIR"

    log_info "Iniciando containers Docker..."
    docker compose down 2>/dev/null || true
    docker compose up -d --build

    log_info "Aguardando containers iniciarem..."
    sleep 10

    # Verificar se esta rodando
    if docker compose ps | grep -q "Up"; then
        log_info "Containers iniciados com sucesso!"
    else
        log_error "Alguns containers podem ter falhado. Verificando logs..."
        docker compose logs --tail=20
    fi
}

# Mostrar informacoes finais
show_info() {
    PUBLIC_IP=$(get_public_ip)

    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║          INSTALACAO CONCLUIDA COM SUCESSO!                ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BLUE}Acesse o sistema:${NC}"
    echo ""
    echo -e "  Dashboard: ${YELLOW}http://${PUBLIC_IP}:3000${NC}"
    echo -e "  API:       ${YELLOW}http://${PUBLIC_IP}${NC}"
    echo ""
    echo -e "${BLUE}Login padrao:${NC}"
    echo ""
    echo -e "  Email:  ${YELLOW}admin@admin.com${NC}"
    echo -e "  Senha:  ${YELLOW}admin123${NC}"
    echo ""
    echo -e "${BLUE}Comandos uteis:${NC}"
    echo ""
    echo -e "  Ver logs:      ${YELLOW}cd /opt/smtpenviador && docker compose logs -f${NC}"
    echo -e "  Reiniciar:     ${YELLOW}cd /opt/smtpenviador && docker compose restart${NC}"
    echo -e "  Parar:         ${YELLOW}cd /opt/smtpenviador && docker compose down${NC}"
    echo -e "  Atualizar:     ${YELLOW}cd /opt/smtpenviador && git pull && docker compose up -d --build${NC}"
    echo ""
    echo -e "${RED}IMPORTANTE: Troque a senha do admin apos o primeiro login!${NC}"
    echo ""
}

# Main
main() {
    print_banner
    check_root
    install_docker
    install_git
    clone_repo
    configure_env
    start_containers
    show_info
}

# Executar
main "$@"
