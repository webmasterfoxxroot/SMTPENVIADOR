package api

import (
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-sasl"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/net/proxy"
)

// Package-level variables for seed batch rotation
var (
	seedBatchOffset int
	seedBatchMutex  sync.Mutex
)

// Constants for seed processing
const (
	maxSeedsPerCycle    = 50 // Maximum seeds to process per cycle
	minDelayBetweenSeed = 5  // Minimum seconds between each seed
	maxDelayBetweenSeed = 10 // Maximum seconds between each seed
)

// ============================================
// DATABASE MIGRATIONS
// ============================================

func (s *Server) initWarmupTables() {
	// First, ensure warmup_settings table exists for tracking migrations
	s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_settings (
			key VARCHAR(100) PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)

	// Check if UUID migration was already done
	var migrationDone string
	s.db.QueryRow(`SELECT value FROM warmup_settings WHERE key = 'uuid_migration_done'`).Scan(&migrationDone)

	// Only check for migration if not already done
	var needsMigration bool
	if migrationDone != "true" {
		// Check if warmup_smtps has wrong column type (VARCHAR instead of UUID)
		var columnType string
		err := s.db.QueryRow(`
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'warmup_smtps' AND column_name = 'id'
		`).Scan(&columnType)
		if err == nil && columnType == "character varying" {
			needsMigration = true
		}

		// Check if warmup_seeds has wrong column type
		err = s.db.QueryRow(`
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'warmup_seeds' AND column_name = 'id'
		`).Scan(&columnType)
		if err == nil && columnType == "character varying" {
			needsMigration = true
		}
	}

	if needsMigration {
		log.Println("[Warmup] Migrating tables to UUID schema...")
		// Drop ALL warmup tables in correct order due to foreign keys
		s.db.Exec(`DROP TABLE IF EXISTS warmup_daily_stats CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_emails CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_templates CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_seeds CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_smtps CASCADE`)

		// Mark migration as done BEFORE creating tables
		s.db.Exec(`INSERT INTO warmup_settings (key, value) VALUES ('uuid_migration_done', 'true')
		           ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = NOW()`)
	}

	// Create warmup_smtps table
	var err error
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_smtps (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			smtp_id UUID NOT NULL REFERENCES smtp_servers(id) ON DELETE CASCADE,
			status VARCHAR(20) DEFAULT 'active',
			recipe_type VARCHAR(20) DEFAULT 'progressive',
			start_date TIMESTAMP DEFAULT NOW(),
			end_date TIMESTAMP,
			current_day INT DEFAULT 1,
			min_emails_per_day INT DEFAULT 5,
			max_emails_per_day INT DEFAULT 40,
			reply_rate INT DEFAULT 30,
			start_hour INT DEFAULT 8,
			end_hour INT DEFAULT 18,
			total_sent INT DEFAULT 0,
			total_inbox INT DEFAULT 0,
			total_spam INT DEFAULT 0,
			total_replies INT DEFAULT 0,
			custom_schedule TEXT,
			internal_warmup BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)
	// Add user_id column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_smtps ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_smtps table: %v", err)
	}

	// Add internal_warmup column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_smtps ADD COLUMN IF NOT EXISTS internal_warmup BOOLEAN DEFAULT false`)
	// Add send_rate column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_smtps ADD COLUMN IF NOT EXISTS send_rate INT DEFAULT 30`)

	// Create warmup_seeds table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_seeds (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			email VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			provider VARCHAR(50) DEFAULT 'other',
			imap_host VARCHAR(255) NOT NULL,
			imap_port INT DEFAULT 993,
			smtp_host VARCHAR(255),
			smtp_port INT DEFAULT 587,
			use_tls BOOLEAN DEFAULT true,
			status VARCHAR(20) DEFAULT 'active',
			send_rate INT DEFAULT 50,
			reply_rate INT DEFAULT 50,
			emails_per_day INT DEFAULT 20,
			auto_reply BOOLEAN DEFAULT true,
			last_check TIMESTAMP,
			error_message TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(user_id, email)
		)
	`)
	// Add user_id column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE`)
	// Update unique constraint for multi-tenancy (drop old, add new)
	s.db.Exec(`ALTER TABLE warmup_seeds DROP CONSTRAINT IF EXISTS warmup_seeds_email_key`)
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS warmup_seeds_user_email_unique ON warmup_seeds(user_id, email)`)
	// Add new columns if they don't exist (migration)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS send_rate INT DEFAULT 50`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS reply_rate INT DEFAULT 50`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS emails_per_day INT DEFAULT 20`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS auto_reply BOOLEAN DEFAULT true`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS oauth_token TEXT`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS oauth_client_id TEXT`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS oauth_access_token TEXT`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS oauth_token_expires TIMESTAMP`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_seeds table: %v", err)
	}

	// Create warmup_emails table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_emails (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			warmup_smtp_id UUID NOT NULL REFERENCES warmup_smtps(id) ON DELETE CASCADE,
			seed_id UUID NOT NULL REFERENCES warmup_seeds(id) ON DELETE CASCADE,
			subject VARCHAR(500),
			message_id VARCHAR(255),
			status VARCHAR(20) DEFAULT 'sent',
			verified BOOLEAN DEFAULT false,
			landed_in_spam BOOLEAN DEFAULT false,
			moved_to_inbox BOOLEAN DEFAULT false,
			sent_at TIMESTAMP DEFAULT NOW(),
			opened_at TIMESTAMP,
			replied_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_emails table: %v", err)
	}

	// Add verified column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_emails ADD COLUMN IF NOT EXISTS verified BOOLEAN DEFAULT false`)

	// Add reply tracking columns to warmup_emails (migration)
	s.db.Exec(`ALTER TABLE warmup_emails ADD COLUMN IF NOT EXISTS reply_from VARCHAR(255)`)
	s.db.Exec(`ALTER TABLE warmup_emails ADD COLUMN IF NOT EXISTS reply_to VARCHAR(255)`)

	// Add total_sent column to warmup_seeds for tracking sent emails
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS total_sent INT DEFAULT 0`)

	// Add TLS mode columns to warmup_seeds (migration)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS imap_tls_mode VARCHAR(20) DEFAULT 'tls'`)      // tls, starttls, none
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS smtp_tls_mode VARCHAR(20) DEFAULT 'starttls'`) // tls, starttls, none

	// Add proxy columns to warmup_seeds (SOAX proxy support)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS proxy_enabled BOOLEAN DEFAULT false`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS proxy_host VARCHAR(255) DEFAULT 'proxy.soax.com'`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS proxy_port INT DEFAULT 9000`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS proxy_username VARCHAR(255)`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS proxy_password VARCHAR(255)`)

	// Add IMAP fields to smtp_senders for internal warmup (migration)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_host VARCHAR(255)`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_port INT DEFAULT 993`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_password VARCHAR(255)`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_use_tls BOOLEAN DEFAULT true`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_tls_mode VARCHAR(20) DEFAULT 'tls'`) // tls, starttls, none
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_status VARCHAR(20) DEFAULT 'unchecked'`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_last_check TIMESTAMP`)
	s.db.Exec(`ALTER TABLE smtp_senders ADD COLUMN IF NOT EXISTS imap_error TEXT`)

	// Create warmup_templates table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_templates (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			subject VARCHAR(500) NOT NULL,
			body TEXT NOT NULL,
			category VARCHAR(50) DEFAULT 'business',
			template_type VARCHAR(20) DEFAULT 'send',
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	// Add user_id column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_templates ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_templates table: %v", err)
	}

	// Add template_type column if not exists
	s.db.Exec(`ALTER TABLE warmup_templates ADD COLUMN IF NOT EXISTS template_type VARCHAR(20) DEFAULT 'send'`)

	// Create warmup_daily_stats table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_daily_stats (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			warmup_smtp_id UUID NOT NULL REFERENCES warmup_smtps(id) ON DELETE CASCADE,
			date DATE NOT NULL,
			scheduled INT DEFAULT 0,
			sent INT DEFAULT 0,
			inbox INT DEFAULT 0,
			spam INT DEFAULT 0,
			replies INT DEFAULT 0,
			reply_percent FLOAT DEFAULT 0,
			UNIQUE(warmup_smtp_id, date)
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_daily_stats table: %v", err)
	}

	// Create warmup_internal_emails table for internal SMTP-to-SMTP warmup
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_internal_emails (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			from_smtp_id UUID NOT NULL REFERENCES smtp_servers(id) ON DELETE CASCADE,
			to_smtp_id UUID NOT NULL REFERENCES smtp_servers(id) ON DELETE CASCADE,
			from_sender_email VARCHAR(255) NOT NULL,
			to_sender_id UUID REFERENCES smtp_senders(id) ON DELETE SET NULL,
			subject VARCHAR(500),
			message_id VARCHAR(255),
			status VARCHAR(20) DEFAULT 'sent',
			received BOOLEAN DEFAULT false,
			replied BOOLEAN DEFAULT false,
			sent_at TIMESTAMP DEFAULT NOW(),
			received_at TIMESTAMP,
			replied_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_internal_emails table: %v", err)
	}

	// Create warmup_seed_emails table for tracking Seed→SMTP emails
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_seed_emails (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			seed_id UUID NOT NULL REFERENCES warmup_seeds(id) ON DELETE CASCADE,
			to_email VARCHAR(255) NOT NULL,
			to_warmup_smtp_id UUID REFERENCES warmup_smtps(id) ON DELETE SET NULL,
			subject VARCHAR(500),
			message_id VARCHAR(255),
			status VARCHAR(20) DEFAULT 'sent',
			sent_at TIMESTAMP DEFAULT NOW(),
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_seed_emails table: %v", err)
	}

	// Create warmup_activity table for tracking events (moved to inbox, etc.)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_activity (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			activity_type VARCHAR(50) NOT NULL,
			email_id UUID,
			seed_id UUID REFERENCES warmup_seeds(id) ON DELETE SET NULL,
			warmup_smtp_id UUID REFERENCES warmup_smtps(id) ON DELETE SET NULL,
			from_email VARCHAR(255),
			to_email VARCHAR(255),
			subject VARCHAR(500),
			details TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	// Add user_id column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_activity ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_activity table: %v", err)
	}

	// Insert default warmup templates
	s.insertDefaultWarmupTemplates()

	// Create warmup_settings table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_settings (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			setting_key VARCHAR(100) NOT NULL,
			setting_value TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(user_id, setting_key)
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_settings table: %v", err)
	}
	// Add user_id column if missing (migration)
	s.db.Exec(`ALTER TABLE warmup_settings ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE`)
	// Update unique constraint for multi-tenancy
	s.db.Exec(`ALTER TABLE warmup_settings DROP CONSTRAINT IF EXISTS warmup_settings_setting_key_key`)
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS warmup_settings_user_key_unique ON warmup_settings(user_id, setting_key)`)

	// Insert default settings
	s.insertDefaultWarmupSettings()

	log.Println("[Warmup] Tables initialized")
}

func (s *Server) insertDefaultWarmupSettings() {
	defaultSettings := []struct {
		key         string
		value       string
		description string
	}{
		// Internal warmup settings
		{"internal_send_rate", "30", "Porcentagem de chance de envio por ciclo (0-100)"},
		{"internal_start_hour", "6", "Hora de início do envio (0-23)"},
		{"internal_end_hour", "22", "Hora de término do envio (0-23)"},
		{"internal_cycle_minutes", "2", "Intervalo entre ciclos em minutos"},
		{"internal_reply_rate", "40", "Porcentagem de chance de resposta automática (0-100)"},
		{"internal_mark_read_rate", "80", "Porcentagem de chance de marcar como lido (0-100)"},

		// External warmup settings (SMTP -> Seeds)
		{"external_send_rate", "50", "Porcentagem de chance de envio para seeds (0-100)"},
		{"external_start_hour", "8", "Hora de início do envio para seeds (0-23)"},
		{"external_end_hour", "18", "Hora de término do envio para seeds (0-23)"},
		{"external_cycle_minutes", "5", "Intervalo entre ciclos para seeds em minutos"},

		// General settings
		{"warmup_enabled", "true", "Ativar/desativar todo o sistema de warmup"},
		{"max_emails_per_smtp_per_day", "50", "Máximo de emails por SMTP por dia"},
		{"imap_check_interval", "5", "Intervalo de verificação IMAP em minutos"},
	}

	for _, setting := range defaultSettings {
		s.db.Exec(`
			INSERT INTO warmup_settings (setting_key, setting_value, description)
			VALUES ($1, $2, $3)
			ON CONFLICT (setting_key) DO NOTHING
		`, setting.key, setting.value, setting.description)
	}
}

func (s *Server) insertDefaultWarmupTemplates() {
	// Check if we need to add more templates (need at least 50 for good variety)
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates`).Scan(&count)
	if count >= 50 {
		return
	}

	// Clear old templates if we have too few
	if count > 0 && count < 50 {
		s.db.Exec(`DELETE FROM warmup_templates`)
		log.Println("[Warmup] Regenerating templates for more variety")
	}

	type template struct {
		subject      string
		body         string
		category     string
		templateType string
	}

	templates := []template{
		// ==========================================
		// SEND TEMPLATES - Business (30+)
		// ==========================================
		{"Duvida sobre seus servicos", "Ola,\n\nEncontrei sua empresa e gostaria de entrar em contato. Voce tem alguns minutos para discutir uma possivel colaboracao?\n\nAtenciosamente", "business", "send"},
		{"Retornando sobre nossa conversa", "Ola,\n\nGostaria de dar continuidade a nossa conversa anterior. Voce teve a chance de revisar as informacoes que enviei?\n\nAguardo seu retorno.", "business", "send"},
		{"Oportunidade de parceria", "Ola,\n\nAcredito que pode haver uma otima oportunidade para trabalharmos juntos. Voce estaria disponivel para uma breve conversa esta semana?\n\nObrigado!", "business", "send"},
		{"Solicitacao de reuniao", "Ola,\n\nGostaria de agendar uma reuniao para discutir algumas ideias. Qual seria sua disponibilidade na proxima semana?\n\nAbracos", "business", "send"},
		{"Apresentacao da empresa", "Ola,\n\nEstou entrando em contato porque acredito que podemos nos beneficiar dessa conexao. Me avise se tiver interesse em conversar.\n\nAtenciosamente", "business", "send"},
		{"Proposta comercial", "Ola,\n\nGostaria de apresentar uma proposta que pode ser interessante para sua empresa. Podemos agendar uma conversa?\n\nAguardo retorno.", "business", "send"},
		{"Acompanhamento do projeto", "Ola,\n\nEstou fazendo um acompanhamento sobre o projeto que discutimos. Alguma novidade do seu lado?\n\nObrigado!", "business", "send"},
		{"Informacoes solicitadas", "Ola,\n\nSegue as informacoes que voce solicitou. Qualquer duvida, estou a disposicao.\n\nAtenciosamente", "business", "send"},
		{"Feedback sobre proposta", "Ola,\n\nGostaria de saber se voce teve a oportunidade de analisar nossa proposta. Estou disponivel para esclarecer qualquer duvida.\n\nAbracos", "business", "send"},
		{"Convite para evento", "Ola,\n\nGostaria de convida-lo para um evento que estamos organizando. Seria uma otima oportunidade de networking.\n\nConte comigo!", "business", "send"},
		{"Parceria estrategica", "Ola,\n\nIdentifiquei uma oportunidade de parceria entre nossas empresas. Voce teria interesse em explorar isso?\n\nAguardo seu contato.", "business", "send"},
		{"Sobre o orcamento", "Ola,\n\nEstou entrando em contato sobre o orcamento que solicitou. Precisa de algum ajuste ou esclarecimento?\n\nAtenciosamente", "business", "send"},
		{"Novidades do mercado", "Ola,\n\nVi algumas novidades no mercado que podem impactar nosso setor. Gostaria de compartilhar e ouvir sua opiniao.\n\nAte mais!", "business", "send"},
		{"Renovacao de contrato", "Ola,\n\nO prazo do nosso contrato esta se aproximando. Podemos conversar sobre a renovacao?\n\nObrigado!", "business", "send"},
		{"Indicacao de servicos", "Ola,\n\nFui indicado por um colega para entrar em contato. Ele mencionou que voce poderia estar interessado em nossos servicos.\n\nPodemos conversar?", "business", "send"},

		// ==========================================
		// SEND TEMPLATES - Casual (30+)
		// ==========================================
		{"Ei, uma pergunta rapida", "Oi!\n\nEspero que esteja tudo bem. Tenho uma pergunta rapida - voce tem um momento?\n\nObrigado!", "casual", "send"},
		{"Passando para ver como esta", "Ola,\n\nSo queria saber como estao as coisas por ai. Me avise se precisar de algo!\n\nAbracos", "casual", "send"},
		{"Lembrei de voce", "Oi,\n\nVi algo hoje que me fez lembrar de voce. Espero que esteja tudo otimo!\n\nFalamos em breve", "casual", "send"},
		{"Faz tempo que nao conversamos", "Oi!\n\nFaz um tempo desde a ultima vez que nos falamos. Como voce tem estado? Adoraria colocar o papo em dia.\n\nAbracos", "casual", "send"},
		{"Atualizacao rapida", "Oi,\n\nSo queria te dar uma atualizacao rapida sobre as coisas. Me avise quando tiver alguns minutos para conversar.\n\nObrigado!", "casual", "send"},
		{"Tudo bem por ai?", "Oi!\n\nSo passando para saber se esta tudo bem. Qualquer coisa, estou por aqui!\n\nAbracos", "casual", "send"},
		{"Bom dia!", "Ola!\n\nBom dia! Espero que sua semana esteja sendo produtiva. Precisando de algo, e so falar!\n\nAte mais", "casual", "send"},
		{"Boa tarde!", "Oi!\n\nBoa tarde! Como estao as coisas? Espero que tudo esteja correndo bem por ai.\n\nAbracos", "casual", "send"},
		{"Pensando em voce", "Ola,\n\nPassei aqui so para dizer que lembrei de voce hoje. Espero que esteja bem!\n\nUm abraco", "casual", "send"},
		{"Novidades?", "Oi!\n\nAlguma novidade por ai? Faz um tempo que nao nos falamos. Conta as noticias!\n\nAbracos", "casual", "send"},
		{"Vamos marcar algo", "Ola,\n\nQue tal marcarmos um cafe ou uma conversa essa semana? Seria legal colocar o papo em dia.\n\nMe avise!", "casual", "send"},
		{"Recomendacao para voce", "Oi!\n\nVi algo que achei que voce ia gostar e resolvi compartilhar. Espero que seja util!\n\nAte mais", "casual", "send"},
		{"Como foi o fim de semana?", "Ola!\n\nEspero que seu fim de semana tenha sido otimo! Como estao as coisas?\n\nAbracos", "casual", "send"},
		{"Feliz aniversario!", "Oi!\n\nPassando para desejar um otimo dia! Que seja um ano cheio de realizacoes.\n\nUm abraco!", "casual", "send"},
		{"Boas festas!", "Ola!\n\nPassando para desejar boas festas! Que seja um periodo de descanso e alegria.\n\nAbracos", "casual", "send"},

		// ==========================================
		// SEND TEMPLATES - Newsletter (20+)
		// ==========================================
		{"Resumo semanal", "Ola,\n\nAqui esta seu resumo semanal de noticias e atualizacoes do setor. Confira os destaques abaixo.\n\nMantenha-se informado!", "newsletter", "send"},
		{"Nao perca", "Ola,\n\nTemos algumas novidades empolgantes para compartilhar com voce. De uma olhada quando puder!\n\nAtenciosamente", "newsletter", "send"},
		{"Seu resumo mensal", "Ola,\n\nAqui esta um resumo do que aconteceu este mes. Muito progresso foi feito!\n\nAbracos", "newsletter", "send"},
		{"Novos recursos disponiveis", "Ola,\n\nAcabamos de lancar alguns novos recursos que podem te interessar. Confira!\n\nObrigado pelo apoio continuo", "newsletter", "send"},
		{"Comunicado importante", "Ola,\n\nTemos um comunicado importante para compartilhar com voce. Por favor, reserve um momento para ler.\n\nObrigado!", "newsletter", "send"},
		{"Destaques da semana", "Ola,\n\nConfira os principais destaques desta semana. Preparamos um conteudo especial para voce.\n\nBoa leitura!", "newsletter", "send"},
		{"Dicas do mes", "Ola,\n\nSeparamos algumas dicas especiais para voce aproveitar este mes. Esperamos que sejam uteis!\n\nAte a proxima", "newsletter", "send"},
		{"Atualizacao de servicos", "Ola,\n\nGostaríamos de informar sobre algumas atualizacoes em nossos servicos. Confira os detalhes.\n\nAtenciosamente", "newsletter", "send"},
		{"Novidades do setor", "Ola,\n\nTrazemos as principais novidades do setor desta semana. Mantenha-se atualizado!\n\nBoa leitura", "newsletter", "send"},
		{"Promocao especial", "Ola,\n\nPreparamos uma promocao especial para voce. Aproveite!\n\nAte breve", "newsletter", "send"},
		{"Convite exclusivo", "Ola,\n\nVoce foi selecionado para receber um convite exclusivo. Confira os detalhes!\n\nAguardamos voce", "newsletter", "send"},
		{"Relatorio mensal", "Ola,\n\nSegue nosso relatorio mensal com as principais metricas e resultados. Esperamos que seja util.\n\nAtenciosamente", "newsletter", "send"},
		{"Tendencias do mercado", "Ola,\n\nConfira as principais tendencias do mercado que identificamos. Informacao valiosa para suas decisoes.\n\nBoa leitura!", "newsletter", "send"},
		{"Webinar gratuito", "Ola,\n\nGostaramos de convida-lo para nosso proximo webinar gratuito. Sera uma otima oportunidade de aprendizado.\n\nInscreva-se!", "newsletter", "send"},
		{"Pesquisa de satisfacao", "Ola,\n\nSua opiniao e muito importante para nos. Poderia responder uma breve pesquisa?\n\nObrigado!", "newsletter", "send"},

		// ==========================================
		// REPLY TEMPLATES (30+)
		// ==========================================
		{"Re: ", "Ola,\n\nObrigado pelo contato! Recebi sua mensagem e vou analisar com atencao.\n\nRetorno em breve!", "business", "reply"},
		{"Re: ", "Oi!\n\nQue bom receber sua mensagem! Vou verificar e te respondo o mais rapido possivel.\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço o email. Estou analisando as informacoes e em breve darei um retorno.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nRecebi! Vou dar uma olhada e ja te falo.\n\nValeu!", "casual", "reply"},
		{"Re: ", "Ola,\n\nMuito obrigado pela mensagem. Vou revisar o conteudo e responderei assim que possivel.\n\nAbracos", "business", "reply"},
		{"Re: ", "Oi!\n\nTudo bem? Recebi seu email e achei muito interessante. Vamos conversar mais sobre isso!\n\nAte ja", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço o envio. Vou avaliar com cuidado e retorno com uma resposta completa.\n\nObrigado!", "business", "reply"},
		{"Re: ", "Oi!\n\nQue otimo! Recebi sua mensagem. Me da um tempinho que ja te respondo direitinho.\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nRecebi seu email e agradeço pelo contato. Vou verificar internamente e volto com novidades.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nObrigado por escrever! Vou checar aqui e te dou um retorno.\n\nValeu!", "casual", "reply"},
		{"Re: ", "Ola,\n\nMuito obrigado pela informacao. Vou processar tudo e entro em contato em breve.\n\nAbracos", "business", "reply"},
		{"Re: ", "Oi!\n\nRecebi! Interessante o que voce mencionou. Vamos marcar para conversar?\n\nAte mais", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço muito o contato. Vou analisar a proposta e retorno com feedback.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nQue legal receber sua mensagem! Vou pensar sobre isso e te falo.\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nObrigado pelo email. As informacoes sao muito uteis. Darei um retorno em breve.\n\nObrigado!", "business", "reply"},
		{"Re: ", "Oi!\n\nRecebi sim! Valeu por lembrar de mim. Vamos nos falando!\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço seu contato. Encaminhei para a equipe responsavel e retornaremos em breve.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nTudo certo! Vi sua mensagem e achei bem interessante. Bora trocar uma ideia?\n\nAte ja", "casual", "reply"},
		{"Re: ", "Ola,\n\nMuito obrigado por entrar em contato. Vou revisar os detalhes e responderei assim que possivel.\n\nAbracos", "business", "reply"},
		{"Re: ", "Oi!\n\nAi sim! Recebi seu email. Deixa eu ver direitinho e ja te falo.\n\nValeu!", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço a mensagem. Estou verificando a disponibilidade e retorno com uma resposta.\n\nObrigado!", "business", "reply"},
		{"Re: ", "Oi!\n\nBoa! Recebi aqui. Vou analisar com calma e te dou um retorno.\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nRecebi sua solicitacao. Vou processar e entro em contato para dar andamento.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nShow! Vou checar o que voce mandou e ja te respondo.\n\nAte mais", "casual", "reply"},
		{"Re: ", "Ola,\n\nMuito obrigado pelo contato. Sua mensagem foi recebida e sera analisada.\n\nRetorno em breve!", "business", "reply"},
		{"Re: ", "Oi!\n\nQue bom ter noticias suas! Vou ver isso aqui e te falo.\n\nAbracos", "casual", "reply"},
		{"Re: ", "Ola,\n\nAgradeço o envio das informacoes. Vou revisar e darei um retorno completo.\n\nObrigado!", "business", "reply"},
		{"Re: ", "Oi!\n\nRecebi! Massa o que voce falou. Deixa eu pensar e te respondo.\n\nValeu!", "casual", "reply"},
		{"Re: ", "Ola,\n\nObrigado pela mensagem. Estou ciente do assunto e providenciarei o necessario.\n\nAtenciosamente", "business", "reply"},
		{"Re: ", "Oi!\n\nTudo bem! Recebi seu recado. Ja ja te dou um retorno!\n\nAbracos", "casual", "reply"},
	}

	for _, t := range templates {
		s.db.Exec(`
			INSERT INTO warmup_templates (id, subject, body, category, template_type, active)
			VALUES ($1, $2, $3, $4, $5, true)
		`, uuid.New().String(), t.subject, t.body, t.category, t.templateType)
	}

	log.Printf("[Warmup] Inserted %d default templates (send + reply)", len(templates))
}

// ============================================
// WARMUP SMTP HANDLERS
// ============================================

// listWarmupSMTPs returns all SMTPs enrolled in warmup
func (s *Server) listWarmupSMTPs(c *fiber.Ctx) error {
	userID := getUserID(c)
	rows, err := s.db.Query(`
		SELECT
			w.id, w.smtp_id, COALESCE(s.name, 'Unknown') as smtp_name,
			COALESCE(w.status, 'active'), COALESCE(w.recipe_type, 'progressive'),
			COALESCE(w.start_date, NOW()), w.end_date, COALESCE(w.current_day, 1),
			COALESCE(w.min_emails_per_day, 5), COALESCE(w.max_emails_per_day, 40),
			COALESCE(w.send_rate, 30), COALESCE(w.reply_rate, 30),
			COALESCE(w.start_hour, 8), COALESCE(w.end_hour, 18),
			COALESCE(w.total_sent, 0), COALESCE(w.total_inbox, 0),
			COALESCE(w.total_spam, 0), COALESCE(w.total_replies, 0),
			w.custom_schedule, COALESCE(w.internal_warmup, false),
			COALESCE(w.created_at, NOW()), COALESCE(w.updated_at, NOW())
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC
	`, userID)
	if err != nil {
		log.Printf("[Warmup API] listWarmupSMTPs query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var smtps []fiber.Map
	for rows.Next() {
		var id, smtpID, smtpName, status, recipeType string
		var startDate time.Time
		var endDate sql.NullTime
		var currentDay, minEmails, maxEmails, sendRate, replyRate, startHour, endHour int
		var totalSent, totalInbox, totalSpam, totalReplies int
		var customSchedule sql.NullString
		var internalWarmup bool
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&id, &smtpID, &smtpName, &status, &recipeType,
			&startDate, &endDate, &currentDay,
			&minEmails, &maxEmails, &sendRate, &replyRate,
			&startHour, &endHour,
			&totalSent, &totalInbox, &totalSpam, &totalReplies,
			&customSchedule, &internalWarmup, &createdAt, &updatedAt,
		)
		if err != nil {
			log.Printf("[Warmup API] listWarmupSMTPs scan error: %v", err)
			continue
		}

		// Get internal email count for this SMTP
		var internalSent int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_internal_emails WHERE from_smtp_id = $1
		`, smtpID).Scan(&internalSent)
		totalSent += internalSent

		smtp := fiber.Map{
			"id":                 id,
			"smtp_id":            smtpID,
			"smtp_name":          smtpName,
			"status":             status,
			"recipe_type":        recipeType,
			"start_date":         startDate,
			"current_day":        currentDay,
			"min_emails_per_day": minEmails,
			"max_emails_per_day": maxEmails,
			"send_rate":          sendRate,
			"reply_rate":         replyRate,
			"start_hour":         startHour,
			"end_hour":           endHour,
			"total_sent":         totalSent,
			"total_inbox":        totalInbox,
			"total_spam":         totalSpam,
			"total_replies":      totalReplies,
			"internal_warmup":    internalWarmup,
			"created_at":         createdAt,
			"updated_at":         updatedAt,
		}

		if endDate.Valid {
			smtp["end_date"] = endDate.Time
		}
		if customSchedule.Valid {
			smtp["custom_schedule"] = customSchedule.String
		}

		// Calculate inbox rate
		if totalSent > 0 {
			smtp["inbox_rate"] = float64(totalInbox) / float64(totalSent) * 100
			smtp["spam_rate"] = float64(totalSpam) / float64(totalSent) * 100
		} else {
			smtp["inbox_rate"] = 0.0
			smtp["spam_rate"] = 0.0
		}

		smtps = append(smtps, smtp)
	}

	if smtps == nil {
		smtps = []fiber.Map{}
	}

	return c.JSON(smtps)
}

// createWarmupSMTP adds an SMTP to warmup program
func (s *Server) createWarmupSMTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	var req struct {
		SMTPID          string `json:"smtp_id"`
		RecipeType      string `json:"recipe_type"`
		StartDate       string `json:"start_date"`
		EndDate         string `json:"end_date"`
		MinEmailsPerDay int    `json:"min_emails_per_day"`
		MaxEmailsPerDay int    `json:"max_emails_per_day"`
		SendRate        int    `json:"send_rate"`
		ReplyRate       int    `json:"reply_rate"`
		StartHour       int    `json:"start_hour"`
		EndHour         int    `json:"end_hour"`
		CustomSchedule  []int  `json:"custom_schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate SMTP exists and belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM smtp_servers WHERE id = $1 AND user_id = $2)`, req.SMTPID, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP not found"})
	}

	// Check if already enrolled
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM warmup_smtps WHERE smtp_id = $1 AND user_id = $2)`, req.SMTPID, userID).Scan(&exists)
	if exists {
		return c.Status(400).JSON(fiber.Map{"error": "SMTP already enrolled in warmup"})
	}

	// Set defaults
	if req.RecipeType == "" {
		req.RecipeType = "progressive"
	}
	if req.MinEmailsPerDay == 0 {
		req.MinEmailsPerDay = 5
	}
	if req.MaxEmailsPerDay == 0 {
		req.MaxEmailsPerDay = 40
	}
	if req.SendRate == 0 {
		req.SendRate = 30
	}
	if req.ReplyRate == 0 {
		req.ReplyRate = 30
	}
	if req.StartHour == 0 {
		req.StartHour = 8
	}
	if req.EndHour == 0 {
		req.EndHour = 18
	}

	id := uuid.New().String()

	// Parse dates
	startDate := time.Now()
	if req.StartDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.StartDate); err == nil {
			startDate = parsed
		}
	}

	var endDate *time.Time
	if req.EndDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.EndDate); err == nil {
			endDate = &parsed
		}
	}

	// Convert custom schedule to JSON
	var customScheduleJSON sql.NullString
	if len(req.CustomSchedule) > 0 {
		if jsonBytes, err := json.Marshal(req.CustomSchedule); err == nil {
			customScheduleJSON = sql.NullString{String: string(jsonBytes), Valid: true}
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO warmup_smtps (
			id, user_id, smtp_id, status, recipe_type, start_date, end_date,
			min_emails_per_day, max_emails_per_day, send_rate, reply_rate,
			start_hour, end_hour, custom_schedule
		) VALUES ($1, $2, $3, 'active', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, id, userID, req.SMTPID, req.RecipeType, startDate, endDate,
		req.MinEmailsPerDay, req.MaxEmailsPerDay, req.SendRate, req.ReplyRate,
		req.StartHour, req.EndHour, customScheduleJSON)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Generate initial schedule if progressive
	if req.RecipeType == "progressive" {
		s.generateProgressiveSchedule(id, req.MinEmailsPerDay, req.MaxEmailsPerDay, startDate, endDate)
	}

	return c.JSON(fiber.Map{"id": id, "message": "SMTP enrolled in warmup"})
}

// updateWarmupSMTP updates warmup settings for an SMTP
func (s *Server) updateWarmupSMTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var req struct {
		Status          string `json:"status"`
		RecipeType      string `json:"recipe_type"`
		MinEmailsPerDay int    `json:"min_emails_per_day"`
		MaxEmailsPerDay int    `json:"max_emails_per_day"`
		SendRate        int    `json:"send_rate"`
		ReplyRate       int    `json:"reply_rate"`
		StartHour       int    `json:"start_hour"`
		EndHour         int    `json:"end_hour"`
		CustomSchedule  []int  `json:"custom_schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Convert custom schedule to JSON
	var customScheduleJSON sql.NullString
	if len(req.CustomSchedule) > 0 {
		if jsonBytes, err := json.Marshal(req.CustomSchedule); err == nil {
			customScheduleJSON = sql.NullString{String: string(jsonBytes), Valid: true}
		}
	}

	_, err := s.db.Exec(`
		UPDATE warmup_smtps SET
			status = COALESCE(NULLIF($1, ''), status),
			recipe_type = COALESCE(NULLIF($2, ''), recipe_type),
			min_emails_per_day = CASE WHEN $3 > 0 THEN $3 ELSE min_emails_per_day END,
			max_emails_per_day = CASE WHEN $4 > 0 THEN $4 ELSE max_emails_per_day END,
			send_rate = CASE WHEN $5 > 0 THEN $5 ELSE send_rate END,
			reply_rate = CASE WHEN $6 > 0 THEN $6 ELSE reply_rate END,
			start_hour = CASE WHEN $7 >= 0 THEN $7 ELSE start_hour END,
			end_hour = CASE WHEN $8 > 0 THEN $8 ELSE end_hour END,
			custom_schedule = COALESCE($9, custom_schedule),
			updated_at = NOW()
		WHERE id = $10 AND user_id = $11
	`, req.Status, req.RecipeType, req.MinEmailsPerDay, req.MaxEmailsPerDay,
		req.SendRate, req.ReplyRate, req.StartHour, req.EndHour, customScheduleJSON, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Warmup settings updated"})
}

// deleteWarmupSMTP removes an SMTP from warmup program
func (s *Server) deleteWarmupSMTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_smtps WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "SMTP removed from warmup"})
}

