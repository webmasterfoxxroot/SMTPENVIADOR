-- Add proxy settings for Seed->SMTP warmup
INSERT INTO settings (key, value, description) VALUES
('proxy_enabled', 'false', 'Enable proxy for Seed->SMTP warmup'),
('proxy_host', '', 'Proxy hostname'),
('proxy_port', '', 'Proxy port'),
('proxy_user', '', 'Proxy username'),
('proxy_pass', '', 'Proxy password')
ON CONFLICT (key) DO NOTHING;
