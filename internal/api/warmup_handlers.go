package api

import (
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// DATABASE MIGRATIONS
// ============================================

func (s *Server) initWarmupTables() {
	// Check if migration needed
	var needsMigration bool

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

	// Check if warmup_emails table exists (might have failed to create)
	var emailsTableExists bool
	s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_emails')
	`).Scan(&emailsTableExists)

	// Also check if warmup_smtps exists but warmup_emails doesn't
	var smtpsTableExists bool
	s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_smtps')
	`).Scan(&smtpsTableExists)

	if smtpsTableExists && !emailsTableExists {
		needsMigration = true
	}

	if needsMigration {
		log.Println("[Warmup] Migrating tables to UUID schema...")
		// Drop ALL warmup tables in correct order due to foreign keys
		s.db.Exec(`DROP TABLE IF EXISTS warmup_daily_stats CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_emails CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_templates CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_seeds CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_smtps CASCADE`)
	}

	// Create warmup_smtps table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_smtps (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
			email VARCHAR(255) NOT NULL UNIQUE,
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
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)
	// Add new columns if they don't exist (migration)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS send_rate INT DEFAULT 50`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS reply_rate INT DEFAULT 50`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS emails_per_day INT DEFAULT 20`)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS auto_reply BOOLEAN DEFAULT true`)
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

	// Add total_sent column to warmup_seeds for tracking sent emails
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS total_sent INT DEFAULT 0`)

	// Add TLS mode columns to warmup_seeds (migration)
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS imap_tls_mode VARCHAR(20) DEFAULT 'tls'`) // tls, starttls, none
	s.db.Exec(`ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS smtp_tls_mode VARCHAR(20) DEFAULT 'starttls'`) // tls, starttls, none

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
			subject VARCHAR(500) NOT NULL,
			body TEXT NOT NULL,
			category VARCHAR(50) DEFAULT 'business',
			template_type VARCHAR(20) DEFAULT 'send',
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
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

	// Insert default warmup templates
	s.insertDefaultWarmupTemplates()

	// Create warmup_settings table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_settings (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			setting_key VARCHAR(100) UNIQUE NOT NULL,
			setting_value TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_settings table: %v", err)
	}

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
	rows, err := s.db.Query(`
		SELECT
			w.id, w.smtp_id, s.name as smtp_name, w.status, w.recipe_type,
			w.start_date, w.end_date, w.current_day,
			w.min_emails_per_day, w.max_emails_per_day, w.send_rate, w.reply_rate,
			w.start_hour, w.end_hour,
			w.total_sent, w.total_inbox, w.total_spam, w.total_replies,
			w.custom_schedule, w.internal_warmup, w.created_at, w.updated_at
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		ORDER BY w.created_at DESC
	`)
	if err != nil {
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

	// Validate SMTP exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM smtp_servers WHERE id = $1)`, req.SMTPID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP not found"})
	}

	// Check if already enrolled
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM warmup_smtps WHERE smtp_id = $1)`, req.SMTPID).Scan(&exists)
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
			id, smtp_id, status, recipe_type, start_date, end_date,
			min_emails_per_day, max_emails_per_day, send_rate, reply_rate,
			start_hour, end_hour, custom_schedule
		) VALUES ($1, $2, 'active', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, req.SMTPID, req.RecipeType, startDate, endDate,
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
		WHERE id = $10
	`, req.Status, req.RecipeType, req.MinEmailsPerDay, req.MaxEmailsPerDay,
		req.SendRate, req.ReplyRate, req.StartHour, req.EndHour, customScheduleJSON, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Warmup settings updated"})
}

// deleteWarmupSMTP removes an SMTP from warmup program
func (s *Server) deleteWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_smtps WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "SMTP removed from warmup"})
}