// toggleWarmupSMTP toggles warmup on/off for an SMTP
func (s *Server) toggleWarmupSMTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var currentStatus string
	err := s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1 AND user_id = $2`, id, userID).Scan(&currentStatus)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	newStatus := "active"
	if currentStatus == "active" {
		newStatus = "paused"
	}

	s.db.Exec(`UPDATE warmup_smtps SET status = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`, newStatus, id, userID)

	return c.JSON(fiber.Map{"status": newStatus})
}

// triggerWarmupSMTP manually sends warmup emails for testing
func (s *Server) triggerWarmupSMTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get number of emails to send (default 1)
	count := c.QueryInt("count", 1)
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10 // Max 10 at a time for safety
	}

	// Get SMTP and warmup info
	var smtpID, recipeType string
	var host, username, password, tlsMode string
	var port, replyRate int

	err := s.db.QueryRow(`
		SELECT w.smtp_id, w.recipe_type, w.reply_rate,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.id = $1 AND w.user_id = $2
	`, id, userID).Scan(&smtpID, &recipeType, &replyRate, &host, &port, &username, &password, &tlsMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	// Check for active seeds (for this user)
	var activeSeeds int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active' AND user_id = $1`, userID).Scan(&activeSeeds)
	if activeSeeds == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No active seed accounts available"})
	}

	// Send emails
	sent := 0
	errors := []string{}
	for i := 0; i < count; i++ {
		err := s.sendWarmupEmailWithResult(id, smtpID, host, port, username, password, tlsMode, replyRate)
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			sent++
		}
	}

	result := fiber.Map{
		"sent":      sent,
		"requested": count,
		"message":   fmt.Sprintf("Sent %d of %d warmup emails", sent, count),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}

	return c.JSON(result)
}

