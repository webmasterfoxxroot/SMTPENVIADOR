-- Migration: Add import batch size setting
-- This allows users to configure how many emails are processed at once during import

INSERT INTO settings (key, value, description) VALUES
('import_batch_size', '1000', 'Quantidade de emails processados por vez durante importação')
ON CONFLICT (key) DO NOTHING;
