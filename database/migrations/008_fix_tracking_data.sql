-- Migration: Fix existing tracking data
-- This migration updates existing tracking_events with proper parsing of user_agent data

-- 1. Mark HeadlessChrome as bot
UPDATE tracking_events
SET is_bot = true,
    bot_type = 'headless_browser',
    bot_name = 'HeadlessChrome',
    bot_score = 90,
    device_type = 'bot'
WHERE LOWER(user_agent) LIKE '%headlesschrome%'
  AND (is_bot = false OR is_bot IS NULL);

-- 2. Mark other automation tools as bots
UPDATE tracking_events
SET is_bot = true,
    bot_type = 'automation',
    bot_name = CASE
        WHEN LOWER(user_agent) LIKE '%puppeteer%' THEN 'Puppeteer'
        WHEN LOWER(user_agent) LIKE '%selenium%' THEN 'Selenium'
        WHEN LOWER(user_agent) LIKE '%phantomjs%' THEN 'PhantomJS'
        WHEN LOWER(user_agent) LIKE '%playwright%' THEN 'Playwright'
        ELSE 'Automation Tool'
    END,
    bot_score = 90,
    device_type = 'bot'
WHERE (LOWER(user_agent) LIKE '%puppeteer%'
    OR LOWER(user_agent) LIKE '%selenium%'
    OR LOWER(user_agent) LIKE '%phantomjs%'
    OR LOWER(user_agent) LIKE '%playwright%'
    OR LOWER(user_agent) LIKE '%webdriver%')
  AND (is_bot = false OR is_bot IS NULL);

-- 3. Update truncated user-agents (Gmail proxy)
UPDATE tracking_events
SET browser = 'Email Proxy',
    device_type = 'unknown',
    email_client = 'Navegador Web'
WHERE user_agent = 'Mozilla/5.0'
  AND (browser IS NULL OR browser = '' OR browser = 'Unknown');

-- 4. Update empty user-agents
UPDATE tracking_events
SET browser = 'Unknown',
    os = 'Unknown',
    device = 'Unknown',
    device_type = 'unknown',
    email_client = 'Unknown'
WHERE (user_agent IS NULL OR user_agent = '')
  AND (browser IS NULL OR browser = '');

-- 5. Detect Chrome browser
UPDATE tracking_events
SET browser = 'Chrome',
    browser_version = COALESCE(
        SUBSTRING(user_agent FROM 'Chrome/([0-9]+)'),
        ''
    )
WHERE LOWER(user_agent) LIKE '%chrome%'
  AND LOWER(user_agent) NOT LIKE '%edge%'
  AND LOWER(user_agent) NOT LIKE '%edg/%'
  AND LOWER(user_agent) NOT LIKE '%opr%'
  AND LOWER(user_agent) NOT LIKE '%opera%'
  AND LOWER(user_agent) NOT LIKE '%headlesschrome%'
  AND (browser IS NULL OR browser = '')
  AND LENGTH(user_agent) > 50;

-- 6. Detect Firefox browser
UPDATE tracking_events
SET browser = 'Firefox',
    browser_version = COALESCE(
        SUBSTRING(user_agent FROM 'Firefox/([0-9]+)'),
        ''
    )
WHERE LOWER(user_agent) LIKE '%firefox%'
  AND (browser IS NULL OR browser = '');

-- 7. Detect Edge browser
UPDATE tracking_events
SET browser = 'Edge',
    browser_version = COALESCE(
        SUBSTRING(user_agent FROM 'Edg/([0-9]+)'),
        SUBSTRING(user_agent FROM 'Edge/([0-9]+)'),
        ''
    )
WHERE (LOWER(user_agent) LIKE '%edg/%' OR LOWER(user_agent) LIKE '%edge/%')
  AND (browser IS NULL OR browser = '');

-- 8. Detect Safari browser (not Chrome-based)
UPDATE tracking_events
SET browser = 'Safari',
    browser_version = COALESCE(
        SUBSTRING(user_agent FROM 'Version/([0-9]+)'),
        ''
    )
WHERE LOWER(user_agent) LIKE '%safari%'
  AND LOWER(user_agent) NOT LIKE '%chrome%'
  AND LOWER(user_agent) NOT LIKE '%chromium%'
  AND (browser IS NULL OR browser = '');

-- 9. Detect Windows OS
UPDATE tracking_events
SET os = 'Windows',
    os_version = CASE
        WHEN user_agent LIKE '%Windows NT 10.0%' THEN '10/11'
        WHEN user_agent LIKE '%Windows NT 6.3%' THEN '8.1'
        WHEN user_agent LIKE '%Windows NT 6.2%' THEN '8'
        WHEN user_agent LIKE '%Windows NT 6.1%' THEN '7'
        ELSE ''
    END
WHERE LOWER(user_agent) LIKE '%windows%'
  AND (os IS NULL OR os = '')
  AND is_bot = false;