// sendWarmupEmailWithResult sends a warmup email and returns error if any
func (s *Server) sendWarmupEmailWithResult(warmupID, smtpID, host string, port int, username, password, tlsMode string, replyRate int) error {
	// Get a random active seed
	var seedID, seedEmail string
	err := s.db.QueryRow(`
		SELECT id, email FROM warmup_seeds
		WHERE status = 'active'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&seedID, &seedEmail)

	if err != nil {
		return fmt.Errorf("no active seeds available")
	}

	// Get a random sender from smtp_senders (or fall back to username)
	var senderEmail string
	var senderName sql.NullString
	err = s.db.QueryRow(`
		SELECT email, name FROM smtp_senders
		WHERE smtp_id = $1 AND active = true
		ORDER BY RANDOM()
		LIMIT 1
	`, smtpID).Scan(&senderEmail, &senderName)

	if err != nil || senderEmail == "" {
		// Fall back to SMTP username if no senders configured
		senderEmail = username
	}

	// Format From address with name if available
	fromAddress := senderEmail
	if senderName.Valid && senderName.String != "" {
		fromAddress = fmt.Sprintf("%s <%s>", senderName.String, senderEmail)
	}

	// Get a random template (only 'send' type, not 'reply')
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true AND template_type = 'send'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&subject, &body)

	if err != nil {
		return fmt.Errorf("no templates available")
	}

	// Add some randomization to subject
	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

	// Generate message ID
	messageID := fmt.Sprintf("<%s@warmup>", uuid.New().String())

	// Send email via SMTP
	err = s.sendSMTPEmail(host, port, username, password, tlsMode, fromAddress, seedEmail, subject, body, messageID)
	if err != nil {
		return fmt.Errorf("SMTP error: %v", err)
	}

	// Record the email
	emailID := uuid.New().String()
	s.db.Exec(`
		INSERT INTO warmup_emails (id, warmup_smtp_id, seed_id, subject, message_id, status, sent_at)
		VALUES ($1, $2, $3, $4, $5, 'sent', NOW())
	`, emailID, warmupID, seedID, subject, messageID)

	// Update stats
	s.db.Exec(`
		UPDATE warmup_smtps SET total_sent = total_sent + 1, updated_at = NOW()
		WHERE id = $1
	`, warmupID)

	// Update daily stats
	s.updateWarmupDailyStats(warmupID, "sent")

	// Update sender stats
	s.db.Exec(`UPDATE smtp_senders SET total_sent = total_sent + 1 WHERE email = $1 AND smtp_id = $2`, senderEmail, smtpID)

	log.Printf("[Warmup Manual] ✉️ Sent from %s to %s: %s", senderEmail, seedEmail, subject)
	return nil
}

// getWarmupSMTPStats gets detailed stats for a warmup SMTP
func (s *Server) getWarmupSMTPStats(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Verify warmup SMTP belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM warmup_smtps WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	// Get daily stats for the last 30 days
	rows, err := s.db.Query(`
		SELECT date, scheduled, sent, inbox, spam, replies, reply_percent
		FROM warmup_daily_stats
		WHERE warmup_smtp_id = $1
		ORDER BY date DESC
		LIMIT 30
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var dailyStats []fiber.Map
	for rows.Next() {
		var date time.Time
		var scheduled, sent, inbox, spam, replies int
		var replyPercent float64

		rows.Scan(&date, &scheduled, &sent, &inbox, &spam, &replies, &replyPercent)

		dailyStats = append(dailyStats, fiber.Map{
			"date":          date.Format("2006-01-02"),
			"scheduled":     scheduled,
			"sent":          sent,
			"inbox":         inbox,
			"spam":          spam,
			"replies":       replies,
			"reply_percent": replyPercent,
		})
	}

	// Get recent emails (including internal warmup)
	emailRows, _ := s.db.Query(`
		(
			SELECT e.id, e.subject, e.status, e.landed_in_spam, e.sent_at, s.email as target_email, 'seed' as email_type
			FROM warmup_emails e
			JOIN warmup_seeds s ON e.seed_id = s.id
			WHERE e.warmup_smtp_id = $1
		)
		UNION ALL
		(
			SELECT ie.id, ie.subject, ie.status, false as landed_in_spam, ie.sent_at,
				   ss.email as target_email, 'internal' as email_type
			FROM warmup_internal_emails ie
			JOIN smtp_senders ss ON ie.to_sender_id = ss.id
			JOIN warmup_smtps w ON w.smtp_id = ie.from_smtp_id
			WHERE w.id = $1
		)
		ORDER BY sent_at DESC
		LIMIT 20
	`, id)
	defer emailRows.Close()

	var recentEmails []fiber.Map
	for emailRows.Next() {
		var emailID, subject, status, targetEmail, emailType string
		var landedInSpam bool
		var sentAt time.Time

		emailRows.Scan(&emailID, &subject, &status, &landedInSpam, &sentAt, &targetEmail, &emailType)

		recentEmails = append(recentEmails, fiber.Map{
			"id":             emailID,
			"subject":        subject,
			"status":         status,
			"landed_in_spam": landedInSpam,
			"sent_at":        sentAt,
			"seed_email":     targetEmail,
			"email_type":     emailType,
		})
	}

	// Get internal email count for this SMTP
	var internalSent int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_internal_emails ie
		JOIN warmup_smtps w ON w.smtp_id = ie.from_smtp_id
		WHERE w.id = $1
	`, id).Scan(&internalSent)

	return c.JSON(fiber.Map{
		"daily_stats":   dailyStats,
		"recent_emails": recentEmails,
		"internal_sent": internalSent,
	})
}

// updateWarmupSchedule updates the custom schedule (from draggable chart)
func (s *Server) updateWarmupSchedule(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var req struct {
		Schedule []int `json:"schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	jsonBytes, _ := json.Marshal(req.Schedule)

	_, err := s.db.Exec(`
		UPDATE warmup_smtps
		SET custom_schedule = $1, recipe_type = 'custom', updated_at = NOW()
		WHERE id = $2 AND user_id = $3
	`, string(jsonBytes), id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Schedule updated"})
}

// ============================================
// WARMUP SEED HANDLERS
// ============================================

// listWarmupSeeds returns all seed accounts with statistics
func (s *Server) listWarmupSeeds(c *fiber.Ctx) error {
	userID := getUserID(c)
	rows, err := s.db.Query(`
		SELECT
			ws.id, COALESCE(ws.email, ''), COALESCE(ws.provider, 'other'),
			COALESCE(ws.imap_host, ''), COALESCE(ws.imap_port, 993),
			COALESCE(ws.smtp_host, ''), COALESCE(ws.smtp_port, 587),
			COALESCE(ws.use_tls, true), COALESCE(ws.status, 'active'),
			ws.last_check, ws.error_message, COALESCE(ws.created_at, NOW()),
			COALESCE(ws.send_rate, 50), COALESCE(ws.reply_rate, 50),
			COALESCE(ws.emails_per_day, 20), COALESCE(ws.auto_reply, true),
			COALESCE(ws.imap_tls_mode, 'tls'), COALESCE(ws.smtp_tls_mode, 'starttls'),
			COALESCE(ws.total_sent, 0) + COALESCE(stats.total_replied, 0) as total_sent,
			COALESCE(stats.total_received, 0) as total_received,
			COALESCE(stats.total_inbox, 0) as total_inbox,
			COALESCE(stats.total_spam, 0) as total_spam,
			COALESCE(stats.total_moved, 0) as total_moved,
			COALESCE(stats.total_replied, 0) as total_replied,
			COALESCE(ws.proxy_enabled, false), COALESCE(ws.proxy_host, 'proxy.soax.com'),
			COALESCE(ws.proxy_port, 9000), COALESCE(ws.proxy_username, ''), COALESCE(ws.proxy_password, '')
		FROM warmup_seeds ws
		LEFT JOIN (
			SELECT
				seed_id,
				COUNT(CASE WHEN verified = true THEN 1 END) as total_received,
				COUNT(CASE WHEN verified = true AND landed_in_spam = false THEN 1 END) as total_inbox,
				COUNT(CASE WHEN verified = true AND landed_in_spam = true THEN 1 END) as total_spam,
				COUNT(CASE WHEN verified = true AND moved_to_inbox = true THEN 1 END) as total_moved,
				COUNT(CASE WHEN replied_at IS NOT NULL THEN 1 END) as total_replied
			FROM warmup_emails
			GROUP BY seed_id
		) stats ON ws.id = stats.seed_id
		WHERE ws.user_id = $1
		ORDER BY ws.created_at DESC
	`, userID)
	if err != nil {
		log.Printf("[Warmup API] listWarmupSeeds query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var seeds []fiber.Map
	for rows.Next() {
		var id, email, provider, imapHost, smtpHost, status string
		var imapPort, smtpPort int
		var useTLS, autoReply bool
		var sendRate, replyRate, emailsPerDay int
		var imapTLSMode, smtpTLSMode string
		var lastCheck sql.NullTime
		var errorMsg sql.NullString
		var createdAt time.Time
		var totalSent, totalReceived, totalInbox, totalSpam, totalMoved, totalReplied int
		var proxyEnabled bool
		var proxyHost, proxyUsername, proxyPassword string
		var proxyPort int

		err := rows.Scan(&id, &email, &provider, &imapHost, &imapPort, &smtpHost, &smtpPort,
			&useTLS, &status, &lastCheck, &errorMsg, &createdAt,
			&sendRate, &replyRate, &emailsPerDay, &autoReply,
			&imapTLSMode, &smtpTLSMode,
			&totalSent, &totalReceived, &totalInbox, &totalSpam, &totalMoved, &totalReplied,
			&proxyEnabled, &proxyHost, &proxyPort, &proxyUsername, &proxyPassword)
		if err != nil {
			log.Printf("[Warmup API] listWarmupSeeds scan error: %v", err)
			continue
		}

		seed := fiber.Map{
			"id":             id,
			"email":          email,
			"provider":       provider,
			"imap_host":      imapHost,
			"imap_port":      imapPort,
			"imap_tls_mode":  imapTLSMode,
			"smtp_host":      smtpHost,
			"smtp_port":      smtpPort,
			"smtp_tls_mode":  smtpTLSMode,
			"use_tls":        useTLS,
			"status":         status,
			"send_rate":      sendRate,
			"reply_rate":     replyRate,
			"emails_per_day": emailsPerDay,
			"auto_reply":     autoReply,
			"created_at":     createdAt,
			"total_sent":     totalSent,
			"total_received": totalReceived,
			"total_inbox":    totalInbox,
			"total_spam":     totalSpam,
			"total_moved":    totalMoved,
			"total_replied":  totalReplied,
			"proxy_enabled":  proxyEnabled,
			"proxy_host":     proxyHost,
			"proxy_port":     proxyPort,
			"proxy_username": proxyUsername,
			"proxy_password": proxyPassword,
		}

		if lastCheck.Valid {
			seed["last_check"] = lastCheck.Time
		}
		if errorMsg.Valid {
			seed["error_message"] = errorMsg.String
		}

		seeds = append(seeds, seed)
	}

	if seeds == nil {
		seeds = []fiber.Map{}
	}

	return c.JSON(seeds)
}

// createWarmupSeed adds a new seed account
func (s *Server) createWarmupSeed(c *fiber.Ctx) error {
	userID := getUserID(c)
	var req struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		Provider      string `json:"provider"`
		IMAPHost      string `json:"imap_host"`
		IMAPPort      int    `json:"imap_port"`
		IMAPTLSMode   string `json:"imap_tls_mode"` // tls, starttls, none
		SMTPHost      string `json:"smtp_host"`
		SMTPPort      int    `json:"smtp_port"`
		SMTPTLSMode   string `json:"smtp_tls_mode"` // tls, starttls, none
		SendRate      int    `json:"send_rate"`
		ReplyRate     int    `json:"reply_rate"`
		EmailsPerDay  int    `json:"emails_per_day"`
		AutoReply     *bool  `json:"auto_reply"`
		OAuthToken    string `json:"oauth_token"`     // Microsoft OAuth2 refresh token
		OAuthClientID string `json:"oauth_client_id"` // Microsoft OAuth2 client_id
		// SOAX Proxy settings
		ProxyEnabled  bool   `json:"proxy_enabled"`
		ProxyHost     string `json:"proxy_host"`
		ProxyPort     int    `json:"proxy_port"`
		ProxyUsername string `json:"proxy_username"`
		ProxyPassword string `json:"proxy_password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Auto-detect provider settings if not provided
	if req.Provider == "" || req.IMAPHost == "" {
		req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort = detectProviderSettings(req.Email)
	}

	if req.IMAPPort == 0 {
		req.IMAPPort = 993
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}

	// Set default values
	if req.SendRate == 0 {
		req.SendRate = 50
	}
	if req.ReplyRate == 0 {
		req.ReplyRate = 50
	}
	if req.EmailsPerDay == 0 {
		req.EmailsPerDay = 20
	}
	autoReply := true
	if req.AutoReply != nil {
		autoReply = *req.AutoReply
	}

	// Auto-detect TLS mode based on port if not specified
	if req.IMAPTLSMode == "" {
		if req.IMAPPort == 993 {
			req.IMAPTLSMode = "tls"
		} else if req.IMAPPort == 143 {
			req.IMAPTLSMode = "starttls"
		} else {
			req.IMAPTLSMode = "none"
		}
	}
	if req.SMTPTLSMode == "" {
		if req.SMTPPort == 465 {
			req.SMTPTLSMode = "tls"
		} else if req.SMTPPort == 587 || req.SMTPPort == 25 {
			req.SMTPTLSMode = "starttls"
		} else {
			req.SMTPTLSMode = "none"
		}
	}

	id := uuid.New().String()

	// Handle oauth fields - if provided, they might be empty or null
	var oauthToken, oauthClientID interface{}
	if req.OAuthToken != "" {
		oauthToken = req.OAuthToken
	}
	if req.OAuthClientID != "" {
		oauthClientID = req.OAuthClientID
	}

	// Set proxy defaults
	if req.ProxyHost == "" {
		req.ProxyHost = "proxy.soax.com"
	}
	if req.ProxyPort == 0 {
		req.ProxyPort = 9000
	}

	_, err := s.db.Exec(`
		INSERT INTO warmup_seeds (id, user_id, email, password, provider, imap_host, imap_port, imap_tls_mode,
			smtp_host, smtp_port, smtp_tls_mode, send_rate, reply_rate, emails_per_day, auto_reply, oauth_token, oauth_client_id,
			proxy_enabled, proxy_host, proxy_port, proxy_username, proxy_password, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, 'active')
	`, id, userID, req.Email, req.Password, req.Provider, req.IMAPHost, req.IMAPPort, req.IMAPTLSMode,
		req.SMTPHost, req.SMTPPort, req.SMTPTLSMode, req.SendRate, req.ReplyRate, req.EmailsPerDay, autoReply, oauthToken, oauthClientID,
		req.ProxyEnabled, req.ProxyHost, req.ProxyPort, req.ProxyUsername, req.ProxyPassword)

	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return c.Status(400).JSON(fiber.Map{"error": "Email already exists"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Test connection
	go s.testSeedConnection(id)

	return c.JSON(fiber.Map{"id": id, "message": "Seed account added"})
}

// updateWarmupSeed updates a seed account
func (s *Server) updateWarmupSeed(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var req struct {
		Password      string  `json:"password"`
		IMAPHost      string  `json:"imap_host"`
		IMAPPort      int     `json:"imap_port"`
		IMAPTLSMode   string  `json:"imap_tls_mode"`
		SMTPHost      string  `json:"smtp_host"`
		SMTPPort      int     `json:"smtp_port"`
		SMTPTLSMode   string  `json:"smtp_tls_mode"`
		SendRate      *int    `json:"send_rate"`
		ReplyRate     *int    `json:"reply_rate"`
		EmailsPerDay  *int    `json:"emails_per_day"`
		AutoReply     *bool   `json:"auto_reply"`
		ProxyEnabled  *bool   `json:"proxy_enabled"`
		ProxyHost     string  `json:"proxy_host"`
		ProxyPort     *int    `json:"proxy_port"`
		ProxyUsername *string `json:"proxy_username"`
		ProxyPassword *string `json:"proxy_password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Build dynamic update query
	query := `UPDATE warmup_seeds SET updated_at = NOW()`
	params := []interface{}{}
	paramIdx := 1

	if req.Password != "" {
		query += fmt.Sprintf(", password = $%d", paramIdx)
		params = append(params, req.Password)
		paramIdx++
	}
	if req.IMAPHost != "" {
		query += fmt.Sprintf(", imap_host = $%d", paramIdx)
		params = append(params, req.IMAPHost)
		paramIdx++
	}
	if req.IMAPPort > 0 {
		query += fmt.Sprintf(", imap_port = $%d", paramIdx)
		params = append(params, req.IMAPPort)
		paramIdx++
	}
	if req.IMAPTLSMode != "" {
		query += fmt.Sprintf(", imap_tls_mode = $%d", paramIdx)
		params = append(params, req.IMAPTLSMode)
		paramIdx++
	}
	if req.SMTPHost != "" {
		query += fmt.Sprintf(", smtp_host = $%d", paramIdx)
		params = append(params, req.SMTPHost)
		paramIdx++
	}
	if req.SMTPPort > 0 {
		query += fmt.Sprintf(", smtp_port = $%d", paramIdx)
		params = append(params, req.SMTPPort)
		paramIdx++
	}
	if req.SMTPTLSMode != "" {
		query += fmt.Sprintf(", smtp_tls_mode = $%d", paramIdx)
		params = append(params, req.SMTPTLSMode)
		paramIdx++
	}
	if req.SendRate != nil {
		query += fmt.Sprintf(", send_rate = $%d", paramIdx)
		params = append(params, *req.SendRate)
		paramIdx++
	}
	if req.ReplyRate != nil {
		query += fmt.Sprintf(", reply_rate = $%d", paramIdx)
		params = append(params, *req.ReplyRate)
		paramIdx++
	}
	if req.EmailsPerDay != nil && *req.EmailsPerDay > 0 {
		query += fmt.Sprintf(", emails_per_day = $%d", paramIdx)
		params = append(params, *req.EmailsPerDay)
		paramIdx++
	}
	if req.AutoReply != nil {
		query += fmt.Sprintf(", auto_reply = $%d", paramIdx)
		params = append(params, *req.AutoReply)
		paramIdx++
	}
	// Proxy settings
	if req.ProxyEnabled != nil {
		query += fmt.Sprintf(", proxy_enabled = $%d", paramIdx)
		params = append(params, *req.ProxyEnabled)
		paramIdx++
	}
	if req.ProxyHost != "" {
		query += fmt.Sprintf(", proxy_host = $%d", paramIdx)
		params = append(params, req.ProxyHost)
		paramIdx++
	}
	if req.ProxyPort != nil {
		query += fmt.Sprintf(", proxy_port = $%d", paramIdx)
		params = append(params, *req.ProxyPort)
		paramIdx++
	}
	if req.ProxyUsername != nil {
		query += fmt.Sprintf(", proxy_username = $%d", paramIdx)
		params = append(params, *req.ProxyUsername)
		paramIdx++
	}
	if req.ProxyPassword != nil {
		query += fmt.Sprintf(", proxy_password = $%d", paramIdx)
		params = append(params, *req.ProxyPassword)
		paramIdx++
	}

	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", paramIdx, paramIdx+1)
	params = append(params, id, userID)

	_, err := s.db.Exec(query, params...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Seed updated"})
}

// deleteSeed removes a seed account
func (s *Server) deleteWarmupSeed(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_seeds WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Seed removed"})
}

// testWarmupSeed tests IMAP connection for a seed (with OAuth2 support)
func (s *Server) testWarmupSeed(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var email, password, imapHost string
	var imapPort int
	var imapTLSMode, oauthToken, oauthClientID sql.NullString

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, COALESCE(imap_tls_mode, 'tls'), oauth_token, oauth_client_id
		FROM warmup_seeds WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&email, &password, &imapHost, &imapPort, &imapTLSMode, &oauthToken, &oauthClientID)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	tlsMode := "tls"
	if imapTLSMode.Valid && imapTLSMode.String != "" {
		tlsMode = imapTLSMode.String
	}

	oauth := ""
	if oauthToken.Valid && oauthToken.String != "" {
		oauth = oauthToken.String
	}

	clientID := ""
	if oauthClientID.Valid && oauthClientID.String != "" {
		clientID = oauthClientID.String
	}

	// Test IMAP connection with OAuth2 support
	err = testIMAPConnectionWithOAuth(imapHost, imapPort, email, password, tlsMode, oauth, clientID)
	if err != nil {
		// Set status to error and store the error message
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2 AND user_id = $3`,
			err.Error(), id, userID)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1 AND user_id = $2`, id, userID)

	return c.JSON(fiber.Map{"message": "Connection successful"})
}

// toggleWarmupSeed toggles a seed between active and paused
func (s *Server) toggleWarmupSeed(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var currentStatus string
	err := s.db.QueryRow(`SELECT status FROM warmup_seeds WHERE id = $1 AND user_id = $2`, id, userID).Scan(&currentStatus)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	newStatus := "paused"
	if currentStatus != "active" {
		newStatus = "active"
	}

	s.db.Exec(`UPDATE warmup_seeds SET status = $1 WHERE id = $2 AND user_id = $3`, newStatus, id, userID)

	return c.JSON(fiber.Map{"status": newStatus, "message": "Seed status updated"})
}

// verifyAllSeeds tests IMAP connection for all seeds and marks errors
func (s *Server) verifyAllSeeds(c *fiber.Ctx) error {
	userID := getUserID(c)
	// Get all seeds for this user
	rows, err := s.db.Query(`
		SELECT id, email, password, imap_host, imap_port,
		       COALESCE(imap_tls_mode, 'tls'),
		       COALESCE(oauth_token, ''),
		       COALESCE(oauth_client_id, '')
		FROM warmup_seeds
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get seeds"})
	}
	defer rows.Close()

	type seedInfo struct {
		ID            string
		Email         string
		Password      string
		IMAPHost      string
		IMAPPort      int
		TLSMode       string
		OAuthToken    string
		OAuthClientID string
	}

	var seeds []seedInfo
	for rows.Next() {
		var seed seedInfo
		rows.Scan(&seed.ID, &seed.Email, &seed.Password, &seed.IMAPHost, &seed.IMAPPort,
			&seed.TLSMode, &seed.OAuthToken, &seed.OAuthClientID)
		seeds = append(seeds, seed)
	}

	// Test each seed in parallel with a semaphore to limit concurrency
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit to 5 concurrent tests

	var successCount, errorCount int
	var mu sync.Mutex

	db := s.db // Capture db reference for goroutines
	for _, seed := range seeds {
		wg.Add(1)
		go func(seedData seedInfo) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			testErr := testIMAPConnectionWithOAuth(seedData.IMAPHost, seedData.IMAPPort, seedData.Email, seedData.Password, seedData.TLSMode, seedData.OAuthToken, seedData.OAuthClientID)

			mu.Lock()
			defer mu.Unlock()

			if testErr != nil {
				// Mark as error
				db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
					testErr.Error(), seedData.ID)
				errorCount++
				log.Printf("[Verify Seeds] ❌ %s: %v", seedData.Email, testErr)
			} else {
				// Mark as active (only if not already paused manually)
				db.Exec(`UPDATE warmup_seeds SET status = CASE WHEN status = 'paused' THEN 'paused' ELSE 'active' END, error_message = NULL, last_check = NOW() WHERE id = $1`, seedData.ID)
				successCount++
				log.Printf("[Verify Seeds] ✅ %s: OK", seedData.Email)
			}
		}(seed)
	}

	wg.Wait()

	return c.JSON(fiber.Map{
		"message":       "Verificação concluída",
		"total":         len(seeds),
		"success_count": successCount,
		"error_count":   errorCount,
	})
}

// deleteErrorSeeds deletes all seeds with error status
func (s *Server) deleteErrorSeeds(c *fiber.Ctx) error {
	userID := getUserID(c)
	// Count how many will be deleted
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'error' AND user_id = $1`, userID).Scan(&count)

	if count == 0 {
		return c.JSON(fiber.Map{"message": "Nenhuma seed com erro para excluir", "deleted_count": 0})
	}

	// Delete all seeds with error status
	result, err := s.db.Exec(`DELETE FROM warmup_seeds WHERE status = 'error' AND user_id = $1`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao excluir seeds"})
	}

	deleted, _ := result.RowsAffected()

	log.Printf("[Seeds] 🗑️ Deleted %d seeds with error status", deleted)

	return c.JSON(fiber.Map{
		"message":       fmt.Sprintf("Excluídas %d seeds com erro", deleted),
		"deleted_count": deleted,
	})
}

// triggerSeedSend forces the seed to send an email to a random SMTP
func (s *Server) triggerSeedSend(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get seed info including OAuth credentials
	var email, password, smtpHost string
	var smtpPort int
	var oauthToken, oauthClientID sql.NullString
	err := s.db.QueryRow(`
		SELECT email, password, smtp_host, smtp_port, oauth_token, oauth_client_id
		FROM warmup_seeds WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&email, &password, &smtpHost, &smtpPort, &oauthToken, &oauthClientID)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	if smtpHost == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Seed não tem SMTP configurado"})
	}

	// Get a random SMTP sender to send to (from user's warmup SMTPs)
	var targetEmail string
	err = s.db.QueryRow(`
		SELECT ss.email
		FROM warmup_smtps w
		JOIN smtp_senders ss ON ss.smtp_id = w.smtp_id
		WHERE w.status = 'active' AND ss.active = true AND w.user_id = $1
		ORDER BY RANDOM()
		LIMIT 1
	`, userID).Scan(&targetEmail)

	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo encontrado"})
	}

	// Get random template (only 'send' type, not 'reply') - prefer user's templates, fallback to system templates
	var subject, body string
	err = s.db.QueryRow(`SELECT subject, body FROM warmup_templates WHERE active = true AND template_type = 'send' AND (user_id = $1 OR user_id IS NULL) ORDER BY RANDOM() LIMIT 1`, userID).Scan(&subject, &body)
	if err != nil {
		// Fallback to any template
		s.db.QueryRow(`SELECT subject, body FROM warmup_templates WHERE active = true AND template_type = 'send' ORDER BY RANDOM() LIMIT 1`).Scan(&subject, &body)
	}

	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))
	messageID := fmt.Sprintf("<%s@seed-warmup>", uuid.New().String())

	tlsMode := "starttls"
	if smtpPort == 465 {
		tlsMode = "tls"
	}

	// Extract OAuth credentials
	oauth := ""
	clientID := ""
	if oauthToken.Valid && oauthToken.String != "" {
		oauth = oauthToken.String
	}
	if oauthClientID.Valid && oauthClientID.String != "" {
		clientID = oauthClientID.String
	}

	err = s.sendSMTPEmailWithOAuth(smtpHost, smtpPort, email, password, tlsMode, email, targetEmail, subject, body, messageID, oauth, clientID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Update counter
	s.db.Exec(`UPDATE warmup_seeds SET total_sent = total_sent + 1 WHERE id = $1 AND user_id = $2`, id, userID)

	log.Printf("[Warmup Seed Manual] ✉️ Sent from %s to %s: %s", email, targetEmail, subject)

	return c.JSON(fiber.Map{"message": "Email enviado com sucesso", "to": targetEmail})
}

// ============================================
// WARMUP STATS HANDLERS
// ============================================

// getWarmupStats returns overall warmup statistics
func (s *Server) getWarmupStats(c *fiber.Ctx) error {
	userID := getUserID(c)
	var totalSMTPs, activeSMTPs, totalSeeds, activeSeeds int
	var totalSent, totalInbox, totalSpam, totalReplies int

	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE user_id = $1`, userID).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active' AND user_id = $1`, userID).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE user_id = $1`, userID).Scan(&totalSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active' AND user_id = $1`, userID).Scan(&activeSeeds)

	s.db.QueryRow(`
		SELECT COALESCE(SUM(total_sent), 0), COALESCE(SUM(total_inbox), 0),
			   COALESCE(SUM(total_spam), 0), COALESCE(SUM(total_replies), 0)
		FROM warmup_smtps
		WHERE user_id = $1
	`, userID).Scan(&totalSent, &totalInbox, &totalSpam, &totalReplies)

	// Add internal warmup email counts (filter by user's SMTPs)
	var internalSent, internalReceived, internalReplies int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_internal_emails ie
		JOIN smtp_servers s ON ie.from_smtp_id = s.id
		WHERE s.user_id = $1
	`, userID).Scan(&internalSent)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_internal_emails ie
		JOIN smtp_servers s ON ie.from_smtp_id = s.id
		WHERE s.user_id = $1 AND ie.received = true
	`, userID).Scan(&internalReceived)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_internal_emails ie
		JOIN smtp_servers s ON ie.from_smtp_id = s.id
		WHERE s.user_id = $1 AND ie.replied = true
	`, userID).Scan(&internalReplies)
	totalSent += internalSent
	totalInbox += internalReceived // Internal received = inbox (found in INBOX, not spam)
	totalReplies += internalReplies

	var spamRate, inboxRate float64
	if totalSent > 0 {
		spamRate = float64(totalSpam) / float64(totalSent) * 100
		inboxRate = float64(totalInbox) / float64(totalSent) * 100
	}

	return c.JSON(fiber.Map{
		"total_smtps":        totalSMTPs,
		"active_smtps":       activeSMTPs,
		"total_seeds":        totalSeeds,
		"active_seeds":       activeSeeds,
		"total_sent":         totalSent,
		"total_interactions": totalInbox + totalReplies,
		"total_replies":      totalReplies,
		"spam_rate":          spamRate,
		"inbox_rate":         inboxRate,
	})
}

// getWarmupDiagnostic returns detailed diagnostic info to troubleshoot warmup issues
func (s *Server) getWarmupDiagnostic(c *fiber.Ctx) error {
	userID := getUserID(c)
	now := time.Now()
	currentHour := now.Hour()

	// Check warmup enabled (user-specific or global)
	warmupEnabled := s.getWarmupSettingForUser(userID, "warmup_enabled", "true")

	// Count active elements for this user
	var activeSeeds, activeTemplates, activeSMTPs, smtpsWithInternal int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active' AND user_id = $1`, userID).Scan(&activeSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates WHERE active = true AND (user_id = $1 OR user_id IS NULL)`, userID).Scan(&activeTemplates)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active' AND user_id = $1`, userID).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active' AND internal_warmup = true AND user_id = $1`, userID).Scan(&smtpsWithInternal)

	// SMTPs with active underlying smtp_server
	var smtpsWithActiveServer int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true AND w.user_id = $1
	`, userID).Scan(&smtpsWithActiveServer)

	// SMTPs in valid hours for external warmup
	var smtpsInValidHours int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true AND w.user_id = $1
		AND $2 >= w.start_hour AND $2 < w.end_hour
	`, userID, currentHour).Scan(&smtpsInValidHours)

	// SMTPs with internal warmup in valid hours
	var internalInValidHours int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true AND w.internal_warmup = true AND w.user_id = $1
		AND $2 >= w.start_hour AND $2 < w.end_hour
	`, userID, currentHour).Scan(&internalInValidHours)

	// Senders with IMAP configured (needed for internal warmup)
	var sendersWithIMAP int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM smtp_senders ss
		JOIN smtp_servers s ON ss.smtp_id = s.id
		JOIN warmup_smtps w ON w.smtp_id = s.id
		WHERE ss.active = true AND s.active = true AND w.status = 'active' AND w.internal_warmup = true AND w.user_id = $1
		AND ss.imap_host IS NOT NULL AND ss.imap_host != ''
	`, userID).Scan(&sendersWithIMAP)

	// Today's sent count for this user
	var sentTodayExternal, sentTodayInternal int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_emails e
		JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
		WHERE DATE(e.sent_at) = $1 AND w.user_id = $2
	`, now.Format("2006-01-02"), userID).Scan(&sentTodayExternal)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_internal_emails ie
		JOIN smtp_servers s ON ie.from_smtp_id = s.id
		WHERE DATE(ie.sent_at) = $1 AND s.user_id = $2
	`, now.Format("2006-01-02"), userID).Scan(&sentTodayInternal)

	// Build issues list
	issues := []string{}

	if warmupEnabled != "true" {
		issues = append(issues, "Warmup está DESABILITADO globalmente")
	}

	// External warmup issues
	if activeSeeds == 0 {
		issues = append(issues, "SMTP→Seed: Nenhuma conta Seed ativa")
	}
	if activeTemplates == 0 {
		issues = append(issues, "Nenhum template de email ativo")
	}
	if activeSMTPs == 0 {
		issues = append(issues, "Nenhum SMTP em warmup ativo")
	}
	if smtpsWithActiveServer == 0 && activeSMTPs > 0 {
		issues = append(issues, "SMTPs em warmup não têm servidor SMTP subjacente ativo")
	}
	if smtpsInValidHours == 0 && smtpsWithActiveServer > 0 {
		issues = append(issues, fmt.Sprintf("SMTP→Seed: Nenhum SMTP no horário de envio (hora atual: %d)", currentHour))
	}

	// Internal warmup issues
	if smtpsWithInternal < 2 {
		issues = append(issues, fmt.Sprintf("SMTP→SMTP: Precisa de pelo menos 2 SMTPs com 'Interno' ativado (tem %d)", smtpsWithInternal))
	}
	if internalInValidHours < 2 && smtpsWithInternal >= 2 {
		issues = append(issues, fmt.Sprintf("SMTP→SMTP: Menos de 2 SMTPs no horário válido (hora atual: %d)", currentHour))
	}
	if sendersWithIMAP == 0 && smtpsWithInternal >= 2 {
		issues = append(issues, "SMTP→SMTP: Nenhum sender tem IMAP configurado (necessário para receber)")
	}

	// Get SMTP details
	type smtpDetail struct {
		Host            string `json:"host"`
		Status          string `json:"status"`
		Internal        bool   `json:"internal"`
		StartHour       int    `json:"start_hour"`
		EndHour         int    `json:"end_hour"`
		InHours         bool   `json:"in_hours"`
		TodayLimit      int    `json:"today_limit"`
		SentToday       int    `json:"sent_today"`
		ServerActive    bool   `json:"server_active"`
		SendersCount    int    `json:"senders_count"`
		SendersWithIMAP int    `json:"senders_with_imap"`
	}

	smtpRows, _ := s.db.Query(`
		SELECT s.host, w.status, w.internal_warmup, w.start_hour, w.end_hour,
		       w.min_emails_per_day, w.max_emails_per_day, w.recipe_type, w.start_date,
		       s.active, s.id
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.user_id = $1
	`, userID)
	defer smtpRows.Close()

	var smtpDetails []smtpDetail
	for smtpRows.Next() {
		var host, status, recipeType, serverID string
		var internal, serverActive bool
		var startHour, endHour, minEmails, maxEmails int
		var startDate time.Time
		smtpRows.Scan(&host, &status, &internal, &startHour, &endHour,
			&minEmails, &maxEmails, &recipeType, &startDate, &serverActive, &serverID)

		// Calculate current day based on calendar days (not hours)
		startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		currentDay := int(today.Sub(startDay).Hours()/24) + 1
		if currentDay < 1 {
			currentDay = 1
		}
		todayLimit := s.calculateDailyLimit(recipeType, currentDay, minEmails, maxEmails, "")

		var sentToday, sendersCount, sendersWithIMAPCount int
		s.db.QueryRow(`SELECT COUNT(*) FROM warmup_internal_emails WHERE from_smtp_id = $1 AND DATE(sent_at) = $2`, serverID, now.Format("2006-01-02")).Scan(&sentToday)
		s.db.QueryRow(`SELECT COUNT(*) FROM smtp_senders WHERE smtp_id = $1 AND active = true`, serverID).Scan(&sendersCount)
		s.db.QueryRow(`SELECT COUNT(*) FROM smtp_senders WHERE smtp_id = $1 AND active = true AND imap_host IS NOT NULL AND imap_host != ''`, serverID).Scan(&sendersWithIMAPCount)

		inHours := currentHour >= startHour && currentHour < endHour

		smtpDetails = append(smtpDetails, smtpDetail{
			Host:            host,
			Status:          status,
			Internal:        internal,
			StartHour:       startHour,
			EndHour:         endHour,
			InHours:         inHours,
			TodayLimit:      todayLimit,
			SentToday:       sentToday,
			ServerActive:    serverActive,
			SendersCount:    sendersCount,
			SendersWithIMAP: sendersWithIMAPCount,
		})
	}

	return c.JSON(fiber.Map{
		"timestamp":      now.Format("2006-01-02 15:04:05"),
		"current_hour":   currentHour,
		"warmup_enabled": warmupEnabled == "true",
		"external_warmup": fiber.Map{
			"active_seeds":     activeSeeds,
			"active_templates": activeTemplates,
			"active_smtps":     smtpsWithActiveServer,
			"smtps_in_hours":   smtpsInValidHours,
			"sent_today":       sentTodayExternal,
		},
		"internal_warmup": fiber.Map{
			"smtps_with_internal": smtpsWithInternal,
			"smtps_in_hours":      internalInValidHours,
			"senders_with_imap":   sendersWithIMAP,
			"sent_today":          sentTodayInternal,
		},
		"smtp_details": smtpDetails,
		"issues":       issues,
		"ok":           len(issues) == 0,
	})
}