// toggleWarmupSMTP toggles warmup on/off for an SMTP
func (s *Server) toggleWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var currentStatus string
	err := s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1`, id).Scan(&currentStatus)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	newStatus := "active"
	if currentStatus == "active" {
		newStatus = "paused"
	}

	s.db.Exec(`UPDATE warmup_smtps SET status = $1, updated_at = NOW() WHERE id = $2`, newStatus, id)

	return c.JSON(fiber.Map{"status": newStatus})
}

// triggerWarmupSMTP manually sends warmup emails for testing
func (s *Server) triggerWarmupSMTP(c *fiber.Ctx) error {
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
		WHERE w.id = $1
	`, id).Scan(&smtpID, &recipeType, &replyRate, &host, &port, &username, &password, &tlsMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	// Check for active seeds
	var activeSeeds int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
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

	// Get a random template
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true
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
	id := c.Params("id")

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
		WHERE id = $2
	`, string(jsonBytes), id)

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
	rows, err := s.db.Query(`
		SELECT
			ws.id, ws.email, ws.provider, ws.imap_host, ws.imap_port, ws.smtp_host, ws.smtp_port,
			ws.use_tls, ws.status, ws.last_check, ws.error_message, ws.created_at,
			COALESCE(ws.send_rate, 50), COALESCE(ws.reply_rate, 50),
			COALESCE(ws.emails_per_day, 20), COALESCE(ws.auto_reply, true),
			COALESCE(ws.imap_tls_mode, 'tls'), COALESCE(ws.smtp_tls_mode, 'starttls'),
			COALESCE(ws.total_sent, 0) + COALESCE(stats.total_replied, 0) as total_sent,
			COALESCE(stats.total_received, 0) as total_received,
			COALESCE(stats.total_inbox, 0) as total_inbox,
			COALESCE(stats.total_spam, 0) as total_spam,
			COALESCE(stats.total_moved, 0) as total_moved,
			COALESCE(stats.total_replied, 0) as total_replied
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
		ORDER BY ws.created_at DESC
	`)
	if err != nil {
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

		rows.Scan(&id, &email, &provider, &imapHost, &imapPort, &smtpHost, &smtpPort,
			&useTLS, &status, &lastCheck, &errorMsg, &createdAt,
			&sendRate, &replyRate, &emailsPerDay, &autoReply,
			&imapTLSMode, &smtpTLSMode,
			&totalSent, &totalReceived, &totalInbox, &totalSpam, &totalMoved, &totalReplied)

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
	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		Provider     string `json:"provider"`
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPTLSMode  string `json:"imap_tls_mode"` // tls, starttls, none
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPTLSMode  string `json:"smtp_tls_mode"` // tls, starttls, none
		SendRate     int    `json:"send_rate"`
		ReplyRate    int    `json:"reply_rate"`
		EmailsPerDay int    `json:"emails_per_day"`
		AutoReply    *bool  `json:"auto_reply"`
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

	_, err := s.db.Exec(`
		INSERT INTO warmup_seeds (id, email, password, provider, imap_host, imap_port, imap_tls_mode,
			smtp_host, smtp_port, smtp_tls_mode, send_rate, reply_rate, emails_per_day, auto_reply, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'active')
	`, id, req.Email, req.Password, req.Provider, req.IMAPHost, req.IMAPPort, req.IMAPTLSMode,
		req.SMTPHost, req.SMTPPort, req.SMTPTLSMode, req.SendRate, req.ReplyRate, req.EmailsPerDay, autoReply)

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
	id := c.Params("id")

	var req struct {
		Password     string `json:"password"`
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPTLSMode  string `json:"imap_tls_mode"`
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPTLSMode  string `json:"smtp_tls_mode"`
		SendRate     *int   `json:"send_rate"`
		ReplyRate    *int   `json:"reply_rate"`
		EmailsPerDay *int   `json:"emails_per_day"`
		AutoReply    *bool  `json:"auto_reply"`
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

	query += fmt.Sprintf(" WHERE id = $%d", paramIdx)
	params = append(params, id)

	_, err := s.db.Exec(query, params...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Seed updated"})
}

// deleteSeed removes a seed account
func (s *Server) deleteWarmupSeed(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_seeds WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Seed removed"})
}

// testWarmupSeed tests IMAP connection for a seed
func (s *Server) testWarmupSeed(c *fiber.Ctx) error {
	id := c.Params("id")

	var email, password, imapHost string
	var imapPort int
	var useTLS bool

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, use_tls
		FROM warmup_seeds WHERE id = $1
	`, id).Scan(&email, &password, &imapHost, &imapPort, &useTLS)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	// Test IMAP connection
	err = testIMAPConnection(imapHost, imapPort, email, password, useTLS)
	if err != nil {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), id)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1`, id)

	return c.JSON(fiber.Map{"message": "Connection successful"})
}

// toggleWarmupSeed toggles a seed between active and paused
func (s *Server) toggleWarmupSeed(c *fiber.Ctx) error {
	id := c.Params("id")

	var currentStatus string
	err := s.db.QueryRow(`SELECT status FROM warmup_seeds WHERE id = $1`, id).Scan(&currentStatus)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	newStatus := "paused"
	if currentStatus != "active" {
		newStatus = "active"
	}

	s.db.Exec(`UPDATE warmup_seeds SET status = $1 WHERE id = $2`, newStatus, id)

	return c.JSON(fiber.Map{"status": newStatus, "message": "Seed status updated"})
}

// triggerSeedSend forces the seed to send an email to a random SMTP
func (s *Server) triggerSeedSend(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get seed info
	var email, password, smtpHost string
	var smtpPort int
	err := s.db.QueryRow(`
		SELECT email, password, smtp_host, smtp_port
		FROM warmup_seeds WHERE id = $1
	`, id).Scan(&email, &password, &smtpHost, &smtpPort)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	if smtpHost == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Seed não tem SMTP configurado"})
	}

	// Get a random SMTP sender to send to
	var targetEmail string
	err = s.db.QueryRow(`
		SELECT ss.email
		FROM warmup_smtps w
		JOIN smtp_senders ss ON ss.smtp_id = w.smtp_id
		WHERE w.status = 'active' AND ss.active = true
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&targetEmail)

	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo encontrado"})
	}

	// Get random template
	var subject, body string
	s.db.QueryRow(`SELECT subject, body FROM warmup_templates WHERE active = true ORDER BY RANDOM() LIMIT 1`).Scan(&subject, &body)

	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))
	messageID := fmt.Sprintf("<%s@seed-warmup>", uuid.New().String())

	tlsMode := "starttls"
	if smtpPort == 465 {
		tlsMode = "tls"
	}

	err = s.sendSMTPEmail(smtpHost, smtpPort, email, password, tlsMode, email, targetEmail, subject, body, messageID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Update counter
	s.db.Exec(`UPDATE warmup_seeds SET total_sent = total_sent + 1 WHERE id = $1`, id)

	log.Printf("[Warmup Seed Manual] ✉️ Sent from %s to %s: %s", email, targetEmail, subject)

	return c.JSON(fiber.Map{"message": "Email enviado com sucesso", "to": targetEmail})
}

