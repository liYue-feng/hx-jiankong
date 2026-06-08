const Database = require('better-sqlite3');
const crypto = require('crypto');

const db = new Database('C:/Users/23906/.codepilot/codepilot.db');

const providerId = crypto.randomBytes(16).toString('hex');
const modelId = crypto.randomBytes(16).toString('hex');

// Add provider
const now = new Date().toISOString().replace('T', ' ').replace('Z', '');
db.prepare(`
  INSERT INTO api_providers (id, name, provider_type, protocol, base_url, api_key, is_active, sort_order, extra_env, notes, headers_json, env_overrides_json, role_models_json, options_json)
  VALUES (?, ?, 'anthropic', '', ?, ?, 1, 10, '{}', '', '{}', '', '{}', '{}')
`).run(providerId, '公司', 'https://api.deepseek.com/anthropic', 'sk-577b40852bbb4bb586782059fba42427');

// Add model
const modelId2 = crypto.randomBytes(16).toString('hex');
db.prepare(`
  INSERT INTO provider_models (id, provider_id, model_id, upstream_model_id, display_name, capabilities_json, variants_json, sort_order, enabled)
  VALUES (?, ?, 'deepseek-chat', '', 'DeepSeek V4 (公司)', '{}', '{}', 0, 1)
`).run(modelId2, providerId);

console.log('Provider added:', providerId);
console.log('Model added:', modelId2);

db.close();
console.log('Done!');