// triggerWarmup manually triggers a warmup cycle for testing
func (s *Server) triggerWarmup(c *fiber.Ctx) error {
	var req struct {
		Type string `json:"type"` // "external", "internal", or "both"
	}
	if err := c.BodyParser(&req); err != nil {
		req.Type = "both"
	}

	results := fiber.Map{}

	if req.Type == "external" || req.Type == "both" {
		go s.processWarmupEmails()
		results["external"] = "triggered"
	}

	if req.Type == "internal" || req.Type == "both" {
		go s.processInternalWarmup()
		results["internal"] = "triggered"
	}

	return c.JSON(fiber.Map{
		"message": "Warmup cycles triggered - check logs for results",
		"results": results,
	})
}

// getWarmupActivity returns recent warmup activity (including all types)
func (s *Server) getWarmupActivity(c *fiber.Ctx) error {
	userID := getUserID(c)
	// Query all warmup activity types:
	// 1. SMTP → Seed emails (warmup_emails)
	// 2. Replies (warmup_emails with replied_at)
	// 3. Internal SMTP → SMTP (warmup_internal_emails)
	// 4. Seed → SMTP emails (warmup_seed_emails)
	// 5. Activity events like "moved to inbox" (warmup_activity)
	rows, err := s.db.Query(`
		(
			SELECT DISTINCT ON (e.id) e.id::text, COALESCE(e.subject, ''), COALESCE(e.status, 'sent'), e.sent_at,
				   COALESCE(
				       (SELECT email FROM smtp_senders WHERE smtp_id = sm.id AND active = true LIMIT 1),
				       sm.username,
				       ''
				   ) as from_email,
				   COALESCE(s.email, '') as to_email,
				   'smtp_to_seed' as warmup_type
			FROM warmup_emails e
			JOIN warmup_seeds s ON e.seed_id = s.id
			JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
			JOIN smtp_servers sm ON w.smtp_id = sm.id
			WHERE (e.status != 'replied' OR e.replied_at IS NULL) AND w.user_id = $1
		)
		UNION ALL
		(
			SELECT e.id::text || '-reply' as id, 'Re: ' || COALESCE(e.subject, '') as subject, 'replied' as status, e.replied_at as sent_at,
				   COALESCE(e.reply_from, s.email, '') as from_email,
				   COALESCE(e.reply_to, '') as to_email,
				   'reply' as warmup_type
			FROM warmup_emails e
			JOIN warmup_seeds s ON e.seed_id = s.id
			JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
			WHERE e.replied_at IS NOT NULL AND w.user_id = $1
		)
		UNION ALL
		(
			SELECT ie.id::text, COALESCE(ie.subject, ''), COALESCE(ie.status, 'sent'), ie.sent_at,
				   COALESCE(ie.from_sender_email, '') as from_email,
				   COALESCE(ss.email, '') as to_email,
				   'internal' as warmup_type
			FROM warmup_internal_emails ie
			LEFT JOIN smtp_senders ss ON ie.to_sender_id = ss.id
			JOIN smtp_servers sm ON ie.from_smtp_id = sm.id
			WHERE sm.user_id = $1
		)
		UNION ALL
		(
			SELECT se.id::text, COALESCE(se.subject, ''), COALESCE(se.status, 'sent'), se.sent_at,
				   COALESCE(s.email, '') as from_email,
				   COALESCE(se.to_email, '') as to_email,
				   'seed_to_smtp' as warmup_type
			FROM warmup_seed_emails se
			JOIN warmup_seeds s ON se.seed_id = s.id
			WHERE s.user_id = $1
		)
		UNION ALL
		(
			SELECT a.id::text, COALESCE(a.subject, a.activity_type, ''), a.activity_type as status, a.created_at as sent_at,
				   COALESCE(a.from_email, '') as from_email,
				   COALESCE(a.to_email, '') as to_email,
				   a.activity_type as warmup_type
			FROM warmup_activity a
			WHERE a.user_id = $1
		)
		ORDER BY sent_at DESC
		LIMIT 50
	`, userID)
	if err != nil {
		log.Printf("[Warmup API] getWarmupActivity main query error: %v", err)
		// Fallback to simpler query without new tables
		rows, err = s.db.Query(`
			(
				SELECT DISTINCT ON (e.id) e.id::text, COALESCE(e.subject, ''), COALESCE(e.status, 'sent'), e.sent_at,
					   COALESCE(
					       (SELECT email FROM smtp_senders WHERE smtp_id = sm.id AND active = true LIMIT 1),
					       sm.username,
					       ''
					   ) as from_email,
					   COALESCE(s.email, '') as to_email,
					   'smtp_to_seed' as warmup_type
				FROM warmup_emails e
				JOIN warmup_seeds s ON e.seed_id = s.id
				JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
				JOIN smtp_servers sm ON w.smtp_id = sm.id
				WHERE (e.status != 'replied' OR e.replied_at IS NULL) AND w.user_id = $1
			)
			UNION ALL
			(
				SELECT e.id::text || '-reply' as id, 'Re: ' || COALESCE(e.subject, '') as subject, 'replied' as status, e.replied_at as sent_at,
					   COALESCE(e.reply_from, s.email, '') as from_email,
					   COALESCE(e.reply_to, '') as to_email,
					   'reply' as warmup_type
				FROM warmup_emails e
				JOIN warmup_seeds s ON e.seed_id = s.id
				JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
				WHERE e.replied_at IS NOT NULL AND w.user_id = $1
			)
			ORDER BY sent_at DESC
			LIMIT 50
		`, userID)
		if err != nil {
			log.Printf("[Warmup API] getWarmupActivity fallback query error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	defer rows.Close()

	var activities []fiber.Map
	for rows.Next() {
		var id, subject, status, fromEmail, toEmail, warmupType string
		var sentAt time.Time

		rows.Scan(&id, &subject, &status, &sentAt, &fromEmail, &toEmail, &warmupType)

		activities = append(activities, fiber.Map{
			"id":          id,
			"subject":     subject,
			"status":      status,
			"sent_at":     sentAt,
			"from_email":  fromEmail,
			"to_email":    toEmail,
			"warmup_type": warmupType,
		})
	}

	if activities == nil {
		activities = []fiber.Map{}
	}

	return c.JSON(activities)
}

// ============================================
// WARMUP TEMPLATES HANDLERS
// ============================================

// listWarmupTemplates returns all warmup templates
func (s *Server) listWarmupTemplates(c *fiber.Ctx) error {
	userID := getUserID(c)
	rows, err := s.db.Query(`
		SELECT id, COALESCE(subject, ''), COALESCE(body, ''),
		       COALESCE(category, 'business'), COALESCE(template_type, 'send'),
		       COALESCE(active, true), COALESCE(created_at, NOW())
		FROM warmup_templates
		WHERE user_id = $1 OR user_id IS NULL
		ORDER BY template_type, category, created_at
	`, userID)
	if err != nil {
		log.Printf("[Warmup API] listWarmupTemplates query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var templates []fiber.Map
	for rows.Next() {
		var id, subject, body, category, templateType string
		var active bool
		var createdAt time.Time

		err := rows.Scan(&id, &subject, &body, &category, &templateType, &active, &createdAt)
		if err != nil {
			log.Printf("[Warmup API] listWarmupTemplates scan error: %v", err)
			continue
		}

		templates = append(templates, fiber.Map{
			"id":            id,
			"subject":       subject,
			"body":          body,
			"category":      category,
			"template_type": templateType,
			"active":        active,
			"created_at":    createdAt,
		})
	}

	if templates == nil {
		templates = []fiber.Map{}
	}

	return c.JSON(templates)
}

// createWarmupTemplate creates a new warmup template
func (s *Server) createWarmupTemplate(c *fiber.Ctx) error {
	userID := getUserID(c)
	var req struct {
		Subject      string `json:"subject"`
		Body         string `json:"body"`
		Category     string `json:"category"`
		TemplateType string `json:"template_type"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Category == "" {
		req.Category = "business"
	}
	if req.TemplateType == "" {
		req.TemplateType = "send"
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO warmup_templates (id, user_id, subject, body, category, template_type, active)
		VALUES ($1, $2, $3, $4, $5, $6, true)
	`, id, userID, req.Subject, req.Body, req.Category, req.TemplateType)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"id": id, "message": "Template created"})
}

// deleteWarmupTemplate deletes a warmup template
func (s *Server) deleteWarmupTemplate(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Only allow deleting user's own templates (not system templates where user_id IS NULL)
	_, err := s.db.Exec(`DELETE FROM warmup_templates WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Template deleted"})
}

// triggerIMAPCheck forces an immediate IMAP check on all seeds
func (s *Server) triggerIMAPCheck(c *fiber.Ctx) error {
	log.Println("[Warmup IMAP] Manual check triggered")
	go s.processIMAPInteractions()
	return c.JSON(fiber.Map{"message": "Verificação IMAP iniciada"})
}

// ============================================
// HELPER FUNCTIONS
// ============================================

func detectProviderSettings(email string) (provider, imapHost string, imapPort int, smtpHost string, smtpPort int) {
	domain := strings.ToLower(strings.Split(email, "@")[1])

	switch {
	case strings.Contains(domain, "gmail") || strings.Contains(domain, "googlemail"):
		return "gmail", "imap.gmail.com", 993, "smtp.gmail.com", 587
	case strings.Contains(domain, "yahoo"):
		return "yahoo", "imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 587
	case strings.Contains(domain, "outlook") || strings.Contains(domain, "hotmail") || strings.Contains(domain, "live"):
		return "outlook", "outlook.office365.com", 993, "smtp.office365.com", 587
	case strings.Contains(domain, "aol"):
		return "aol", "imap.aol.com", 993, "smtp.aol.com", 587
	case strings.Contains(domain, "icloud") || strings.Contains(domain, "me.com") || strings.Contains(domain, "mac.com"):
		return "icloud", "imap.mail.me.com", 993, "smtp.mail.me.com", 587
	case strings.Contains(domain, "gmx"):
		return "gmx", "imap.gmx.com", 993, "mail.gmx.com", 587
	default:
		return "other", "imap." + domain, 993, "smtp." + domain, 587
	}
}

// testIMAPConnection tests IMAP connection with different TLS modes
// tlsMode can be: "tls" (implicit TLS), "starttls", or "none"
func testIMAPConnection(host string, port int, email, password string, useTLS bool) error {
	return testIMAPConnectionWithMode(host, port, email, password, getTLSModeFromBool(useTLS, port))
}

func getTLSModeFromBool(useTLS bool, port int) string {
	if useTLS || port == 993 {
		return "tls"
	}
	if port == 143 {
		return "starttls"
	}
	return "none"
}

// Microsoft OAuth2 constants
const (
	// Microsoft Office public client ID (works with personal accounts)
	msOAuthClientID = "d3590ed6-52b3-4102-aeff-aad2292ab01c"
	// Use /common endpoint for both personal and work accounts
	msOAuthTokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	msOAuthScope    = "https://outlook.office.com/IMAP.AccessAsUser.All https://outlook.office.com/SMTP.Send offline_access"
)

// MicrosoftTokenResponse represents the OAuth2 token response from Microsoft
type MicrosoftTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// refreshMicrosoftAccessToken uses a refresh token to get a new access token
// If customClientID is provided, it will be used instead of the default
func refreshMicrosoftAccessToken(refreshToken, customClientID string) (*MicrosoftTokenResponse, error) {
	clientID := msOAuthClientID
	if customClientID != "" {
		clientID = customClientID
	}

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")
	data.Set("scope", msOAuthScope)

	req, err := http.NewRequest("POST", msOAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var tokenResp MicrosoftTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("OAuth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &tokenResp, nil
}

// xoauth2Client implements sasl.Client for XOAUTH2
type xoauth2Client struct {
	Username    string
	AccessToken string
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	// Format: "user=" + email + "\x01auth=Bearer " + accessToken + "\x01\x01"
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", c.Username, c.AccessToken)
	return "XOAUTH2", []byte(authStr), nil
}

func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	// XOAUTH2 doesn't have additional challenges in normal flow
	// If we get here, it's likely an error response from the server
	return nil, fmt.Errorf("XOAUTH2 challenge: %s", string(challenge))
}

// newXOAuth2Client creates a new XOAUTH2 SASL client
func newXOAuth2Client(username, accessToken string) sasl.Client {
	return &xoauth2Client{Username: username, AccessToken: accessToken}
}

// authenticateIMAPWithXOAuth2 authenticates using XOAUTH2
func authenticateIMAPWithXOAuth2(c *client.Client, email, accessToken string) error {
	saslClient := newXOAuth2Client(email, accessToken)
	if err := c.Authenticate(saslClient); err != nil {
		return fmt.Errorf("XOAUTH2 auth failed: %v", err)
	}
	return nil
}

// testIMAPConnectionWithOAuth tests IMAP connection with OAuth2 support
func testIMAPConnectionWithOAuth(host string, port int, email, password, tlsMode, oauthToken, oauthClientID string) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	var c *client.Client
	var err error

	dialFunc := func() (*client.Client, error) {
		switch tlsMode {
		case "tls":
			return client.DialTLS(addr, &tls.Config{
				ServerName:         host,
				InsecureSkipVerify: true,
			})
		case "starttls":
			conn, err := client.Dial(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to connect: %v", err)
			}
			tlsConfig := &tls.Config{
				ServerName:         host,
				InsecureSkipVerify: true,
			}
			if err = conn.StartTLS(tlsConfig); err != nil {
				conn.Logout()
				return nil, fmt.Errorf("STARTTLS failed: %v", err)
			}
			return conn, nil
		default:
			return client.Dial(addr)
		}
	}

	c, err = dialFunc()
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	// If we have OAuth token AND client_id, try OAuth2 first
	if oauthToken != "" && oauthClientID != "" {
		log.Printf("[OAuth] Trying OAuth2 with custom client_id: %s...", oauthClientID[:8])
		tokenResp, oauthErr := refreshMicrosoftAccessToken(oauthToken, oauthClientID)
		if oauthErr == nil {
			// Try XOAUTH2 authentication
			if authErr := authenticateIMAPWithXOAuth2(c, email, tokenResp.AccessToken); authErr == nil {
				log.Printf("[OAuth] XOAUTH2 authentication successful!")
				return nil
			} else {
				log.Printf("[OAuth] XOAUTH2 failed: %v, trying password...", authErr)
			}
		} else {
			log.Printf("[OAuth] Token refresh failed: %v, trying password...", oauthErr)
		}

		// Reconnect for password attempt (connection may be in bad state)
		c.Logout()
		c, err = dialFunc()
		if err != nil {
			return fmt.Errorf("failed to reconnect: %v", err)
		}
	}

	// Try password login (works with App Passwords)
	if err := c.Login(email, password); err != nil {
		// Check if this is Outlook/Hotmail
		domain := strings.ToLower(email)
		isOutlook := strings.Contains(domain, "outlook") || strings.Contains(domain, "hotmail") ||
			strings.Contains(domain, "live.") || strings.Contains(domain, "msn.")

		if isOutlook {
			if oauthToken != "" && oauthClientID != "" {
				return fmt.Errorf("LOGIN e OAuth2 falharam: %v. Verifique se o token e client_id estão corretos", err)
			}
			return fmt.Errorf("LOGIN failed: %v. Para Outlook/Hotmail: use App Password ou forneça token+client_id OAuth2", err)
		}
		return fmt.Errorf("login failed: %v", err)
	}

	return nil
}

func testIMAPConnectionWithMode(host string, port int, email, password, tlsMode string) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	var c *client.Client
	var err error

	switch tlsMode {
	case "tls":
		// Implicit TLS (port 993)
		c, err = client.DialTLS(addr, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		})
	case "starttls":
		// STARTTLS (typically port 143)
		c, err = client.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		// Upgrade to TLS
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		if err = c.StartTLS(tlsConfig); err != nil {
			c.Logout()
			return fmt.Errorf("STARTTLS failed: %v", err)
		}
	default: // "none"
		// Plain connection without TLS
		c, err = client.Dial(addr)
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	if err := c.Login(email, password); err != nil {
		return fmt.Errorf("login failed: %v", err)
	}

	return nil
}

func (s *Server) testSeedConnection(seedID string) {
	var email, password, imapHost string
	var imapPort int
	var imapTLSMode, oauthToken, oauthClientID sql.NullString

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, COALESCE(imap_tls_mode, 'tls'), oauth_token, oauth_client_id
		FROM warmup_seeds WHERE id = $1
	`, seedID).Scan(&email, &password, &imapHost, &imapPort, &imapTLSMode, &oauthToken, &oauthClientID)

	if err != nil {
		return
	}

	tlsMode := "tls"
	if imapTLSMode.Valid && imapTLSMode.String != "" {
		tlsMode = imapTLSMode.String
	}

	// Use OAuth2 connection if token is available
	oauth := ""
	if oauthToken.Valid && oauthToken.String != "" {
		oauth = oauthToken.String
	}

	clientID := ""
	if oauthClientID.Valid && oauthClientID.String != "" {
		clientID = oauthClientID.String
	}

	err = testIMAPConnectionWithOAuth(imapHost, imapPort, email, password, tlsMode, oauth, clientID)
	if err != nil {
		// Set status to error and store the error message
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
	} else {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1`, seedID)
	}
}

// testSeedConnectionPreview tests IMAP/SMTP connection WITHOUT saving (for preview before creating)
func (s *Server) testSeedConnectionPreview(c *fiber.Ctx) error {
	var req struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		IMAPHost      string `json:"imap_host"`
		IMAPPort      int    `json:"imap_port"`
		IMAPTLSMode   string `json:"imap_tls_mode"` // tls, starttls, none
		SMTPHost      string `json:"smtp_host"`
		SMTPPort      int    `json:"smtp_port"`
		SMTPTLSMode   string `json:"smtp_tls_mode"`   // tls, starttls, none
		TestType      string `json:"test_type"`       // imap, smtp, both
		OAuthToken    string `json:"oauth_token"`     // Microsoft OAuth2 refresh token
		OAuthClientID string `json:"oauth_client_id"` // Microsoft OAuth2 client_id
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Requisição inválida"})
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email e senha são obrigatórios"})
	}

	results := fiber.Map{}

	// Test IMAP if requested
	if req.TestType == "imap" || req.TestType == "both" || req.TestType == "" {
		if req.IMAPHost == "" {
			results["imap"] = fiber.Map{"success": false, "error": "Host IMAP não informado"}
		} else {
			if req.IMAPPort == 0 {
				req.IMAPPort = 993
			}
			if req.IMAPTLSMode == "" {
				req.IMAPTLSMode = getTLSModeFromBool(true, req.IMAPPort)
			}

			// Use OAuth connection if token provided
			err := testIMAPConnectionWithOAuth(req.IMAPHost, req.IMAPPort, req.Email, req.Password, req.IMAPTLSMode, req.OAuthToken, req.OAuthClientID)
			if err != nil {
				results["imap"] = fiber.Map{"success": false, "error": err.Error()}
			} else {
				if req.OAuthToken != "" && req.OAuthClientID != "" {
					results["imap"] = fiber.Map{"success": true, "message": "Conexão IMAP com OAuth2 OK!"}
				} else {
					results["imap"] = fiber.Map{"success": true, "message": "Conexão IMAP OK!"}
				}
			}
		}
	}

	// Test SMTP if requested
	if req.TestType == "smtp" || req.TestType == "both" {
		if req.SMTPHost == "" {
			results["smtp"] = fiber.Map{"success": false, "error": "Host SMTP não informado"}
		} else {
			if req.SMTPPort == 0 {
				req.SMTPPort = 587
			}
			if req.SMTPTLSMode == "" {
				if req.SMTPPort == 465 {
					req.SMTPTLSMode = "tls"
				} else {
					req.SMTPTLSMode = "starttls"
				}
			}

			// Test SMTP connection with OAuth if available
			err := s.testSMTPConnectionWithOAuth(req.SMTPHost, req.SMTPPort, req.Email, req.Password, req.SMTPTLSMode, req.OAuthToken, req.OAuthClientID)
			if err != nil {
				results["smtp"] = fiber.Map{"success": false, "error": err.Error()}
			} else {
				if req.OAuthToken != "" && req.OAuthClientID != "" {
					results["smtp"] = fiber.Map{"success": true, "message": "Conexão SMTP com OAuth2 OK!"}
				} else {
					results["smtp"] = fiber.Map{"success": true, "message": "Conexão SMTP OK!"}
				}
			}
		}
	}

	return c.JSON(results)
}

// testSMTPConnection tests SMTP connection without sending email (legacy, no OAuth)
func (s *Server) testSMTPConnection(host string, port int, username, password, tlsMode string) error {
	return s.testSMTPConnectionWithOAuth(host, port, username, password, tlsMode, "", "")
}

// testSMTPConnectionWithOAuth tests SMTP connection with OAuth2 support
func (s *Server) testSMTPConnectionWithOAuth(host string, port int, username, password, tlsMode, oauthToken, oauthClientID string) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Helper function to authenticate with OAuth or password
	authenticateWithOAuth := func(client *smtp.Client) error {
		// If we have OAuth token AND client_id, try OAuth2 first
		if oauthToken != "" && oauthClientID != "" {
			log.Printf("[SMTP OAuth] Trying OAuth2 for %s with client_id: %s...", username, oauthClientID[:8])

			// Get access token from refresh token
			tokenResp, oauthErr := refreshMicrosoftAccessToken(oauthToken, oauthClientID)
			if oauthErr == nil && tokenResp.AccessToken != "" {
				log.Printf("[SMTP OAuth] Got access token, trying XOAUTH2...")

				// Try XOAUTH2 authentication
				auth := newXOAuth2SMTPAuth(username, tokenResp.AccessToken)
				if err := client.Auth(auth); err == nil {
					log.Printf("[SMTP OAuth] XOAUTH2 authentication successful!")
					return nil
				} else {
					log.Printf("[SMTP OAuth] XOAUTH2 failed: %v, trying password...", err)
				}
			} else {
				log.Printf("[SMTP OAuth] Token refresh failed: %v, trying password...", oauthErr)
			}
		}

		// Fall back to password authentication
		return s.authenticateSMTP(client, host, username, password)
	}

	switch tlsMode {
	case "tls":
		// Implicit TLS
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("TLS connection failed: %v", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("SMTP client error: %v", err)
		}
		defer client.Close()

		if err := authenticateWithOAuth(client); err != nil {
			return err
		}
		return client.Quit()

	case "starttls":
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("Connection failed: %v", err)
		}

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP client error: %v", err)
		}
		defer client.Close()

		if err := client.Hello("[127.0.0.1]"); err != nil {
			return fmt.Errorf("EHLO error: %v", err)
		}

		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS failed: %v", err)
		}

		if err := authenticateWithOAuth(client); err != nil {
			return err
		}
		return client.Quit()

	default: // "none"
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("Connection failed: %v", err)
		}

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP client error: %v", err)
		}
		defer client.Close()

		if err := client.Hello("[127.0.0.1]"); err != nil {
			return fmt.Errorf("EHLO error: %v", err)
		}

		if err := authenticateWithOAuth(client); err != nil {
			return err
		}
		return client.Quit()
	}
}

func (s *Server) generateProgressiveSchedule(warmupID string, minEmails, maxEmails int, startDate time.Time, endDate *time.Time) {
	// Calculate number of days
	days := 45
	if endDate != nil {
		days = int(endDate.Sub(startDate).Hours() / 24)
	}

	// Generate progressive schedule
	schedule := make([]int, days)
	increment := float64(maxEmails-minEmails) / float64(days-1)

	for i := 0; i < days; i++ {
		schedule[i] = minEmails + int(float64(i)*increment)
		if schedule[i] > maxEmails {
			schedule[i] = maxEmails
		}
	}

	jsonBytes, _ := json.Marshal(schedule)
	s.db.Exec(`UPDATE warmup_smtps SET custom_schedule = $1 WHERE id = $2`, string(jsonBytes), warmupID)
}

// ============================================
// WARMUP ENGINE (Background Process)
// ============================================

// safeRun executes a function with panic recovery and timeout tracking
func (s *Server) safeRun(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Warmup Engine] PANIC in %s: %v", name, r)
		}
	}()

	start := time.Now()
	log.Printf("[Warmup Engine] Starting: %s", name)
	fn()
	log.Printf("[Warmup Engine] Completed: %s (took %v)", name, time.Since(start))
}

func (s *Server) startWarmupEngine() {
	log.Println("[Warmup Engine] Starting...")

	// Run all tasks immediately on startup (in goroutines)
	// DISABLED: IMAP check causes too many simultaneous connections to Outlook/Hotmail
	// go s.safeRun("Initial IMAP Check", s.processIMAPInteractions)
	go s.safeRun("Initial Internal Warmup", s.processInternalWarmup)
	go s.safeRun("Initial Internal IMAP", s.processInternalWarmupIMAP)
	go s.safeRun("Initial External Warmup", s.processWarmupEmails)
	go s.safeRun("Initial Seed→SMTP", s.processSeedToSMTPEmails)

	// Get configurable intervals from settings
	imapInterval := s.getWarmupSettingInt("imap_check_interval", 5)
	internalCycleInterval := s.getWarmupSettingInt("internal_cycle_minutes", 2)

	log.Printf("[Warmup Engine] Intervals: External=1m, Seed→SMTP=3m, Internal=%dm, InternalIMAP=%dm (Seed IMAP disabled)", internalCycleInterval, imapInterval)

	// Create all tickers
	externalTicker := time.NewTicker(1 * time.Minute)
	// DISABLED: IMAP check causes too many simultaneous connections to Outlook/Hotmail
	// imapTicker := time.NewTicker(time.Duration(imapInterval) * time.Minute)
	seedToSMTPTicker := time.NewTicker(3 * time.Minute)
	internalWarmupTicker := time.NewTicker(time.Duration(internalCycleInterval) * time.Minute)
	internalImapTicker := time.NewTicker(time.Duration(imapInterval) * time.Minute)

	defer externalTicker.Stop()
	// defer imapTicker.Stop() // DISABLED
	defer seedToSMTPTicker.Stop()
	defer internalWarmupTicker.Stop()
	defer internalImapTicker.Stop()

	log.Printf("[Warmup Engine] Started successfully - all tickers running...")

	// Keep track of last run time to detect stuck tickers
	lastTick := time.Now()

	for {
		select {
		case <-externalTicker.C:
			lastTick = time.Now()
			go s.safeRun("External Warmup (SMTP→Seeds)", s.processWarmupEmails)

		// DISABLED: IMAP check causes too many simultaneous connections to Outlook/Hotmail
		// case <-imapTicker.C:
		// 	lastTick = time.Now()
		// 	go s.safeRun("IMAP Interactions", s.processIMAPInteractions)

		case <-seedToSMTPTicker.C:
			lastTick = time.Now()
			go s.safeRun("Seed→SMTP Emails", s.processSeedToSMTPEmails)

		case <-internalWarmupTicker.C:
			lastTick = time.Now()
			go s.safeRun("Internal Warmup (SMTP→SMTP)", s.processInternalWarmup)

		case <-internalImapTicker.C:
			lastTick = time.Now()
			go s.safeRun("Internal IMAP Check", s.processInternalWarmupIMAP)

		default:
			// Safety check - if no tick for 10 minutes, log warning
			if time.Since(lastTick) > 10*time.Minute {
				log.Printf("[Warmup Engine] WARNING: No tick received for %v", time.Since(lastTick))
				lastTick = time.Now()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *Server) processWarmupEmails() {
	// FIRST: Check if any SMTPs are active - if all paused, skip entire cycle
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	if activeSMTPs == 0 {
		// Silent skip when all paused - no log spam
		return
	}

	now := time.Now()
	currentHour := now.Hour()

	// Log active counts
	var activeSeeds, activeTemplates int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates WHERE active = true`).Scan(&activeTemplates)

	log.Printf("[Warmup Engine] Processing at %s (hour %d) - Active: %d SMTPs, %d Seeds, %d Templates",
		now.Format("15:04:05"), currentHour, activeSMTPs, activeSeeds, activeTemplates)

	if activeSeeds == 0 {
		log.Printf("[Warmup Engine] No active seeds available - cannot send warmup emails")
		return
	}
	if activeTemplates == 0 {
		log.Printf("[Warmup Engine] No active templates available - cannot send warmup emails")
		return
	}

	// Get active warmup SMTPs that should send now (includes user_id for isolation)
	rows, err := s.db.Query(`
		SELECT w.id, w.smtp_id, w.user_id, w.start_date, w.min_emails_per_day, w.max_emails_per_day,
			   w.recipe_type, w.custom_schedule, w.reply_rate, w.start_hour, w.end_hour,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active'
		AND s.active = true
	`)
	if err != nil {
		log.Printf("[Warmup Engine] Error querying SMTPs: %v", err)
		return
	}
	defer rows.Close()

	smtpCount := 0
	for rows.Next() {
		smtpCount++
		var warmupID, smtpID, userID, recipeType string
		var startDate time.Time
		var minEmails, maxEmails, replyRate, startHour, endHour int
		var customSchedule sql.NullString
		var host, username, password, tlsMode string
		var port int

		rows.Scan(&warmupID, &smtpID, &userID, &startDate, &minEmails, &maxEmails,
			&recipeType, &customSchedule, &replyRate, &startHour, &endHour,
			&host, &port, &username, &password, &tlsMode)

		// Calculate current day based on calendar days (not hours)
		startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		currentDay := int(today.Sub(startDay).Hours()/24) + 1
		if currentDay < 1 {
			currentDay = 1
		}

		// Update current_day in database for UI display
		s.db.Exec(`UPDATE warmup_smtps SET current_day = $1 WHERE id = $2`, currentDay, warmupID)

		log.Printf("[Warmup Engine] SMTP %s: Day %d, hours %d-%d, current hour %d", warmupID, currentDay, startHour, endHour, currentHour)

		// Check if within sending hours
		if currentHour < startHour || currentHour >= endHour {
			log.Printf("[Warmup Engine] SMTP %s: Outside sending hours, skipping", warmupID)
			continue
		}

		// Calculate today's limit
		todayLimit := s.calculateDailyLimit(recipeType, currentDay, minEmails, maxEmails, customSchedule.String)

		// Get how many already sent today
		var sentToday int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_emails
			WHERE warmup_smtp_id = $1 AND DATE(sent_at) = $2
		`, warmupID, now.Format("2006-01-02")).Scan(&sentToday)

		log.Printf("[Warmup Engine] SMTP %s: Today's limit %d, sent so far %d", warmupID, todayLimit, sentToday)

		if sentToday >= todayLimit {
			log.Printf("[Warmup Engine] SMTP %s: Daily limit reached, skipping", warmupID)
			continue
		}

		// Calculate how many to send this minute (spread throughout remaining time)
		remaining := todayLimit - sentToday
		sendingHours := endHour - startHour
		if sendingHours <= 0 {
			sendingHours = 1
		}

		// Calculate remaining minutes in the sending window
		currentMinuteInWindow := (currentHour-startHour)*60 + time.Now().Minute()
		totalSendingMinutes := sendingHours * 60
		remainingMinutes := totalSendingMinutes - currentMinuteInWindow
		if remainingMinutes <= 0 {
			remainingMinutes = 1
		}

		// Use remaining minutes to calculate rate (so it catches up if behind)
		emailsPerMinute := (remaining + remainingMinutes - 1) / remainingMinutes // Round up division
		if emailsPerMinute < 1 && remaining > 0 {
			// For low volume, use probability
			probability := float64(remaining) / float64(remainingMinutes)
			if rand.Float64() < probability {
				emailsPerMinute = 1
			}
		}

		if emailsPerMinute > remaining {
			emailsPerMinute = remaining
		}

		// Send warmup emails (check status before each send to allow immediate pause)
		log.Printf("[Warmup Engine] SMTP %s: Sending %d emails this minute (remaining: %d)", warmupID, emailsPerMinute, remaining)
		for i := 0; i < emailsPerMinute; i++ {
			// Re-check status before each email (allows immediate stop when paused)
			var currentStatus string
			s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1`, warmupID).Scan(&currentStatus)
			if currentStatus != "active" {
				log.Printf("[Warmup Engine] SMTP %s paused mid-cycle - stopping", warmupID[:8])
				break
			}
			s.sendWarmupEmail(warmupID, smtpID, userID, host, port, username, password, tlsMode, replyRate)
		}
	}

	if smtpCount == 0 {
		log.Printf("[Warmup Engine] No active warmup SMTPs found (check if warmup_smtps.status='active' AND smtp_servers.active=true)")
	} else {
		log.Printf("[Warmup Engine] Processed %d warmup SMTPs", smtpCount)
	}
}

func (s *Server) calculateDailyLimit(recipeType string, currentDay, minEmails, maxEmails int, customSchedule string) int {
	switch recipeType {
	case "flat":
		return minEmails
	case "randomized":
		return minEmails + rand.Intn(maxEmails-minEmails+1)
	case "custom":
		if customSchedule != "" {
			var schedule []int
			if err := json.Unmarshal([]byte(customSchedule), &schedule); err == nil {
				if currentDay > 0 && currentDay <= len(schedule) {
					return schedule[currentDay-1]
				}
			}
		}
		fallthrough
	default: // progressive
		increment := float64(maxEmails-minEmails) / 45.0
		limit := minEmails + int(float64(currentDay-1)*increment)
		if limit > maxEmails {
			limit = maxEmails
		}
		return limit
	}
}

func (s *Server) sendWarmupEmail(warmupID, smtpID, userID, host string, port int, username, password, tlsMode string, replyRate int) {
	// Check if this specific warmup SMTP is still active (allows immediate stop)
	var status string
	s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1`, warmupID).Scan(&status)
	if status != "active" {
		log.Printf("[Warmup] SMTP %s is %s - skipping", warmupID[:8], status)
		return
	}

	// Get a random active seed FROM THE SAME USER (isolation)
	var seedID, seedEmail string
	err := s.db.QueryRow(`
		SELECT id, email FROM warmup_seeds
		WHERE status = 'active' AND user_id = $1
		ORDER BY RANDOM()
		LIMIT 1
	`, userID).Scan(&seedID, &seedEmail)

	if err != nil {
		log.Printf("[Warmup] No active seeds available")
		return
	}

	// Get a random sender from smtp_senders (or fall back to username)
	var senderEmail string
	var senderName sql.NullString
	err = s.db.QueryRow(`
		SELECT email, name FROM smtp_senders
		WHERE smtp_id = $1 AND active = true
		ORDER BY RANDOM()
		LIMIT 1
	`, smtpID).Scan(&senderEmail, &senderName)

	if err != nil || senderEmail == "" {
		// Fall back to SMTP username if no senders configured
		senderEmail = username
		log.Printf("[Warmup] No senders configured for SMTP, using username: %s", username)
	}

	// Format From address with name if available
	fromAddress := senderEmail
	if senderName.Valid && senderName.String != "" {
		fromAddress = fmt.Sprintf("%s <%s>", senderName.String, senderEmail)
	}

	// Get a random template (only 'send' type, not 'reply')
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true AND template_type = 'send'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&subject, &body)

	if err != nil {
		log.Printf("[Warmup] No templates available")
		return
	}

	// Add some randomization to subject
	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

	// Generate message ID
	messageID := fmt.Sprintf("<%s@warmup>", uuid.New().String())

	// Send email via SMTP
	err = s.sendSMTPEmail(host, port, username, password, tlsMode, fromAddress, seedEmail, subject, body, messageID)
	if err != nil {
		log.Printf("[Warmup] Failed to send from %s to %s: %v", senderEmail, seedEmail, err)
		return
	}

	// Record the email
	emailID := uuid.New().String()
	s.db.Exec(`
		INSERT INTO warmup_emails (id, warmup_smtp_id, seed_id, subject, message_id, status, sent_at)
		VALUES ($1, $2, $3, $4, $5, 'sent', NOW())
	`, emailID, warmupID, seedID, subject, messageID)

	// Update stats
	s.db.Exec(`
		UPDATE warmup_smtps SET total_sent = total_sent + 1, updated_at = NOW()
		WHERE id = $1
	`, warmupID)

	// Update daily stats
	s.updateWarmupDailyStats(warmupID, "sent")

	// Update sender stats
	s.db.Exec(`UPDATE smtp_senders SET total_sent = total_sent + 1 WHERE email = $1 AND smtp_id = $2`, senderEmail, smtpID)

	log.Printf("[Warmup] ✉️ Sent from %s to %s: %s", senderEmail, seedEmail, subject)
}

// sendSMTPEmailWithOAuth sends email with OAuth2 support for Outlook
func (s *Server) sendSMTPEmailWithOAuth(host string, port int, username, password, tlsMode, from, to, subject, body, messageID, oauthToken, oauthClientID string) error {
	// Extract email from "Name <email>" format if present
	fromEmail := from
	fromHeader := from
	if strings.Contains(from, "<") && strings.Contains(from, ">") {
		start := strings.Index(from, "<") + 1
		end := strings.Index(from, ">")
		if start > 0 && end > start {
			fromEmail = from[start:end]
		}
		nameEnd := strings.Index(from, "<")
		if nameEnd > 0 {
			name := strings.TrimSpace(from[:nameEnd])
			if needsEncoding(name) {
				fromHeader = mimeEncode(name) + " <" + fromEmail + ">"
			}
		}
	}

	encodedSubject := subject
	if needsEncoding(subject) {
		encodedSubject = mimeEncode(subject)
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+
		"Date: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		"\r\n"+
		"%s", fromHeader, to, encodedSubject, messageID, time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"), encodeBase64WithLineBreaks([]byte(body)))

	addr := fmt.Sprintf("%s:%d", host, port)

	// Handle different TLS modes with OAuth support
	switch tlsMode {
	case "tls":
		return s.sendWithImplicitTLSOAuth(addr, host, username, password, fromEmail, to, []byte(msg), oauthToken, oauthClientID)
	case "starttls":
		return s.sendWithSTARTTLSOAuth(addr, host, username, password, fromEmail, to, []byte(msg), oauthToken, oauthClientID)
	default:
		return s.sendPlainSMTPOAuth(addr, host, username, password, fromEmail, to, []byte(msg), oauthToken, oauthClientID)
	}
}

// sendSMTPEmailWithProxy sends email through a SOCKS5 proxy (for SOAX integration)
func (s *Server) sendSMTPEmailWithProxy(smtpHost string, smtpPort int, username, password, tlsMode, from, to, subject, body, messageID string,
	proxyHost string, proxyPort int, proxyUsername, proxyPassword string) error {

	// Extract email from "Name <email>" format if present
	fromEmail := from
	fromHeader := from
	if strings.Contains(from, "<") && strings.Contains(from, ">") {
		start := strings.Index(from, "<") + 1
		end := strings.Index(from, ">")
		if start > 0 && end > start {
			fromEmail = from[start:end]
		}
		nameEnd := strings.Index(from, "<")
		if nameEnd > 0 {
			name := strings.TrimSpace(from[:nameEnd])
			if needsEncoding(name) {
				fromHeader = mimeEncode(name) + " <" + fromEmail + ">"
			}
		}
	}

	encodedSubject := subject
	if needsEncoding(subject) {
		encodedSubject = mimeEncode(subject)
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+
		"Date: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		"\r\n"+
		"%s", fromHeader, to, encodedSubject, messageID, time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"), encodeBase64WithLineBreaks([]byte(body)))

	// Create SOCKS5 dialer with authentication
	proxyAddr := fmt.Sprintf("%s:%d", proxyHost, proxyPort)
	var auth *proxy.Auth
	if proxyUsername != "" {
		auth = &proxy.Auth{
			User:     proxyUsername,
			Password: proxyPassword,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 dialer: %v", err)
	}

	smtpAddr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	// Connect through proxy based on TLS mode
	switch tlsMode {
	case "tls":
		// Implicit TLS (port 465)
		conn, err := dialer.Dial("tcp", smtpAddr)
		if err != nil {
			return fmt.Errorf("proxy connection failed: %v", err)
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         smtpHost,
			InsecureSkipVerify: true,
		})
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return fmt.Errorf("TLS handshake failed: %v", err)
		}
		c, err := smtp.NewClient(tlsConn, smtpHost)
		if err != nil {
			tlsConn.Close()
			return fmt.Errorf("SMTP client creation failed: %v", err)
		}
		defer c.Close()
		return s.sendEmailViaClient(c, smtpHost, username, password, fromEmail, to, []byte(msg))

	case "starttls":
		// STARTTLS (port 587)
		conn, err := dialer.Dial("tcp", smtpAddr)
		if err != nil {
			return fmt.Errorf("proxy connection failed: %v", err)
		}
		c, err := smtp.NewClient(conn, smtpHost)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP client creation failed: %v", err)
		}
		defer c.Close()
		if err := c.StartTLS(&tls.Config{
			ServerName:         smtpHost,
			InsecureSkipVerify: true,
		}); err != nil {
			return fmt.Errorf("STARTTLS failed: %v", err)
		}
		return s.sendEmailViaClient(c, smtpHost, username, password, fromEmail, to, []byte(msg))

	default:
		// Plain (no TLS)
		conn, err := dialer.Dial("tcp", smtpAddr)
		if err != nil {
			return fmt.Errorf("proxy connection failed: %v", err)
		}
		c, err := smtp.NewClient(conn, smtpHost)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP client creation failed: %v", err)
		}
		defer c.Close()
		return s.sendEmailViaClient(c, smtpHost, username, password, fromEmail, to, []byte(msg))
	}
}

// sendEmailViaClient sends email using an established SMTP client connection
func (s *Server) sendEmailViaClient(c *smtp.Client, host, username, password, from, to string, msg []byte) error {
	// Authenticate
	auth := smtp.PlainAuth("", username, password, host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	// Set sender
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %v", err)
	}

	// Set recipient
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO failed: %v", err)
	}

	// Send message body
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %v", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message failed: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing message failed: %v", err)
	}

	return c.Quit()
}

func (s *Server) sendSMTPEmail(host string, port int, username, password, tlsMode, from, to, subject, body, messageID string) error {
	// Extract email from "Name <email>" format if present
	fromEmail := from
	fromHeader := from
	if strings.Contains(from, "<") && strings.Contains(from, ">") {
		start := strings.Index(from, "<") + 1
		end := strings.Index(from, ">")
		if start > 0 && end > start {
			fromEmail = from[start:end]
		}
		// Encode the name part if it contains non-ASCII
		nameEnd := strings.Index(from, "<")
		if nameEnd > 0 {
			name := strings.TrimSpace(from[:nameEnd])
			if needsEncoding(name) {
				fromHeader = mimeEncode(name) + " <" + fromEmail + ">"
			}
		}
	}

	// Encode subject if it contains non-ASCII characters
	encodedSubject := subject
	if needsEncoding(subject) {
		encodedSubject = mimeEncode(subject)
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+
		"Date: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		"\r\n"+
		"%s", fromHeader, to, encodedSubject, messageID, time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"), encodeBase64WithLineBreaks([]byte(body)))

	addr := fmt.Sprintf("%s:%d", host, port)

	// Handle different TLS modes like the campaign engine
	switch tlsMode {
	case "tls":
		return s.sendWithImplicitTLS(addr, host, username, password, fromEmail, to, []byte(msg))
	case "starttls":
		return s.sendWithSTARTTLS(addr, host, username, password, fromEmail, to, []byte(msg))
	default: // "none" or empty
		return s.sendPlainSMTP(addr, host, username, password, fromEmail, to, []byte(msg))
	}
}

// needsEncoding checks if a string contains non-ASCII characters
func needsEncoding(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// mimeEncode encodes a string using MIME base64 encoding (RFC 2047)
func mimeEncode(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

// encodeBase64WithLineBreaks encodes data to base64 with 76-char line breaks (RFC 2045)
func encodeBase64WithLineBreaks(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	// Insert line breaks every 76 characters
	var result string
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		result += encoded[i:end] + "\r\n"
	}
	return result
}

// sendWithImplicitTLS sends email using implicit TLS (port 465)
func (s *Server) sendWithImplicitTLS(addr, host, username, password, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	// First establish TCP connection with timeout
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tcpConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP dial error: %v", err)
	}

	// Then upgrade to TLS
	conn := tls.Client(tcpConn, tlsConfig)
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := conn.Handshake(); err != nil {
		tcpConn.Close()
		return fmt.Errorf("TLS handshake error: %v", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendWithSTARTTLS sends email using STARTTLS
func (s *Server) sendWithSTARTTLS(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS error: %v", err)
	}

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendPlainSMTP sends email without TLS (for PowerMTA and similar)
func (s *Server) sendPlainSMTP(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// ============================================
// OAuth-enabled SMTP send functions
// ============================================

// sendWithImplicitTLSOAuth sends email using implicit TLS with OAuth2 support
func (s *Server) sendWithImplicitTLSOAuth(addr, host, username, password, from, to string, msg []byte, oauthToken, oauthClientID string) error {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tcpConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP dial error: %v", err)
	}

	conn := tls.Client(tcpConn, tlsConfig)
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := conn.Handshake(); err != nil {
		tcpConn.Close()
		return fmt.Errorf("TLS handshake error: %v", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := s.authenticateSMTPWithOAuth(client, host, username, password, oauthToken, oauthClientID); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendWithSTARTTLSOAuth sends email using STARTTLS with OAuth2 support
func (s *Server) sendWithSTARTTLSOAuth(addr, host, username, password, from, to string, msg []byte, oauthToken, oauthClientID string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS error: %v", err)
	}

	if err := s.authenticateSMTPWithOAuth(client, host, username, password, oauthToken, oauthClientID); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendPlainSMTPOAuth sends email without TLS with OAuth2 support
func (s *Server) sendPlainSMTPOAuth(addr, host, username, password, from, to string, msg []byte, oauthToken, oauthClientID string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	if err := s.authenticateSMTPWithOAuth(client, host, username, password, oauthToken, oauthClientID); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// authenticateSMTPWithOAuth tries OAuth2 first if available, then falls back to password
func (s *Server) authenticateSMTPWithOAuth(client *smtp.Client, host, username, password, oauthToken, oauthClientID string) error {
	if username == "" {
		return nil // No auth needed
	}

	// Try OAuth2 first if we have token and client_id
	if oauthToken != "" && oauthClientID != "" {
		log.Printf("[SMTP OAuth] Trying OAuth2 for %s...", username)
		tokenResp, oauthErr := refreshMicrosoftAccessToken(oauthToken, oauthClientID)
		if oauthErr == nil && tokenResp.AccessToken != "" {
			auth := newXOAuth2SMTPAuth(username, tokenResp.AccessToken)
			if err := client.Auth(auth); err == nil {
				log.Printf("[SMTP OAuth] XOAUTH2 authentication successful!")
				return nil
			} else {
				log.Printf("[SMTP OAuth] XOAUTH2 failed: %v, trying password...", err)
			}
		} else {
			log.Printf("[SMTP OAuth] Token refresh failed: %v, trying password...", oauthErr)
		}
	}

	// Fall back to password authentication
	return s.authenticateSMTP(client, host, username, password)
}

// authenticateSMTP tries LOGIN auth first (works without TLS), then PLAIN
func (s *Server) authenticateSMTP(client *smtp.Client, host, username, password string) error {
	if username == "" || password == "" {
		return nil // No auth needed
	}

	// Try LOGIN auth first (doesn't require encrypted connection)
	if err := client.Auth(warmupLoginAuth(username, password)); err == nil {
		return nil
	}

	// Try PLAIN auth as fallback
	auth := smtp.PlainAuth("", username, password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth error (tried LOGIN, PLAIN): %v", err)
	}

	return nil
}

// sendSMTPMessage sends the email message after authentication
func (s *Server) sendSMTPMessage(client *smtp.Client, from, to string, msg []byte) error {
	// Send MAIL FROM without SMTPUTF8 extension (Go adds it automatically, causing issues)
	// Use raw command like the campaign engine does
	id, err := client.Text.Cmd("MAIL FROM:<%s>", from)
	if err != nil {
		return fmt.Errorf("MAIL FROM error: %v", err)
	}
	client.Text.StartResponse(id)
	code, message, err := client.Text.ReadResponse(250)
	client.Text.EndResponse(id)
	if err != nil {
		return fmt.Errorf("MAIL FROM failed (%d): %s - %v", code, message, err)
	}

	// Send RCPT TO
	id, err = client.Text.Cmd("RCPT TO:<%s>", to)
	if err != nil {
		return fmt.Errorf("RCPT TO error: %v", err)
	}
	client.Text.StartResponse(id)
	code, message, err = client.Text.ReadResponse(250)
	client.Text.EndResponse(id)
	if err != nil {
		return fmt.Errorf("RCPT TO failed (%d): %s - %v", code, message, err)
	}

	// Send DATA
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA error: %v", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close error: %v", err)
	}

	return client.Quit()
}

// warmupLoginAuth implements LOGIN authentication (works without TLS)
type warmupLoginAuthStruct struct {
	username, password string
}

func warmupLoginAuth(username, password string) smtp.Auth {
	return &warmupLoginAuthStruct{username, password}
}

func (a *warmupLoginAuthStruct) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *warmupLoginAuthStruct) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, fmt.Errorf("unknown from server: %s", string(fromServer))
		}
	}
	return nil, nil
}

// xoauth2SMTPAuth implements XOAUTH2 authentication for SMTP (Microsoft OAuth2)
type xoauth2SMTPAuth struct {
	username    string
	accessToken string
}

func newXOAuth2SMTPAuth(username, accessToken string) smtp.Auth {
	return &xoauth2SMTPAuth{username, accessToken}
}

func (a *xoauth2SMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// XOAUTH2 format: "user=<email>\x01auth=Bearer <token>\x01\x01"
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.accessToken)
	return "XOAUTH2", []byte(authStr), nil
}

func (a *xoauth2SMTPAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// Server sent an error response, return empty to abort
		return nil, fmt.Errorf("XOAUTH2 error: %s", string(fromServer))
	}
	return nil, nil
}

func (s *Server) updateWarmupDailyStats(warmupID, statType string) {
	today := time.Now().Format("2006-01-02")

	// Try to update existing record
	result, _ := s.db.Exec(fmt.Sprintf(`
		UPDATE warmup_daily_stats SET %s = %s + 1
		WHERE warmup_smtp_id = $1 AND date = $2
	`, statType, statType), warmupID, today)

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Insert new record
		s.db.Exec(`
			INSERT INTO warmup_daily_stats (id, warmup_smtp_id, date, sent, inbox, spam, replies)
			VALUES ($1, $2, $3, 0, 0, 0, 0)
		`, uuid.New().String(), warmupID, today)

		// Update again
		s.db.Exec(fmt.Sprintf(`
			UPDATE warmup_daily_stats SET %s = %s + 1
			WHERE warmup_smtp_id = $1 AND date = $2
		`, statType, statType), warmupID, today)
	}
}

// ============================================
// SEED TO SMTP EMAIL PROCESSOR
// ============================================

// isQuotaOrSpamBlockError checks if an SMTP error indicates the account has been blocked
// by the provider due to spam detection or quota limits (common with Outlook/Hotmail)
func isQuotaOrSpamBlockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// Outlook/Hotmail specific errors
	return strings.Contains(errStr, "outboundspamexception") ||
		strings.Contains(errStr, "refusequota") ||
		strings.Contains(errStr, "554 5.2.0") ||
		strings.Contains(errStr, "wascl useraction") ||
		strings.Contains(errStr, "showtirupgrade") ||
		// Generic spam/abuse errors
		strings.Contains(errStr, "spam") && strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "account suspended") ||
		strings.Contains(errStr, "sending limit exceeded") ||
		strings.Contains(errStr, "too many messages")
}

func (s *Server) processSeedToSMTPEmails() {
	// FIRST: Check if any SMTPs are active - if all paused, skip entire cycle
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	if activeSMTPs == 0 {
		// Silent skip when all paused - no log spam
		return
	}

	now := time.Now()
	currentHour := now.Hour()

	// Get total count of active seeds first
	var totalSeeds int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_seeds
		WHERE status = 'active' AND smtp_host IS NOT NULL AND smtp_host != ''
	`).Scan(&totalSeeds)

	if totalSeeds == 0 {
		log.Printf("[Warmup Seed→SMTP] No active seeds with SMTP configured")
		return
	}

	// Get current batch offset (thread-safe)
	seedBatchMutex.Lock()
	currentOffset := seedBatchOffset
	// Update offset for next cycle
	seedBatchOffset += maxSeedsPerCycle
	if seedBatchOffset >= totalSeeds {
		seedBatchOffset = 0 // Reset to beginning
	}
	seedBatchMutex.Unlock()

	// Calculate batch info
	batchNumber := (currentOffset / maxSeedsPerCycle) + 1
	totalBatches := (totalSeeds + maxSeedsPerCycle - 1) / maxSeedsPerCycle

	log.Printf("[Warmup Seed→SMTP] Processing batch %d/%d (seeds %d-%d of %d total, hour: %d)",
		batchNumber, totalBatches, currentOffset+1, min(currentOffset+maxSeedsPerCycle, totalSeeds), totalSeeds, currentHour)

	// Get only this batch of seeds (with LIMIT and OFFSET) - includes user_id for isolation and proxy settings
	seedRows, err := s.db.Query(`
		SELECT id, user_id, email, password, smtp_host, smtp_port, use_tls,
		       COALESCE(send_rate, 50), COALESCE(emails_per_day, 20),
		       COALESCE(oauth_token, ''), COALESCE(oauth_client_id, ''),
		       COALESCE(proxy_enabled, false), COALESCE(proxy_host, 'proxy.soax.com'),
		       COALESCE(proxy_port, 9000), COALESCE(proxy_username, ''), COALESCE(proxy_password, '')
		FROM warmup_seeds
		WHERE status = 'active' AND smtp_host IS NOT NULL AND smtp_host != ''
		ORDER BY id
		LIMIT $1 OFFSET $2
	`, maxSeedsPerCycle, currentOffset)
	if err != nil {
		log.Printf("[Warmup Seed→SMTP] Error getting seeds: %v", err)
		return
	}
	defer seedRows.Close()

	type seedInfo struct {
		ID            string
		UserID        string // For user isolation
		Email         string
		Password      string
		SMTPHost      string
		SMTPPort      int
		UseTLS        bool
		SendRate      int
		EmailsPerDay  int
		OAuthToken    string
		OAuthClientID string
		// Proxy settings (SOAX)
		ProxyEnabled  bool
		ProxyHost     string
		ProxyPort     int
		ProxyUsername string
		ProxyPassword string
	}

	var seeds []seedInfo
	for seedRows.Next() {
		var seed seedInfo
		seedRows.Scan(&seed.ID, &seed.UserID, &seed.Email, &seed.Password, &seed.SMTPHost, &seed.SMTPPort, &seed.UseTLS,
			&seed.SendRate, &seed.EmailsPerDay, &seed.OAuthToken, &seed.OAuthClientID,
			&seed.ProxyEnabled, &seed.ProxyHost, &seed.ProxyPort, &seed.ProxyUsername, &seed.ProxyPassword)
		seeds = append(seeds, seed)
	}

	if len(seeds) == 0 {
		log.Printf("[Warmup Seed→SMTP] No seeds in this batch")
		return
	}

	log.Printf("[Warmup Seed→SMTP] Batch has %d seeds to process", len(seeds))

	// Process each seed - targets are fetched per-seed for user isolation
	totalSent := 0
	for _, seed := range seeds {
		// Get active warmup SMTPs and their senders FOR THIS USER ONLY (user isolation)
		// Only those within their configured hours
		smtpRows, err := s.db.Query(`
			SELECT w.id, w.smtp_id, ss.email as sender_email, w.start_hour, w.end_hour
			FROM warmup_smtps w
			JOIN smtp_senders ss ON ss.smtp_id = w.smtp_id
			WHERE w.status = 'active' AND ss.active = true
			AND w.user_id = $1
			AND $2 >= w.start_hour AND $2 < w.end_hour
		`, seed.UserID, currentHour)
		if err != nil {
			log.Printf("[Warmup Seed→SMTP] Error getting SMTP senders for user %s: %v", seed.UserID, err)
			continue
		}

		var targets []struct {
			WarmupID    string
			SMTPID      string
			SenderEmail string
		}

		// Track min/max hours from the SMTPs we're actually using
		minStartHour := 24
		maxEndHour := 0

		for smtpRows.Next() {
			var target struct {
				WarmupID    string
				SMTPID      string
				SenderEmail string
			}
			var startHour, endHour int
			smtpRows.Scan(&target.WarmupID, &target.SMTPID, &target.SenderEmail, &startHour, &endHour)
			targets = append(targets, target)

			// Track the actual hours from configured SMTPs
			if startHour < minStartHour {
				minStartHour = startHour
			}
			if endHour > maxEndHour {
				maxEndHour = endHour
			}
		}
		smtpRows.Close()

		if len(targets) == 0 {
			log.Printf("[Warmup Seed→SMTP] Seed %s (user %s): No SMTP targets within configured hours (hour: %d)", seed.Email, seed.UserID, currentHour)
			continue
		}

		// Calculate sending hours from the configured SMTP hours
		sendingHours := maxEndHour - minStartHour
		if sendingHours <= 0 {
			sendingHours = 1 // minimum 1 hour to avoid division by zero
		}
		// Check if warmup was paused (allows immediate stop when user clicks pause)
		var activeCount int
		s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeCount)
		if activeCount == 0 {
			log.Printf("[Warmup Seed→SMTP] All warmup SMTPs paused - stopping immediately")
			return // Exit entire function, not just break
		}

		// Check how many this seed already sent today (from warmup_seed_emails, NOT warmup_emails)
		// Use CURRENT_DATE from PostgreSQL to avoid timezone issues
		var sentToday int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_seed_emails
			WHERE seed_id = $1 AND DATE(sent_at) = CURRENT_DATE
		`, seed.ID).Scan(&sentToday)

		// Check daily limit
		if sentToday >= seed.EmailsPerDay {
			log.Printf("[Warmup Seed→SMTP] Seed %s: Daily limit reached (%d/%d)", seed.Email, sentToday, seed.EmailsPerDay)
			continue
		}

		// Calculate how many to send this cycle
		// Seed→SMTP runs every 3 minutes = 20 cycles per hour
		cyclesPerHour := 20
		totalCycles := sendingHours * cyclesPerHour
		remaining := seed.EmailsPerDay - sentToday
		emailsThisCycle := remaining / totalCycles

		// Use send_rate as probability for low volume
		if emailsThisCycle < 1 {
			if rand.Intn(100) < seed.SendRate {
				emailsThisCycle = 1
			} else {
				log.Printf("[Warmup Seed→SMTP] Seed %s: Skipped by rate (remaining=%d, rate=%d%%)", seed.Email, remaining, seed.SendRate)
				continue
			}
		}

		log.Printf("[Warmup Seed→SMTP] Seed %s: Limit=%d, Sent=%d, Remaining=%d, ThisCycle=%d, Rate=%d%%",
			seed.Email, seed.EmailsPerDay, sentToday, remaining, emailsThisCycle, seed.SendRate)

		// Send emails
		for i := 0; i < emailsThisCycle; i++ {
			// Check if warmup was paused before each send
			var activeCount int
			s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeCount)
			if activeCount == 0 {
				log.Printf("[Warmup Seed→SMTP] Warmup paused - stopping seed %s", seed.Email)
				return // Exit entire function
			}

			// Pick random target
			target := targets[rand.Intn(len(targets))]

			// Get a random template (only 'send' type, not 'reply')
			var subject, body string
			err = s.db.QueryRow(`
				SELECT subject, body FROM warmup_templates
				WHERE active = true AND template_type = 'send' ORDER BY RANDOM() LIMIT 1
			`).Scan(&subject, &body)
			if err != nil {
				continue
			}

			// Add randomization to subject
			subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

			// Generate message ID
			messageID := fmt.Sprintf("<%s@seed-warmup>", uuid.New().String())

			// Determine TLS mode for seed's SMTP
			tlsMode := "starttls"
			if seed.SMTPPort == 465 {
				tlsMode = "tls"
			}

			// Send email from seed to SMTP sender
			// Use proxy if enabled, otherwise use direct connection with OAuth support
			if seed.ProxyEnabled && seed.ProxyUsername != "" {
				log.Printf("[Warmup Seed→SMTP] Sending via SOCKS5 proxy %s:%d", seed.ProxyHost, seed.ProxyPort)
				err = s.sendSMTPEmailWithProxy(seed.SMTPHost, seed.SMTPPort, seed.Email, seed.Password, tlsMode,
					seed.Email, target.SenderEmail, subject, body, messageID,
					seed.ProxyHost, seed.ProxyPort, seed.ProxyUsername, seed.ProxyPassword)
			} else {
				// Direct connection with OAuth2 support for Outlook
				err = s.sendSMTPEmailWithOAuth(seed.SMTPHost, seed.SMTPPort, seed.Email, seed.Password, tlsMode,
					seed.Email, target.SenderEmail, subject, body, messageID, seed.OAuthToken, seed.OAuthClientID)
			}

			if err != nil {
				log.Printf("[Warmup Seed→SMTP] Failed to send from %s to %s: %v", seed.Email, target.SenderEmail, err)

				// Check if this is a quota/spam block error - if so, deactivate the seed
				if isQuotaOrSpamBlockError(err) {
					log.Printf("[Warmup Seed→SMTP] ⚠️ Quota/spam block detected for %s - deactivating seed", seed.Email)
					s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
						err.Error(), seed.ID)
					break // Stop sending from this seed
				}
				continue
			}

			// Save to warmup_seed_emails table for activity tracking
			s.db.Exec(`
				INSERT INTO warmup_seed_emails (seed_id, to_email, to_warmup_smtp_id, subject, message_id, status, sent_at)
				VALUES ($1, $2, $3, $4, $5, 'sent', NOW())
			`, seed.ID, target.SenderEmail, target.WarmupID, subject, messageID)

			// Update seed's sent counter
			s.db.Exec(`UPDATE warmup_seeds SET total_sent = total_sent + 1 WHERE id = $1`, seed.ID)

			log.Printf("[Warmup Seed→SMTP] ✉️ Sent from %s to %s: %s", seed.Email, target.SenderEmail, subject)
			totalSent++

			// Small delay between sends from same seed
			time.Sleep(200 * time.Millisecond)
		}

		// Random delay between different seeds (5-10 seconds) to appear more human-like
		// This prevents all seeds from connecting to Outlook/Hotmail simultaneously
		delaySeconds := minDelayBetweenSeed + rand.Intn(maxDelayBetweenSeed-minDelayBetweenSeed+1)
		log.Printf("[Warmup Seed→SMTP] Waiting %ds before next seed...", delaySeconds)
		time.Sleep(time.Duration(delaySeconds) * time.Second)
	}

	log.Printf("[Warmup Seed→SMTP] Batch complete: sent %d emails", totalSent)
}

// ============================================
// IMAP INTERACTION PROCESSOR
// ============================================

func (s *Server) processIMAPInteractions() {
	// Count active seeds
	var activeSeeds int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	log.Printf("[Warmup IMAP] Checking %d active seed inboxes...", activeSeeds)

	if activeSeeds == 0 {
		log.Println("[Warmup IMAP] No active seeds to check")
		return
	}

	// Get all active seeds with OAuth credentials
	rows, err := s.db.Query(`
		SELECT id, email, password, imap_host, imap_port, use_tls,
		       COALESCE(imap_tls_mode, 'tls'), COALESCE(oauth_token, ''), COALESCE(oauth_client_id, '')
		FROM warmup_seeds WHERE status = 'active'
	`)
	if err != nil {
		log.Printf("[Warmup IMAP] Error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var seedID, email, password, imapHost string
		var imapPort int
		var useTLS bool
		var tlsMode, oauthToken, oauthClientID string

		rows.Scan(&seedID, &email, &password, &imapHost, &imapPort, &useTLS, &tlsMode, &oauthToken, &oauthClientID)

		log.Printf("[Warmup IMAP] Processing inbox for %s", email)
		go s.processOneSeedInboxWithOAuth(seedID, email, password, imapHost, imapPort, tlsMode, oauthToken, oauthClientID)
	}
}

func (s *Server) processOneSeedInboxWithOAuth(seedID, email, password, imapHost string, imapPort int, tlsMode, oauthToken, oauthClientID string) {
	addr := fmt.Sprintf("%s:%d", imapHost, imapPort)

	var c *client.Client
	var err error

	// Create dialer with timeout
	dialer := &net.Dialer{Timeout: 30 * time.Second}

	// Determine if we should use TLS
	useTLS := tlsMode == "tls" || imapPort == 993

	if useTLS {
		// First establish TCP connection with timeout
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Warmup IMAP] TCP dial failed for %s: %v", email, dialErr)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				dialErr.Error(), seedID)
			return
		}
		// Upgrade to TLS
		tlsConn := tls.Client(conn, &tls.Config{ServerName: imapHost, InsecureSkipVerify: true})
		tlsConn.SetDeadline(time.Now().Add(30 * time.Second))
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			log.Printf("[Warmup IMAP] TLS handshake failed for %s: %v", email, err)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				err.Error(), seedID)
			return
		}
		c, err = client.New(tlsConn)
	} else if tlsMode == "starttls" {
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Warmup IMAP] TCP dial failed for %s: %v", email, dialErr)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				dialErr.Error(), seedID)
			return
		}
		c, err = client.New(conn)
		if err == nil {
			tlsConfig := &tls.Config{ServerName: imapHost, InsecureSkipVerify: true}
			if startTLSErr := c.StartTLS(tlsConfig); startTLSErr != nil {
				c.Logout()
				log.Printf("[Warmup IMAP] STARTTLS failed for %s: %v", email, startTLSErr)
				s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
					startTLSErr.Error(), seedID)
				return
			}
		}
	} else {
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Warmup IMAP] TCP dial failed for %s: %v", email, dialErr)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				dialErr.Error(), seedID)
			return
		}
		c, err = client.New(conn)
	}

	if err != nil {
		log.Printf("[Warmup IMAP] Failed to connect to %s: %v", email, err)
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
		return
	}
	defer c.Logout()

	// Set timeout for IMAP operations
	c.Timeout = 60 * time.Second

	// Try OAuth2 first if we have token and client_id
	authSuccess := false
	if oauthToken != "" && oauthClientID != "" {
		log.Printf("[Warmup IMAP] Trying OAuth2 for %s...", email)
		tokenResp, oauthErr := refreshMicrosoftAccessToken(oauthToken, oauthClientID)
		if oauthErr == nil && tokenResp.AccessToken != "" {
			if err := authenticateIMAPWithXOAuth2(c, email, tokenResp.AccessToken); err == nil {
				log.Printf("[Warmup IMAP] OAuth2 authentication successful for %s", email)
				authSuccess = true
			} else {
				log.Printf("[Warmup IMAP] OAuth2 failed for %s: %v, trying password...", email, err)
			}
		} else {
			log.Printf("[Warmup IMAP] Token refresh failed for %s: %v, trying password...", email, oauthErr)
		}
	}

	// Fall back to password auth
	if !authSuccess {
		if err := c.Login(email, password); err != nil {
			log.Printf("[Warmup IMAP] Login failed for %s: %v", email, err)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				err.Error(), seedID)
			return
		}
	}

	// Update last check - clear error message on success
	s.db.Exec(`UPDATE warmup_seeds SET status = 'active', last_check = NOW(), error_message = NULL WHERE id = $1`, seedID)

	log.Printf("[Warmup IMAP] Connected successfully to %s, checking mailboxes...", email)

	// Check INBOX for warmup emails
	s.checkMailbox(c, seedID, "INBOX", false)

	// Check Spam/Junk folder
	spamFolders := []string{"[Gmail]/Spam", "Junk", "Spam", "Junk E-mail", "Bulk Mail"}
	for _, folder := range spamFolders {
		if s.checkMailbox(c, seedID, folder, true) {
			break
		}
	}

	// Random chance to reply (based on reply rate)
	s.maybeReplyToWarmupEmail(c, seedID, email, password, imapHost)
}

func (s *Server) checkMailbox(c *client.Client, seedID, mailbox string, isSpam bool) bool {
	mbox, err := c.Select(mailbox, false)
	if err != nil {
		return false
	}

	if mbox.Messages == 0 {
		return true
	}

	// Search for emails from the last 7 days (not just unseen)
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().AddDate(0, 0, -7)

	ids, err := c.Search(criteria)
	if err != nil || len(ids) == 0 {
		return true
	}

	// Limit to last 100 messages to avoid processing too many
	if len(ids) > 100 {
		ids = ids[len(ids)-100:]
	}

	log.Printf("[Warmup IMAP] Found %d emails in %s to check", len(ids), mailbox)

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(ids...)

	// Fetch messages
	messages := make(chan *imap.Message, 10)
	section := &imap.BodySectionName{Peek: true}
	go func() {
		c.Fetch(seqSet, []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}, messages)
	}()

	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		// Check if this is a warmup email (by message ID)
		messageID := msg.Envelope.MessageId
		subject := ""
		if msg.Envelope.Subject != "" {
			subject = msg.Envelope.Subject
		}

		// Skip if no message ID
		if messageID == "" {
			continue
		}

		// Clean message ID (remove angle brackets if present)
		cleanMessageID := strings.Trim(messageID, "<>")

		// Check if this contains "warmup" - our marker
		if !strings.Contains(cleanMessageID, "@warmup") {
			continue
		}

		log.Printf("[Warmup IMAP] Found warmup marker in email: %s (MessageID: %s)", subject, messageID)

		var warmupEmailID, warmupSMTPID string
		// Try exact match with angle brackets
		err := s.db.QueryRow(`
			SELECT id, warmup_smtp_id FROM warmup_emails
			WHERE message_id = $1 AND seed_id = $2
		`, "<"+cleanMessageID+">", seedID).Scan(&warmupEmailID, &warmupSMTPID)

		if err != nil {
			// Try without angle brackets
			s.db.QueryRow(`
				SELECT id, warmup_smtp_id FROM warmup_emails
				WHERE message_id = $1 AND seed_id = $2
			`, cleanMessageID, seedID).Scan(&warmupEmailID, &warmupSMTPID)
		}

		if err != nil {
			// Try with LIKE for flexible matching
			s.db.QueryRow(`
				SELECT id, warmup_smtp_id FROM warmup_emails
				WHERE message_id LIKE $1 AND seed_id = $2
			`, "%"+cleanMessageID+"%", seedID).Scan(&warmupEmailID, &warmupSMTPID)
		}

		if warmupEmailID == "" {
			log.Printf("[Warmup IMAP] Email not found in DB for MessageID: %s", cleanMessageID)
			continue
		}

		// Found a warmup email!
		log.Printf("[Warmup IMAP] Found warmup email in %s: %s", mailbox, msg.Envelope.Subject)

		// Mark as read (opens the email)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}
		singleSeq := new(imap.SeqSet)
		singleSeq.AddNum(msg.SeqNum)
		c.Store(singleSeq, item, flags, nil)

		// Check if already verified (avoid counting twice)
		var alreadyVerified bool
		s.db.QueryRow(`SELECT verified FROM warmup_emails WHERE id = $1`, warmupEmailID).Scan(&alreadyVerified)

		if alreadyVerified {
			continue // Already processed this email
		}

		// Mark as verified - we confirmed the email arrived
		s.db.Exec(`UPDATE warmup_emails SET verified = true WHERE id = $1`, warmupEmailID)

		if isSpam {
			// Move from spam to inbox
			s.db.Exec(`UPDATE warmup_emails SET landed_in_spam = true WHERE id = $1`, warmupEmailID)
			s.db.Exec(`UPDATE warmup_smtps SET total_spam = total_spam + 1 WHERE id = $1`, warmupSMTPID)
			s.updateWarmupDailyStats(warmupSMTPID, "spam")

			// Try to move to inbox
			if err := c.Move(singleSeq, "INBOX"); err == nil {
				s.db.Exec(`UPDATE warmup_emails SET moved_to_inbox = true WHERE id = $1`, warmupEmailID)

				// Log activity for "moved to inbox"
				s.db.Exec(`
					INSERT INTO warmup_activity (activity_type, email_id, seed_id, warmup_smtp_id, subject, details)
					SELECT 'moved_to_inbox', e.id, e.seed_id, e.warmup_smtp_id, e.subject, 'Movido do spam para entrada'
					FROM warmup_emails e WHERE e.id = $1
				`, warmupEmailID)

				log.Printf("[Warmup IMAP] Moved email from spam to inbox")
			}
		} else {
			// Landed in inbox - good!
			s.db.Exec(`UPDATE warmup_emails SET status = 'opened', opened_at = NOW() WHERE id = $1`, warmupEmailID)
			s.db.Exec(`UPDATE warmup_smtps SET total_inbox = total_inbox + 1 WHERE id = $1`, warmupSMTPID)
			s.updateWarmupDailyStats(warmupSMTPID, "inbox")
		}
	}

	return true
}

func (s *Server) maybeReplyToWarmupEmail(c *client.Client, seedID, email, password, imapHost string) {
	// First check if this seed has auto_reply enabled
	var autoReply bool
	var seedReplyRate int
	err := s.db.QueryRow(`SELECT COALESCE(auto_reply, true), COALESCE(reply_rate, 50) FROM warmup_seeds WHERE id = $1`, seedID).Scan(&autoReply, &seedReplyRate)
	if err != nil || !autoReply {
		log.Printf("[Warmup Reply] Seed %s: auto_reply disabled or error: %v", email, err)
		return
	}

	// Get a warmup email that hasn't been replied to
	var warmupEmailID, warmupSMTPID, originalSubject, messageID string
	var smtpHost, smtpUsername, smtpPassword, smtpTLSMode string
	var smtpPort int

	err = s.db.QueryRow(`
		SELECT e.id, e.warmup_smtp_id, e.subject, e.message_id,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_emails e
		JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE e.seed_id = $1 AND e.replied_at IS NULL AND e.status = 'opened'
		ORDER BY RANDOM()
		LIMIT 1
	`, seedID).Scan(&warmupEmailID, &warmupSMTPID, &originalSubject, &messageID,
		&smtpHost, &smtpPort, &smtpUsername, &smtpPassword, &smtpTLSMode)

	if err != nil {
		return
	}

	// Check SEED's reply rate (not SMTP's)
	randomValue := rand.Intn(100)
	if randomValue >= seedReplyRate {
		log.Printf("[Warmup Reply] Seed %s: Skipped reply (rate=%d%%, roll=%d)", email, seedReplyRate, randomValue)
		return
	}

	log.Printf("[Warmup Reply] Seed %s: Will reply (rate=%d%%, roll=%d)", email, seedReplyRate, randomValue)

	// Send reply from seed to SMTP
	replySubject := "Re: " + originalSubject
	replyBody := getRandomReplyBody()

	// Get seed's SMTP settings
	var seedSMTPHost string
	var seedSMTPPort int
	s.db.QueryRow(`SELECT smtp_host, smtp_port FROM warmup_seeds WHERE id = $1`, seedID).Scan(&seedSMTPHost, &seedSMTPPort)

	if seedSMTPHost == "" {
		return
	}

	replyMessageID := fmt.Sprintf("<%s@warmup-reply>", uuid.New().String())
	// Seeds typically use STARTTLS - seed email is the From address
	err = s.sendSMTPEmail(seedSMTPHost, seedSMTPPort, email, password, "starttls", email, smtpUsername, replySubject, replyBody, replyMessageID)

	if err != nil {
		log.Printf("[Warmup Reply] Failed to send reply: %v", err)

		// Check if this is a quota/spam block error - if so, deactivate the seed
		if isQuotaOrSpamBlockError(err) {
			log.Printf("[Warmup Reply] ⚠️ Quota/spam block detected for %s - deactivating seed", email)
			s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
				err.Error(), seedID)
		}
		return
	}

	// Update the original email as replied and store who replied
	s.db.Exec(`UPDATE warmup_emails SET replied_at = NOW(), status = 'replied', reply_from = $2, reply_to = $3 WHERE id = $1`, warmupEmailID, email, smtpUsername)
	s.db.Exec(`UPDATE warmup_smtps SET total_replies = total_replies + 1 WHERE id = $1`, warmupSMTPID)
	s.updateWarmupDailyStats(warmupSMTPID, "replies")

	log.Printf("[Warmup Reply] ↩️ Replied from %s to warmup email", email)
}

func getRandomReplyBody() string {
	replies := []string{
		"Obrigado por entrar em contato! Retorno em breve.",
		"Recebi, obrigado pela informação.",
		"Obrigado pela mensagem. Vou analisar e respondo em breve.",
		"Recebido, obrigado!",
		"Obrigado pela atualização. Agradeço!",
		"Perfeito, obrigado por avisar.",
		"Ótimo, vou dar uma olhada nisso.",
		"Obrigado! Entro em contato em breve.",
		"Anotado. Obrigado por enviar.",
		"Obrigado, vou dar seguimento nisso em breve.",
	}
	return replies[rand.Intn(len(replies))]
}

// ============================================
// INTERNAL WARMUP (SMTP → SMTP)
// ============================================

// processInternalWarmup sends emails between SMTPs for internal warmup
func (s *Server) processInternalWarmup() {
	// FIRST: Check if any SMTPs are active - if all paused, skip entire cycle
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	if activeSMTPs == 0 {
		// Silent skip when all paused - no log spam
		return
	}

	log.Printf("[Internal Warmup] === Cycle starting ===")

	// Check if warmup is enabled
	warmupEnabled := s.getWarmupSetting("warmup_enabled", "true")
	if warmupEnabled != "true" {
		log.Printf("[Internal Warmup] Disabled globally")
		return
	}

	now := time.Now()
	currentHour := now.Hour()

	// Get SMTPs with internal warmup enabled (include schedule settings)
	rows, err := s.db.Query(`
		SELECT w.id, w.smtp_id, w.start_date, w.min_emails_per_day, w.max_emails_per_day,
		       w.recipe_type, w.custom_schedule, w.send_rate, w.reply_rate, w.start_hour, w.end_hour,
		       s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND w.internal_warmup = true AND s.active = true
	`)
	if err != nil {
		log.Printf("[Internal Warmup] Error getting SMTPs: %v", err)
		return
	}
	defer rows.Close()

	type smtpInfo struct {
		WarmupID       string
		SMTPID         string
		StartDate      time.Time
		MinEmails      int
		MaxEmails      int
		RecipeType     string
		CustomSchedule sql.NullString
		SendRate       int
		ReplyRate      int
		StartHour      int
		EndHour        int
		Host           string
		Port           int
		Username       string
		Password       string
		TLSMode        string
	}

	var smtps []smtpInfo
	for rows.Next() {
		var smtp smtpInfo
		rows.Scan(&smtp.WarmupID, &smtp.SMTPID, &smtp.StartDate, &smtp.MinEmails, &smtp.MaxEmails,
			&smtp.RecipeType, &smtp.CustomSchedule, &smtp.SendRate, &smtp.ReplyRate, &smtp.StartHour, &smtp.EndHour,
			&smtp.Host, &smtp.Port, &smtp.Username, &smtp.Password, &smtp.TLSMode)
		smtps = append(smtps, smtp)
	}

	log.Printf("[Internal Warmup] Found %d SMTPs with internal warmup enabled", len(smtps))

	if len(smtps) < 2 {
		log.Printf("[Internal Warmup] Need at least 2 SMTPs for cross-SMTP warmup, have %d", len(smtps))
		return
	}

	// Filter SMTPs that are within their configured hours
	var activeSmtps []smtpInfo
	for _, smtp := range smtps {
		isWithinHours := false
		if smtp.StartHour <= smtp.EndHour {
			isWithinHours = currentHour >= smtp.StartHour && currentHour <= smtp.EndHour
		} else {
			isWithinHours = currentHour >= smtp.StartHour || currentHour <= smtp.EndHour
		}

		if isWithinHours {
			activeSmtps = append(activeSmtps, smtp)
		} else {
			log.Printf("[Internal Warmup] SMTP %s outside hours (current: %d, range: %d-%d)",
				smtp.Host, currentHour, smtp.StartHour, smtp.EndHour)
		}
	}

	if len(activeSmtps) < 2 {
		log.Printf("[Internal Warmup] Need at least 2 SMTPs within active hours, have %d", len(activeSmtps))
		return
	}

	// Process each SMTP - calculate how many to send based on schedule
	totalEmailsSent := 0
	for _, fromSMTP := range activeSmtps {
		// Calculate current day based on calendar days (not hours)
		startDay := time.Date(fromSMTP.StartDate.Year(), fromSMTP.StartDate.Month(), fromSMTP.StartDate.Day(), 0, 0, 0, 0, fromSMTP.StartDate.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		currentDay := int(today.Sub(startDay).Hours()/24) + 1
		if currentDay < 1 {
			currentDay = 1
		}

		// Calculate today's limit based on recipe type
		todayLimit := s.calculateDailyLimit(fromSMTP.RecipeType, currentDay, fromSMTP.MinEmails, fromSMTP.MaxEmails, fromSMTP.CustomSchedule.String)

		// Get how many internal emails this SMTP already sent today
		var sentToday int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_internal_emails
			WHERE from_smtp_id = $1 AND DATE(sent_at) = $2
		`, fromSMTP.SMTPID, now.Format("2006-01-02")).Scan(&sentToday)

		if sentToday >= todayLimit {
			log.Printf("[Internal Warmup] SMTP %s: Daily limit reached (%d/%d), skipping", fromSMTP.Host, sentToday, todayLimit)
			continue
		}

		// Calculate how many to send this cycle (spread throughout the day)
		remaining := todayLimit - sentToday
		sendingHours := fromSMTP.EndHour - fromSMTP.StartHour
		if sendingHours <= 0 {
			sendingHours = 24
		}

		// Get cycle interval from settings (default 2 minutes)
		cycleMinutes := s.getWarmupSettingInt("internal_cycle_minutes", 2)
		cyclesPerHour := 60 / cycleMinutes
		totalCycles := sendingHours * cyclesPerHour
		if totalCycles <= 0 {
			totalCycles = 1
		}

		emailsThisCycle := remaining / totalCycles
		if emailsThisCycle < 1 {
			// Use send_rate as probability for low volume
			sendRate := fromSMTP.SendRate
			if sendRate <= 0 {
				sendRate = 30
			}
			if rand.Intn(100) < sendRate {
				emailsThisCycle = 1
			} else {
				log.Printf("[Internal Warmup] SMTP %s: Skipped by rate (remaining=%d, rate=%d%%)", fromSMTP.Host, remaining, sendRate)
				continue
			}
		}

		log.Printf("[Internal Warmup] SMTP %s: Day %d, Limit=%d, Sent=%d, Remaining=%d, ThisCycle=%d",
			fromSMTP.Host, currentDay, todayLimit, sentToday, remaining, emailsThisCycle)

		// Send emails for this cycle
		for i := 0; i < emailsThisCycle; i++ {
			// Check if warmup was paused before each send
			var currentStatus string
			s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1`, fromSMTP.WarmupID).Scan(&currentStatus)
			if currentStatus != "active" {
				log.Printf("[Internal Warmup] SMTP %s paused - stopping", fromSMTP.Host)
				return // Exit entire function
			}

			// Pick a DIFFERENT SMTP as destination
			var toSMTP smtpInfo
			found := false
			for j := 0; j < 10; j++ {
				idx := rand.Intn(len(activeSmtps))
				if activeSmtps[idx].SMTPID != fromSMTP.SMTPID {
					toSMTP = activeSmtps[idx]
					found = true
					break
				}
			}
			if !found {
				continue
			}

			// Get a sender from the FROM SMTP
			var fromSenderID, fromSenderEmail string
			var fromSenderName sql.NullString
			err = s.db.QueryRow(`
				SELECT id, email, name FROM smtp_senders
				WHERE smtp_id = $1 AND active = true
				ORDER BY RANDOM() LIMIT 1
			`, fromSMTP.SMTPID).Scan(&fromSenderID, &fromSenderEmail, &fromSenderName)
			if err != nil {
				continue
			}

			// Get a sender from the TO SMTP (must have IMAP configured)
			var toSenderID, toSenderEmail string
			err = s.db.QueryRow(`
				SELECT id, email FROM smtp_senders
				WHERE smtp_id = $1 AND active = true
				AND imap_host IS NOT NULL AND imap_host != ''
				ORDER BY RANDOM() LIMIT 1
			`, toSMTP.SMTPID).Scan(&toSenderID, &toSenderEmail)
			if err != nil {
				continue
			}

			// Get a random template (only 'send' type, not 'reply')
			var subject, body string
			err = s.db.QueryRow(`
				SELECT subject, body FROM warmup_templates
				WHERE active = true AND template_type = 'send' ORDER BY RANDOM() LIMIT 1
			`).Scan(&subject, &body)
			if err != nil {
				continue
			}

			// Add randomization to subject
			subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

			// Generate message ID
			messageID := fmt.Sprintf("<%s@internal-warmup>", uuid.New().String())

			// Format From address
			fromAddress := fromSenderEmail
			if fromSenderName.Valid && fromSenderName.String != "" {
				fromAddress = fmt.Sprintf("%s <%s>", fromSenderName.String, fromSenderEmail)
			}

			// Send email
			err = s.sendSMTPEmail(fromSMTP.Host, fromSMTP.Port, fromSMTP.Username, fromSMTP.Password,
				fromSMTP.TLSMode, fromAddress, toSenderEmail, subject, body, messageID)

			if err != nil {
				log.Printf("[Internal Warmup] ❌ Failed: %s → %s: %v", fromSenderEmail, toSenderEmail, err)
				continue
			}

			// Record the email
			emailID := uuid.New().String()
			s.db.Exec(`
				INSERT INTO warmup_internal_emails (id, from_smtp_id, to_smtp_id, from_sender_email, to_sender_id, subject, message_id, status, sent_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 'sent', NOW())
			`, emailID, fromSMTP.SMTPID, toSMTP.SMTPID, fromSenderEmail, toSenderID, subject, messageID)

			log.Printf("[Internal Warmup] ✉️ Sent: %s → %s", fromSenderEmail, toSenderEmail)
			totalEmailsSent++

			// Small delay between sends
			time.Sleep(300 * time.Millisecond)
		}
	}

	log.Printf("[Internal Warmup] === Cycle complete: sent %d emails ===", totalEmailsSent)
}

// processInternalWarmupIMAP checks IMAP for smtp_senders to receive and reply to internal warmup emails
func (s *Server) processInternalWarmupIMAP() {
	// FIRST: Check if any SMTPs are active - if all paused, skip entire cycle
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	if activeSMTPs == 0 {
		// Silent skip when all paused - no log spam
		return
	}

	// Get smtp_senders with IMAP configured
	rows, err := s.db.Query(`
		SELECT ss.id, ss.email, ss.imap_host, ss.imap_port, ss.imap_password, COALESCE(ss.imap_tls_mode, 'tls'),
		       sm.id as smtp_id, sm.host, sm.port, sm.username, sm.password, sm.tls_mode
		FROM smtp_senders ss
		JOIN smtp_servers sm ON ss.smtp_id = sm.id
		JOIN warmup_smtps w ON w.smtp_id = sm.id
		WHERE w.internal_warmup = true AND w.status = 'active'
		AND ss.imap_host IS NOT NULL AND ss.imap_host != ''
		AND ss.imap_password IS NOT NULL AND ss.imap_password != ''
	`)
	if err != nil {
		log.Printf("[Internal Warmup IMAP] Error: %v", err)
		return
	}
	defer rows.Close()

	// Collect all senders first
	type senderInfo struct {
		senderID, senderEmail, imapHost         string
		imapPort                                int
		imapPassword, imapTLSMode               string
		smtpID, smtpHost                        string
		smtpPort                                int
		smtpUsername, smtpPassword, smtpTLSMode string
	}
	var senders []senderInfo

	for rows.Next() {
		var s senderInfo
		rows.Scan(&s.senderID, &s.senderEmail, &s.imapHost, &s.imapPort, &s.imapPassword, &s.imapTLSMode,
			&s.smtpID, &s.smtpHost, &s.smtpPort, &s.smtpUsername, &s.smtpPassword, &s.smtpTLSMode)
		senders = append(senders, s)
	}

	if len(senders) == 0 {
		return
	}

	// Limit concurrent IMAP connections to 3 to avoid overloading mail servers
	const maxConcurrentIMAP = 3
	semaphore := make(chan struct{}, maxConcurrentIMAP)
	var wg sync.WaitGroup

	log.Printf("[Internal Warmup IMAP] Processing %d senders with max %d concurrent connections", len(senders), maxConcurrentIMAP)

	for i, sender := range senders {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore slot

		// Small delay between starting connections to prevent burst
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		go func(sender senderInfo) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore slot

			s.processOneSenderIMAP(sender.senderID, sender.senderEmail, sender.imapHost, sender.imapPort,
				sender.imapPassword, sender.imapTLSMode, sender.smtpID, sender.smtpHost, sender.smtpPort,
				sender.smtpUsername, sender.smtpPassword, sender.smtpTLSMode)
		}(sender)
	}

	wg.Wait()
	log.Printf("[Internal Warmup IMAP] Finished processing all senders")
}

func (s *Server) processOneSenderIMAP(senderID, senderEmail, imapHost string, imapPort int, imapPassword, imapTLSMode,
	smtpID, smtpHost string, smtpPort int, smtpUsername, smtpPassword, smtpTLSMode string) {

	addr := fmt.Sprintf("%s:%d", imapHost, imapPort)

	var c *client.Client
	var err error

	// Create dialer with timeout
	dialer := &net.Dialer{Timeout: 30 * time.Second}

	// Connect based on TLS mode
	switch imapTLSMode {
	case "tls":
		// First establish TCP connection with timeout
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Internal Warmup IMAP] TCP dial failed for %s: %v", senderEmail, dialErr)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				dialErr.Error(), senderID)
			return
		}
		// Upgrade to TLS
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         imapHost,
			InsecureSkipVerify: true,
		})
		tlsConn.SetDeadline(time.Now().Add(30 * time.Second))
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			log.Printf("[Internal Warmup IMAP] TLS handshake failed for %s: %v", senderEmail, err)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				err.Error(), senderID)
			return
		}
		c, err = client.New(tlsConn)
	case "starttls":
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Internal Warmup IMAP] TCP dial failed for %s: %v", senderEmail, dialErr)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				dialErr.Error(), senderID)
			return
		}
		c, err = client.New(conn)
		if err != nil {
			conn.Close()
			log.Printf("[Internal Warmup IMAP] Failed to connect for %s: %v", senderEmail, err)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				err.Error(), senderID)
			return
		}
		c.Timeout = 60 * time.Second
		tlsConfig := &tls.Config{
			ServerName:         imapHost,
			InsecureSkipVerify: true,
		}
		if err = c.StartTLS(tlsConfig); err != nil {
			c.Logout()
			log.Printf("[Internal Warmup IMAP] STARTTLS failed for %s: %v", senderEmail, err)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				err.Error(), senderID)
			return
		}
	default: // "none"
		conn, dialErr := dialer.Dial("tcp", addr)
		if dialErr != nil {
			log.Printf("[Internal Warmup IMAP] TCP dial failed for %s: %v", senderEmail, dialErr)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				dialErr.Error(), senderID)
			return
		}
		c, err = client.New(conn)
	}

	if err != nil {
		log.Printf("[Internal Warmup IMAP] Failed to connect for %s: %v", senderEmail, err)
		s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
			err.Error(), senderID)
		return
	}
	defer c.Logout()

	// Set timeout for IMAP operations
	c.Timeout = 60 * time.Second

	if err := c.Login(senderEmail, imapPassword); err != nil {
		log.Printf("[Internal Warmup IMAP] Login failed for %s: %v", senderEmail, err)
		s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
			err.Error(), senderID)
		return
	}

	// Update status to active
	s.db.Exec(`UPDATE smtp_senders SET imap_status = 'active', imap_error = NULL, imap_last_check = NOW() WHERE id = $1`, senderID)

	// Check INBOX for internal warmup emails
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return
	}

	if mbox.Messages == 0 {
		return
	}

	// Search for emails from the last 7 days
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().AddDate(0, 0, -7)

	ids, err := c.Search(criteria)
	if err != nil || len(ids) == 0 {
		return
	}

	// Limit to last 50 messages
	if len(ids) > 50 {
		ids = ids[len(ids)-50:]
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(ids...)

	messages := make(chan *imap.Message, 10)
	section := &imap.BodySectionName{Peek: true}
	go func() {
		c.Fetch(seqSet, []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}, messages)
	}()

	// Get reply_rate from warmup_smtps for this SMTP
	var replyRate int
	err = s.db.QueryRow(`SELECT COALESCE(reply_rate, 30) FROM warmup_smtps WHERE smtp_id = $1`, smtpID).Scan(&replyRate)
	if err != nil {
		replyRate = 30 // default
	}

	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		messageID := msg.Envelope.MessageId
		if messageID == "" {
			continue
		}

		cleanMessageID := strings.Trim(messageID, "<>")

		// Check if this is an internal warmup email
		if !strings.Contains(cleanMessageID, "@internal-warmup") {
			continue
		}

		log.Printf("[Internal Warmup IMAP] Found internal warmup email for %s: %s", senderEmail, msg.Envelope.Subject)

		// Mark as received in database (this counts as "inbox" since it wasn't in spam)
		s.db.Exec(`UPDATE warmup_internal_emails SET received = true, received_at = NOW() WHERE message_id = $1 AND received = false`, messageID)

		// Mark as read in IMAP
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}
		singleSeq := new(imap.SeqSet)
		singleSeq.AddNum(msg.SeqNum)
		c.Store(singleSeq, item, flags, nil)

		// Check if we should reply based on SMTP's reply_rate setting
		randomValue := rand.Intn(100)
		if randomValue < replyRate {
			// Send reply
			replySubject := "Re: " + msg.Envelope.Subject
			replyBody := getRandomReplyBody()

			// Get the sender's email from the original message
			var fromEmail string
			if len(msg.Envelope.From) > 0 {
				fromEmail = msg.Envelope.From[0].Address()
			}

			if fromEmail == "" {
				continue
			}

			replyMessageID := fmt.Sprintf("<%s@internal-warmup-reply>", uuid.New().String())

			// Send reply from this sender
			err = s.sendSMTPEmail(smtpHost, smtpPort, smtpUsername, smtpPassword, smtpTLSMode,
				senderEmail, fromEmail, replySubject, replyBody, replyMessageID)

			if err != nil {
				log.Printf("[Internal Warmup IMAP] Failed to reply from %s: %v", senderEmail, err)
			} else {
				log.Printf("[Internal Warmup IMAP] ↩️ Replied from %s to %s (rate: %d%%, roll: %d)", senderEmail, fromEmail, replyRate, randomValue)

				// Update the original email as replied
				s.db.Exec(`UPDATE warmup_internal_emails SET replied = true, replied_at = NOW() WHERE message_id = $1`, messageID)
			}
		} else {
			log.Printf("[Internal Warmup IMAP] Skipped reply for %s (rate: %d%%, roll: %d)", senderEmail, replyRate, randomValue)
		}
	}
}

// testSenderIMAP tests IMAP connection for a smtp_sender
func (s *Server) testSenderIMAP(c *fiber.Ctx) error {
	senderID := c.Params("id")

	var email, imapHost string
	var imapPort int
	var imapPassword string
	var imapTLSMode sql.NullString

	err := s.db.QueryRow(`
		SELECT email, imap_host, imap_port, imap_password, imap_tls_mode
		FROM smtp_senders WHERE id = $1
	`, senderID).Scan(&email, &imapHost, &imapPort, &imapPassword, &imapTLSMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Sender não encontrado"})
	}

	if imapHost == "" || imapPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "IMAP não configurado para este sender"})
	}

	// Get TLS mode, default to "tls" if not set
	tlsMode := "tls"
	if imapTLSMode.Valid && imapTLSMode.String != "" {
		tlsMode = imapTLSMode.String
	}

	// Test IMAP connection
	err = testIMAPConnectionWithMode(imapHost, imapPort, email, imapPassword, tlsMode)
	if err != nil {
		s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
			err.Error(), senderID)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	s.db.Exec(`UPDATE smtp_senders SET imap_status = 'active', imap_error = NULL, imap_last_check = NOW() WHERE id = $1`, senderID)

	return c.JSON(fiber.Map{"message": "Conexão IMAP bem sucedida"})
}

// updateSenderIMAP updates IMAP settings for a smtp_sender
func (s *Server) updateSenderIMAP(c *fiber.Ctx) error {
	senderID := c.Params("id")

	var req struct {
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPPassword string `json:"imap_password"`
		IMAPTLSMode  string `json:"imap_tls_mode"` // tls, starttls, none
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Requisição inválida"})
	}

	if req.IMAPPort == 0 {
		req.IMAPPort = 993
	}

	if req.IMAPTLSMode == "" {
		// Auto-detect based on port
		if req.IMAPPort == 993 {
			req.IMAPTLSMode = "tls"
		} else if req.IMAPPort == 143 {
			req.IMAPTLSMode = "starttls"
		} else {
			req.IMAPTLSMode = "none"
		}
	}

	_, err := s.db.Exec(`
		UPDATE smtp_senders
		SET imap_host = $1, imap_port = $2, imap_password = $3, imap_tls_mode = $4, imap_status = 'unchecked'
		WHERE id = $5
	`, req.IMAPHost, req.IMAPPort, req.IMAPPassword, req.IMAPTLSMode, senderID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "IMAP atualizado"})
}

// toggleInternalWarmup toggles internal warmup for a warmup SMTP
func (s *Server) toggleInternalWarmup(c *fiber.Ctx) error {
	id := c.Params("id")

	var current bool
	err := s.db.QueryRow(`SELECT internal_warmup FROM warmup_smtps WHERE id = $1`, id).Scan(&current)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP não encontrado"})
	}

	newValue := !current
	s.db.Exec(`UPDATE warmup_smtps SET internal_warmup = $1, updated_at = NOW() WHERE id = $2`, newValue, id)

	return c.JSON(fiber.Map{"internal_warmup": newValue, "message": "Aquecimento interno atualizado"})
}

// ============================================
// WARMUP SETTINGS
// ============================================

// getWarmupSettings returns all warmup settings
func (s *Server) getWarmupSettings(c *fiber.Ctx) error {
	userID := getUserID(c)
	rows, err := s.db.Query(`
		SELECT setting_key, setting_value, description
		FROM warmup_settings
		WHERE user_id = $1 OR user_id IS NULL
		ORDER BY setting_key
	`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao carregar configurações"})
	}
	defer rows.Close()

	settings := make(map[string]fiber.Map)
	for rows.Next() {
		var key, value string
		var description sql.NullString
		rows.Scan(&key, &value, &description)
		settings[key] = fiber.Map{
			"value":       value,
			"description": description.String,
		}
	}

	return c.JSON(fiber.Map{"settings": settings})
}

// updateWarmupSettings updates warmup settings
func (s *Server) updateWarmupSettings(c *fiber.Ctx) error {
	userID := getUserID(c)
	var req map[string]string
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Requisição inválida"})
	}

	updated := 0
	for key, value := range req {
		// Use UPSERT to insert or update user-specific settings
		result, err := s.db.Exec(`
			INSERT INTO warmup_settings (id, user_id, setting_key, setting_value, updated_at)
			VALUES (uuid_generate_v4(), $1, $2, $3, NOW())
			ON CONFLICT (user_id, setting_key) DO UPDATE
			SET setting_value = $3, updated_at = NOW()
		`, userID, key, value)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows > 0 {
				updated++
			}
		}
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d configurações atualizadas", updated),
		"updated": updated,
	})
}

// getWarmupSetting helper to get a single setting value (global, for background processes)
func (s *Server) getWarmupSetting(key string, defaultValue string) string {
	var value string
	err := s.db.QueryRow(`SELECT setting_value FROM warmup_settings WHERE setting_key = $1 AND user_id IS NULL`, key).Scan(&value)
	if err != nil {
		return defaultValue
	}
	return value
}

// getWarmupSettingForUser helper to get a user-specific setting value (falls back to global)
func (s *Server) getWarmupSettingForUser(userID string, key string, defaultValue string) string {
	var value string
	// First try to get user-specific setting
	err := s.db.QueryRow(`SELECT setting_value FROM warmup_settings WHERE setting_key = $1 AND user_id = $2`, key, userID).Scan(&value)
	if err == nil {
		return value
	}
	// Fall back to global setting
	err = s.db.QueryRow(`SELECT setting_value FROM warmup_settings WHERE setting_key = $1 AND user_id IS NULL`, key).Scan(&value)
	if err != nil {
		return defaultValue
	}
	return value
}

// getWarmupSettingInt helper to get a single setting as int
func (s *Server) getWarmupSettingInt(key string, defaultValue int) int {
	value := s.getWarmupSetting(key, "")
	if value == "" {
		return defaultValue
	}
	var result int
	fmt.Sscanf(value, "%d", &result)
	return result
}