-- 10. Detect macOS
UPDATE tracking_events
SET os = 'macOS'
WHERE (LOWER(user_agent) LIKE '%macintosh%' OR LOWER(user_agent) LIKE '%mac os x%')
  AND (os IS NULL OR os = '')
  AND is_bot = false;

-- 11. Detect iOS (iPhone/iPad)
UPDATE tracking_events
SET os = 'iOS',
    device_type = CASE
        WHEN LOWER(user_agent) LIKE '%ipad%' THEN 'tablet'
        ELSE 'mobile'
    END,
    device = CASE
        WHEN LOWER(user_agent) LIKE '%iphone%' THEN 'Apple iPhone'
        WHEN LOWER(user_agent) LIKE '%ipad%' THEN 'Apple iPad'
        ELSE 'Apple Device'
    END
WHERE (LOWER(user_agent) LIKE '%iphone%' OR LOWER(user_agent) LIKE '%ipad%')
  AND (os IS NULL OR os = '')
  AND is_bot = false;

-- 12. Detect Android
UPDATE tracking_events
SET os = 'Android',
    device_type = CASE
        WHEN LOWER(user_agent) LIKE '%mobile%' THEN 'mobile'
        ELSE 'tablet'
    END
WHERE LOWER(user_agent) LIKE '%android%'
  AND (os IS NULL OR os = '')
  AND is_bot = false;

-- 13. Detect Linux
UPDATE tracking_events
SET os = 'Linux',
    device_type = 'desktop'
WHERE LOWER(user_agent) LIKE '%linux%'
  AND LOWER(user_agent) NOT LIKE '%android%'
  AND (os IS NULL OR os = '')
  AND is_bot = false;

-- 14. Detect Microsoft Outlook
UPDATE tracking_events
SET email_client = 'Outlook'
WHERE (LOWER(user_agent) LIKE '%outlook%'
    OR LOWER(user_agent) LIKE '%ms-office%'
    OR LOWER(user_agent) LIKE '%msoffice%'
    OR LOWER(user_agent) LIKE '%microsoft office%')
  AND (email_client IS NULL OR email_client = '');

-- 15. Detect Foxmail
UPDATE tracking_events
SET email_client = 'Foxmail'
WHERE LOWER(user_agent) LIKE '%foxmail%'
  AND (email_client IS NULL OR email_client = '');

-- 15b. Detect Thunderbird
UPDATE tracking_events
SET email_client = 'Thunderbird'
WHERE LOWER(user_agent) LIKE '%thunderbird%'
  AND (email_client IS NULL OR email_client = '');

-- 15c. Detect Apple Mail
UPDATE tracking_events
SET email_client = 'Apple Mail'
WHERE (LOWER(user_agent) LIKE '%apple mail%' OR LOWER(user_agent) LIKE '%apple-mail%')
  AND (email_client IS NULL OR email_client = '');

-- 16. Set default email_client for browser access
UPDATE tracking_events
SET email_client = 'Navegador Web'
WHERE (email_client IS NULL OR email_client = '')
  AND browser IS NOT NULL
  AND browser != ''
  AND browser != 'Unknown'
  AND is_bot = false;

-- 17. Set default device_type for desktop browsers
UPDATE tracking_events
SET device_type = 'desktop',
    device = CASE
        WHEN os = 'Windows' THEN 'Windows PC'
        WHEN os = 'macOS' THEN 'Apple Mac'
        WHEN os = 'Linux' THEN 'Linux PC'
        ELSE device
    END
WHERE (device_type IS NULL OR device_type = '')
  AND os IN ('Windows', 'macOS', 'Linux')
  AND is_bot = false;

-- 18. Clean region names (remove "State of", "Province of", etc.)
UPDATE tracking_events
SET region = REGEXP_REPLACE(region, '^State of ', '', 'i')
WHERE region ILIKE 'State of %';

UPDATE tracking_events
SET region = REGEXP_REPLACE(region, '^Province of ', '', 'i')
WHERE region ILIKE 'Province of %';

UPDATE tracking_events
SET region = REGEXP_REPLACE(region, '^Region of ', '', 'i')
WHERE region ILIKE 'Region of %';

UPDATE tracking_events
SET region = REGEXP_REPLACE(region, '^Estado de ', '', 'i')
WHERE region ILIKE 'Estado de %';

-- Verify results
SELECT
    'Bots' as category,
    COUNT(*) FILTER (WHERE is_bot = true) as count
FROM tracking_events
UNION ALL
SELECT
    'Humans' as category,
    COUNT(*) FILTER (WHERE is_bot = false OR is_bot IS NULL) as count
FROM tracking_events
UNION ALL
SELECT
    'With Browser' as category,
    COUNT(*) FILTER (WHERE browser IS NOT NULL AND browser != '') as count
FROM tracking_events
UNION ALL
SELECT
    'With OS' as category,
    COUNT(*) FILTER (WHERE os IS NOT NULL AND os != '') as count
FROM tracking_events
UNION ALL
SELECT
    'With Email Client' as category,
    COUNT(*) FILTER (WHERE email_client IS NOT NULL AND email_client != '') as count
FROM tracking_events;