// ============================================
// WARMUP STATS HANDLERS
// ============================================

// getWarmupStats returns overall warmup statistics
func (s *Server) getWarmupStats(c *fiber.Ctx) error {
	var totalSMTPs, activeSMTPs, totalSeeds, activeSeeds int
	var totalSent, totalInbox, totalSpam, totalReplies int

	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps`).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds`).Scan(&totalSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)

	s.db.QueryRow(`
		SELECT COALESCE(SUM(total_sent), 0), COALESCE(SUM(total_inbox), 0),
			   COALESCE(SUM(total_spam), 0), COALESCE(SUM(total_replies), 0)
		FROM warmup_smtps
	`).Scan(&totalSent, &totalInbox, &totalSpam, &totalReplies)

	// Add internal warmup email counts
	var internalSent, internalReceived, internalReplies int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_internal_emails`).Scan(&internalSent)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_internal_emails WHERE received = true`).Scan(&internalReceived)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_internal_emails WHERE replied = true`).Scan(&internalReplies)
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
	now := time.Now()
	currentHour := now.Hour()

	// Check warmup enabled
	warmupEnabled := s.getWarmupSetting("warmup_enabled", "true")

	// Count active elements
	var activeSeeds, activeTemplates, activeSMTPs, smtpsWithInternal int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates WHERE active = true`).Scan(&activeTemplates)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active' AND internal_warmup = true`).Scan(&smtpsWithInternal)

	// SMTPs with active underlying smtp_server
	var smtpsWithActiveServer int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true
	`).Scan(&smtpsWithActiveServer)

	// SMTPs in valid hours for external warmup
	var smtpsInValidHours int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true
		AND $1 >= w.start_hour AND $1 < w.end_hour
	`, currentHour).Scan(&smtpsInValidHours)

	// SMTPs with internal warmup in valid hours
	var internalInValidHours int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active' AND s.active = true AND w.internal_warmup = true
		AND $1 >= w.start_hour AND $1 < w.end_hour
	`, currentHour).Scan(&internalInValidHours)

	// Senders with IMAP configured (needed for internal warmup)
	var sendersWithIMAP int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM smtp_senders ss
		JOIN smtp_servers s ON ss.smtp_id = s.id
		JOIN warmup_smtps w ON w.smtp_id = s.id
		WHERE ss.active = true AND s.active = true AND w.status = 'active' AND w.internal_warmup = true
		AND ss.imap_host IS NOT NULL AND ss.imap_host != ''
	`).Scan(&sendersWithIMAP)

	// Today's sent count
	var sentTodayExternal, sentTodayInternal int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_emails WHERE DATE(sent_at) = $1`, now.Format("2006-01-02")).Scan(&sentTodayExternal)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_internal_emails WHERE DATE(sent_at) = $1`, now.Format("2006-01-02")).Scan(&sentTodayInternal)

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
		Host          string `json:"host"`
		Status        string `json:"status"`
		Internal      bool   `json:"internal"`
		StartHour     int    `json:"start_hour"`
		EndHour       int    `json:"end_hour"`
		InHours       bool   `json:"in_hours"`
		TodayLimit    int    `json:"today_limit"`
		SentToday     int    `json:"sent_today"`
		ServerActive  bool   `json:"server_active"`
		SendersCount  int    `json:"senders_count"`
		SendersWithIMAP int  `json:"senders_with_imap"`
	}

	smtpRows, _ := s.db.Query(`
		SELECT s.host, w.status, w.internal_warmup, w.start_hour, w.end_hour,
		       w.min_emails_per_day, w.max_emails_per_day, w.recipe_type, w.start_date,
		       s.active, s.id
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
	`)
	defer smtpRows.Close()

	var smtpDetails []smtpDetail
	for smtpRows.Next() {
		var host, status, recipeType, serverID string
		var internal, serverActive bool
		var startHour, endHour, minEmails, maxEmails int
		var startDate time.Time
		smtpRows.Scan(&host, &status, &internal, &startHour, &endHour,
			&minEmails, &maxEmails, &recipeType, &startDate, &serverActive, &serverID)

		currentDay := int(now.Sub(startDate).Hours()/24) + 1
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
			Host:          host,
			Status:        status,
			Internal:      internal,
			StartHour:     startHour,
			EndHour:       endHour,
			InHours:       inHours,
			TodayLimit:    todayLimit,
			SentToday:     sentToday,
			ServerActive:  serverActive,
			SendersCount:  sendersCount,
			SendersWithIMAP: sendersWithIMAPCount,
		})
	}

	return c.JSON(fiber.Map{
		"timestamp":      now.Format("2006-01-02 15:04:05"),
		"current_hour":   currentHour,
		"warmup_enabled": warmupEnabled == "true",
		"external_warmup": fiber.Map{
			"active_seeds":      activeSeeds,
			"active_templates":  activeTemplates,
			"active_smtps":      smtpsWithActiveServer,
			"smtps_in_hours":    smtpsInValidHours,
			"sent_today":        sentTodayExternal,
		},
		"internal_warmup": fiber.Map{
			"smtps_with_internal":  smtpsWithInternal,
			"smtps_in_hours":       internalInValidHours,
			"senders_with_imap":    sendersWithIMAP,
			"sent_today":           sentTodayInternal,
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

// getWarmupActivity returns recent warmup activity (including internal warmup)
func (s *Server) getWarmupActivity(c *fiber.Ctx) error {
	// Query both regular warmup emails and internal warmup emails
	rows, err := s.db.Query(`
		(
			SELECT DISTINCT ON (e.id) e.id, e.subject, e.status, e.sent_at,
				   COALESCE(
				       (SELECT email FROM smtp_senders WHERE smtp_id = sm.id AND active = true LIMIT 1),
				       sm.username,
				       ''
				   ) as from_email,
				   s.email as to_email,
				   'seed' as warmup_type
			FROM warmup_emails e
			JOIN warmup_seeds s ON e.seed_id = s.id
			JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
			JOIN smtp_servers sm ON w.smtp_id = sm.id
		)
		UNION ALL
		(
			SELECT ie.id, ie.subject, ie.status, ie.sent_at,
				   ie.from_sender_email as from_email,
				   COALESCE(ss.email, '') as to_email,
				   'internal' as warmup_type
			FROM warmup_internal_emails ie
			LEFT JOIN smtp_senders ss ON ie.to_sender_id = ss.id
		)
		ORDER BY sent_at DESC
		LIMIT 50
	`)
	if err != nil {
		// If warmup_internal_emails table doesn't exist yet, fall back to original query
		rows, err = s.db.Query(`
			SELECT DISTINCT ON (e.id) e.id, e.subject, e.status, e.sent_at,
				   COALESCE(
				       (SELECT email FROM smtp_senders WHERE smtp_id = sm.id AND active = true LIMIT 1),
				       sm.username,
				       ''
				   ) as from_email,
				   s.email as to_email,
				   'seed' as warmup_type
			FROM warmup_emails e
			JOIN warmup_seeds s ON e.seed_id = s.id
			JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
			JOIN smtp_servers sm ON w.smtp_id = sm.id
			ORDER BY e.id, e.sent_at DESC
			LIMIT 50
		`)
		if err != nil {
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
	rows, err := s.db.Query(`
		SELECT id, subject, body, category, COALESCE(template_type, 'send'), active, created_at
		FROM warmup_templates
		ORDER BY template_type, category, created_at
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var templates []fiber.Map
	for rows.Next() {
		var id, subject, body, category, templateType string
		var active bool
		var createdAt time.Time

		rows.Scan(&id, &subject, &body, &category, &templateType, &active, &createdAt)

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
		INSERT INTO warmup_templates (id, subject, body, category, template_type, active)
		VALUES ($1, $2, $3, $4, $5, true)
	`, id, req.Subject, req.Body, req.Category, req.TemplateType)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"id": id, "message": "Template created"})
}

// deleteWarmupTemplate deletes a warmup template
func (s *Server) deleteWarmupTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_templates WHERE id = $1`, id)
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
	var imapTLSMode sql.NullString

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, COALESCE(imap_tls_mode, 'tls')
		FROM warmup_seeds WHERE id = $1
	`, seedID).Scan(&email, &password, &imapHost, &imapPort, &imapTLSMode)

	if err != nil {
		return
	}

	tlsMode := "tls"
	if imapTLSMode.Valid && imapTLSMode.String != "" {
		tlsMode = imapTLSMode.String
	}

	err = testIMAPConnectionWithMode(imapHost, imapPort, email, password, tlsMode)
	if err != nil {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
	} else {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1`, seedID)
	}
}

// testSeedConnectionPreview tests IMAP/SMTP connection WITHOUT saving (for preview before creating)
func (s *Server) testSeedConnectionPreview(c *fiber.Ctx) error {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		IMAPHost    string `json:"imap_host"`
		IMAPPort    int    `json:"imap_port"`
		IMAPTLSMode string `json:"imap_tls_mode"` // tls, starttls, none
		SMTPHost    string `json:"smtp_host"`
		SMTPPort    int    `json:"smtp_port"`
		SMTPTLSMode string `json:"smtp_tls_mode"` // tls, starttls, none
		TestType    string `json:"test_type"`     // imap, smtp, both
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

			err := testIMAPConnectionWithMode(req.IMAPHost, req.IMAPPort, req.Email, req.Password, req.IMAPTLSMode)
			if err != nil {
				results["imap"] = fiber.Map{"success": false, "error": err.Error()}
			} else {
				results["imap"] = fiber.Map{"success": true, "message": "Conexão IMAP OK!"}
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

			// Test SMTP connection
			err := s.testSMTPConnection(req.SMTPHost, req.SMTPPort, req.Email, req.Password, req.SMTPTLSMode)
			if err != nil {
				results["smtp"] = fiber.Map{"success": false, "error": err.Error()}
			} else {
				results["smtp"] = fiber.Map{"success": true, "message": "Conexão SMTP OK!"}
			}
		}
	}

	return c.JSON(results)
}

// testSMTPConnection tests SMTP connection without sending email
func (s *Server) testSMTPConnection(host string, port int, username, password, tlsMode string) error {
	addr := fmt.Sprintf("%s:%d", host, port)

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

		if err := s.authenticateSMTP(client, host, username, password); err != nil {
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

		if err := s.authenticateSMTP(client, host, username, password); err != nil {
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

		if err := s.authenticateSMTP(client, host, username, password); err != nil {
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

func (s *Server) startWarmupEngine() {
	log.Println("[Warmup Engine] Starting...")

	// Run IMAP check immediately on startup
	go s.processIMAPInteractions()

	// Run internal warmup immediately on startup
	go s.processInternalWarmup()

	// Run internal warmup IMAP check immediately on startup
	go s.processInternalWarmupIMAP()

	// Run external warmup (SMTP → Seeds) immediately on startup
	go s.processWarmupEmails()

	// Run every minute to check and send warmup emails
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Get configurable intervals from settings
	imapInterval := s.getWarmupSettingInt("imap_check_interval", 5)
	internalCycleInterval := s.getWarmupSettingInt("internal_cycle_minutes", 2)

	log.Printf("[Warmup Engine] Intervals: IMAP=%dm, Internal=%dm", imapInterval, internalCycleInterval)

	// Also run IMAP check (configurable, default 5 minutes)
	imapTicker := time.NewTicker(time.Duration(imapInterval) * time.Minute)
	defer imapTicker.Stop()

	// Run seed-to-SMTP emails every 3 minutes
	seedToSMTPTicker := time.NewTicker(3 * time.Minute)
	defer seedToSMTPTicker.Stop()

	// Run internal warmup (configurable, default 2 minutes)
	internalWarmupTicker := time.NewTicker(time.Duration(internalCycleInterval) * time.Minute)
	defer internalWarmupTicker.Stop()

	// Run internal warmup IMAP check (same as IMAP interval)
	internalImapTicker := time.NewTicker(time.Duration(imapInterval) * time.Minute)
	defer internalImapTicker.Stop()

	log.Printf("[Warmup Engine] Started successfully - waiting for tickers...")

	for {
		select {
		case <-ticker.C:
			s.processWarmupEmails()
		case <-imapTicker.C:
			s.processIMAPInteractions()
		case <-seedToSMTPTicker.C:
			s.processSeedToSMTPEmails()
		case <-internalWarmupTicker.C:
			s.processInternalWarmup()
		case <-internalImapTicker.C:
			s.processInternalWarmupIMAP()
		}
	}
}

func (s *Server) processWarmupEmails() {
	now := time.Now()
	currentHour := now.Hour()

	// Log active counts
	var activeSMTPs, activeSeeds, activeTemplates int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
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

	// Get active warmup SMTPs that should send now
	rows, err := s.db.Query(`
		SELECT w.id, w.smtp_id, w.start_date, w.min_emails_per_day, w.max_emails_per_day,
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
		var warmupID, smtpID, recipeType string
		var startDate time.Time
		var minEmails, maxEmails, replyRate, startHour, endHour int
		var customSchedule sql.NullString
		var host, username, password, tlsMode string
		var port int

		rows.Scan(&warmupID, &smtpID, &startDate, &minEmails, &maxEmails,
			&recipeType, &customSchedule, &replyRate, &startHour, &endHour,
			&host, &port, &username, &password, &tlsMode)

		// Calculate current day based on start date
		currentDay := int(now.Sub(startDate).Hours()/24) + 1
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

		// Calculate how many to send this minute (spread throughout the day)
		remaining := todayLimit - sentToday
		sendingHours := endHour - startHour
		if sendingHours <= 0 {
			sendingHours = 1
		}
		sendingMinutes := sendingHours * 60
		emailsPerMinute := remaining / sendingMinutes

		// Always send at least 1 if there's remaining quota (probabilistic for low volume)
		if emailsPerMinute < 1 && remaining > 0 {
			// Calculate probability: remaining emails / remaining minutes in window
			currentMinuteInWindow := (currentHour-startHour)*60 + time.Now().Minute()
			remainingMinutes := sendingMinutes - currentMinuteInWindow
			if remainingMinutes <= 0 {
				remainingMinutes = 1
			}
			probability := float64(remaining) / float64(remainingMinutes)
			if rand.Float64() < probability || remaining >= remainingMinutes {
				emailsPerMinute = 1
			}
		}

		if emailsPerMinute > remaining {
			emailsPerMinute = remaining
		}

		// Send warmup emails
		log.Printf("[Warmup Engine] SMTP %s: Sending %d emails this minute (remaining: %d)", warmupID, emailsPerMinute, remaining)
		for i := 0; i < emailsPerMinute; i++ {
			s.sendWarmupEmail(warmupID, smtpID, host, port, username, password, tlsMode, replyRate)
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

func (s *Server) sendWarmupEmail(warmupID, smtpID, host string, port int, username, password, tlsMode string, replyRate int) {
	// Get a random active seed
	var seedID, seedEmail string
	err := s.db.QueryRow(`
		SELECT id, email FROM warmup_seeds
		WHERE status = 'active'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&seedID, &seedEmail)

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

	// Get a random template
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true
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

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial error: %v", err)
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

func (s *Server) processSeedToSMTPEmails() {
	now := time.Now()
	currentHour := now.Hour()

	// Only send during business hours (6-22)
	if currentHour < 6 || currentHour > 22 {
		log.Printf("[Warmup Seed→SMTP] Outside sending hours (current: %d)", currentHour)
		return
	}

	// Get active seeds with SMTP settings AND their warmup config
	seedRows, err := s.db.Query(`
		SELECT id, email, password, smtp_host, smtp_port, use_tls,
		       COALESCE(send_rate, 50), COALESCE(emails_per_day, 20)
		FROM warmup_seeds
		WHERE status = 'active' AND smtp_host IS NOT NULL AND smtp_host != ''
	`)
	if err != nil {
		log.Printf("[Warmup Seed→SMTP] Error getting seeds: %v", err)
		return
	}
	defer seedRows.Close()

	type seedInfo struct {
		ID           string
		Email        string
		Password     string
		SMTPHost     string
		SMTPPort     int
		UseTLS       bool
		SendRate     int
		EmailsPerDay int
	}

	var seeds []seedInfo
	for seedRows.Next() {
		var seed seedInfo
		seedRows.Scan(&seed.ID, &seed.Email, &seed.Password, &seed.SMTPHost, &seed.SMTPPort, &seed.UseTLS,
			&seed.SendRate, &seed.EmailsPerDay)
		seeds = append(seeds, seed)
	}

	if len(seeds) == 0 {
		log.Printf("[Warmup Seed→SMTP] No active seeds with SMTP configured")
		return
	}

	log.Printf("[Warmup Seed→SMTP] Found %d seeds with SMTP", len(seeds))

	// Get active warmup SMTPs and their senders
	smtpRows, err := s.db.Query(`
		SELECT w.id, w.smtp_id, ss.email as sender_email
		FROM warmup_smtps w
		JOIN smtp_senders ss ON ss.smtp_id = w.smtp_id
		WHERE w.status = 'active' AND ss.active = true
	`)
	if err != nil {
		log.Printf("[Warmup Seed→SMTP] Error getting SMTP senders: %v", err)
		return
	}
	defer smtpRows.Close()

	var targets []struct {
		WarmupID    string
		SMTPID      string
		SenderEmail string
	}

	for smtpRows.Next() {
		var target struct {
			WarmupID    string
			SMTPID      string
			SenderEmail string
		}
		smtpRows.Scan(&target.WarmupID, &target.SMTPID, &target.SenderEmail)
		targets = append(targets, target)
	}

	if len(targets) == 0 {
		log.Printf("[Warmup Seed→SMTP] No active SMTP senders to send to")
		return
	}

	log.Printf("[Warmup Seed→SMTP] Found %d SMTP senders as targets", len(targets))

	// Process each seed
	totalSent := 0
	for _, seed := range seeds {
		// Check how many this seed already sent today
		var sentToday int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_emails
			WHERE seed_id = $1 AND DATE(sent_at) = $2 AND status = 'sent'
		`, seed.ID, now.Format("2006-01-02")).Scan(&sentToday)

		// Check daily limit
		if sentToday >= seed.EmailsPerDay {
			log.Printf("[Warmup Seed→SMTP] Seed %s: Daily limit reached (%d/%d)", seed.Email, sentToday, seed.EmailsPerDay)
			continue
		}

		// Calculate how many to send this cycle
		// Seed→SMTP runs every 3 minutes = 20 cycles per hour
		// Sending hours = 16 (6-22), total cycles = 320
		sendingHours := 16
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
			// Pick random target
			target := targets[rand.Intn(len(targets))]

			// Get a random template
			var subject, body string
			err = s.db.QueryRow(`
				SELECT subject, body FROM warmup_templates
				WHERE active = true ORDER BY RANDOM() LIMIT 1
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
			err = s.sendSMTPEmail(seed.SMTPHost, seed.SMTPPort, seed.Email, seed.Password, tlsMode,
				seed.Email, target.SenderEmail, subject, body, messageID)

			if err != nil {
				log.Printf("[Warmup Seed→SMTP] Failed to send from %s to %s: %v", seed.Email, target.SenderEmail, err)
				continue
			}

			// Update seed's sent counter
			s.db.Exec(`UPDATE warmup_seeds SET total_sent = total_sent + 1 WHERE id = $1`, seed.ID)

			log.Printf("[Warmup Seed→SMTP] ✉️ Sent from %s to %s: %s", seed.Email, target.SenderEmail, subject)
			totalSent++

			// Small delay between sends
			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("[Warmup Seed→SMTP] Cycle complete: sent %d emails", totalSent)
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

	// Get all active seeds
	rows, err := s.db.Query(`
		SELECT id, email, password, imap_host, imap_port, use_tls
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

		rows.Scan(&seedID, &email, &password, &imapHost, &imapPort, &useTLS)

		log.Printf("[Warmup IMAP] Processing inbox for %s", email)
		go s.processOneSeedInbox(seedID, email, password, imapHost, imapPort, useTLS)
	}
}

func (s *Server) processOneSeedInbox(seedID, email, password, imapHost string, imapPort int, useTLS bool) {
	addr := fmt.Sprintf("%s:%d", imapHost, imapPort)

	var c *client.Client
	var err error

	if useTLS || imapPort == 993 {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: imapHost})
	} else {
		c, err = client.Dial(addr)
	}

	if err != nil {
		log.Printf("[Warmup IMAP] Failed to connect to %s: %v", email, err)
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
		return
	}
	defer c.Logout()

	if err := c.Login(email, password); err != nil {
		log.Printf("[Warmup IMAP] Login failed for %s: %v", email, err)
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
		return
	}

	// Update last check
	s.db.Exec(`UPDATE warmup_seeds SET last_check = NOW(), status = 'active', error_message = NULL WHERE id = $1`, seedID)

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
		return
	}

	// Update stats
	s.db.Exec(`UPDATE warmup_emails SET replied_at = NOW(), status = 'replied' WHERE id = $1`, warmupEmailID)
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
		// Calculate current day based on start date
		currentDay := int(now.Sub(fromSMTP.StartDate).Hours()/24) + 1
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

			// Get a random template
			var subject, body string
			err = s.db.QueryRow(`
				SELECT subject, body FROM warmup_templates
				WHERE active = true ORDER BY RANDOM() LIMIT 1
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
		senderID, senderEmail, imapHost    string
		imapPort                           int
		imapPassword, imapTLSMode          string
		smtpID, smtpHost                   string
		smtpPort                           int
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

	// Connect based on TLS mode
	switch imapTLSMode {
	case "tls":
		c, err = client.DialTLS(addr, &tls.Config{
			ServerName:         imapHost,
			InsecureSkipVerify: true,
		})
	case "starttls":
		c, err = client.Dial(addr)
		if err != nil {
			log.Printf("[Internal Warmup IMAP] Failed to connect for %s: %v", senderEmail, err)
			s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
				err.Error(), senderID)
			return
		}
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
		c, err = client.Dial(addr)
	}

	if err != nil {
		log.Printf("[Internal Warmup IMAP] Failed to connect for %s: %v", senderEmail, err)
		s.db.Exec(`UPDATE smtp_senders SET imap_status = 'error', imap_error = $1, imap_last_check = NOW() WHERE id = $2`,
			err.Error(), senderID)
		return
	}
	defer c.Logout()

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
		IMAPHost    string `json:"imap_host"`
		IMAPPort    int    `json:"imap_port"`
		IMAPPassword string `json:"imap_password"`
		IMAPTLSMode string `json:"imap_tls_mode"` // tls, starttls, none
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
	rows, err := s.db.Query(`
		SELECT setting_key, setting_value, description
		FROM warmup_settings
		ORDER BY setting_key
	`)
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
	var req map[string]string
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Requisição inválida"})
	}

	updated := 0
	for key, value := range req {
		result, err := s.db.Exec(`
			UPDATE warmup_settings
			SET setting_value = $1, updated_at = NOW()
			WHERE setting_key = $2
		`, value, key)
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

// getWarmupSetting helper to get a single setting value
func (s *Server) getWarmupSetting(key string, defaultValue string) string {
	var value string
	err := s.db.QueryRow(`SELECT setting_value FROM warmup_settings WHERE setting_key = $1`, key).Scan(&value)
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
