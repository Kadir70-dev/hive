#!/usr/bin/env node
/**
 * Offline release preflight: reads env files as text (never executes/sources
 * them), prints field names only, and never calls external services or DNS.
 *
 * node scripts/staging/config-preflight.mjs --mode staging --api-env PATH [--site-env PATH]
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

export function readEnv(text) {
  /** @type {Record<string, string>} */
  const env = {};
  for (const line of text.split(/\r?\n/)) {
    const match = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
    if (!match) continue;
    let value = match[2].trim();
    if ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'"))) value = value.slice(1, -1);
    env[match[1]] = value;
  }
  return env;
}

function missing(value) {
  return !value || /change-this|placeholder|replace-me|ci-dummy|set_in_secret_store|^todo$/i.test(value);
}
function httpsOrigin(value) {
  try {
    const u = new URL(value);
    return u.protocol === 'https:' && !u.username && !u.password &&
      u.origin === value && u.pathname === '/' && !u.search && !u.hash;
  } catch { return false; }
}
export function validateEnv(api, site, mode = 'staging') {
  const errors = [], notes = [];
  const required = (name) => { if (missing(api[name])) errors.push(name + ' is missing or placeholder'); };
  if (!['staging', 'production'].includes(mode)) errors.push('Invalid mode');
  for (const name of ['POSTGRES_PASSWORD','REDIS_PASSWORD','HIVE_PROXY_SHARED_SECRET',
    'MFA_ENCRYPTION_KEY','BACKUP_AGE_RECIPIENT','API_DOMAIN','PUBLIC_APP_URL',
    'ALLOWED_ORIGINS','MAIL_PROVIDER']) required(name);
  for (const name of ['POSTGRES_PASSWORD','REDIS_PASSWORD','HIVE_PROXY_SHARED_SECRET']) {
    if (api[name] && api[name].length < 32) errors.push(name + ' must be 32+ characters');
  }
  if (api.MFA_ENCRYPTION_KEY &&
    (!/^[A-Za-z0-9+/]+={0,2}$/.test(api.MFA_ENCRYPTION_KEY) ||
      Buffer.from(api.MFA_ENCRYPTION_KEY,'base64').length !== 32))
    errors.push('MFA_ENCRYPTION_KEY must be a base64-encoded 32-byte key');
  if (api.BACKUP_AGE_RECIPIENT && !/^age1[a-z0-9]{15,}$/.test(api.BACKUP_AGE_RECIPIENT))
    errors.push('BACKUP_AGE_RECIPIENT must look like an age public recipient');
  if (api.API_DOMAIN && !/^[a-z0-9.-]+$/.test(api.API_DOMAIN))
    errors.push('API_DOMAIN must be a hostname (not a URL)');
  if (api.PUBLIC_APP_URL && !httpsOrigin(api.PUBLIC_APP_URL))
    errors.push('PUBLIC_APP_URL must be an exact HTTPS origin');
  // The Hostinger compose overlay has one public site, not a separate API host.
  if (api.SITE_DOMAIN !== undefined) {
    required('SITE_DOMAIN');
    if (api.SITE_DOMAIN && !/^[a-z0-9.-]+$/.test(api.SITE_DOMAIN))
      errors.push('SITE_DOMAIN must be a hostname (not a URL)');
    if (api.SITE_DOMAIN && api.PUBLIC_APP_URL !== 'https://' + api.SITE_DOMAIN)
      errors.push('Hostinger SITE_DOMAIN must match PUBLIC_APP_URL');
    if (api.SITE_DOMAIN && !(api.ALLOWED_ORIGINS || '').split(',').map(x => x.trim()).includes('https://' + api.SITE_DOMAIN))
      errors.push('Hostinger SITE_DOMAIN must be present in ALLOWED_ORIGINS');
    // HIVE_ENVIRONMENT gates Caddy's noindex header and the build-time robots.txt.
    const env = api.HIVE_ENVIRONMENT || 'staging';
    if (env !== mode) errors.push('Hostinger HIVE_ENVIRONMENT must be ' + mode + ' (noindex/robots follow it)');
    const aliases = (api.SITE_REDIRECT_FROM || '').split(',').map(x => x.trim()).filter(Boolean);
    if (aliases.some(h => !/^[a-z0-9.-]+$/.test(h)))
      errors.push('SITE_REDIRECT_FROM must be comma-separated hostnames (not URLs)');
    if (aliases.includes(api.SITE_DOMAIN))
      errors.push('SITE_REDIRECT_FROM must not include SITE_DOMAIN');
    // Shared-edge layout (docker-compose.edge.yml): www redirects live in the
    // edge Caddy, and each environment needs its own network, alias and image.
    const edge = !!api.HIVE_EDGE_ALIAS || !!api.HIVE_EDGE_NETWORK;
    if (edge) {
      const want = mode === 'production'
        ? { HIVE_EDGE_ALIAS: 'hive-prod-web', HIVE_EDGE_NETWORK: 'hive-edge-prod' }
        : { HIVE_EDGE_ALIAS: 'hive-staging-web', HIVE_EDGE_NETWORK: 'hive-edge-staging' };
      for (const [k, v] of Object.entries(want)) if (api[k] !== v) errors.push(`Shared edge ${mode}: ${k} must be ${v}`);
      if (mode === 'production' && !api.HIVE_WEB_IMAGE_SUFFIX)
        errors.push('Shared edge production: set HIVE_WEB_IMAGE_SUFFIX (e.g. -production) so it never reuses the staging web image');
      if (mode === 'production' && (api.COMPOSE_PROJECT_NAME || 'hive') === 'hive')
        errors.push('Shared edge production: COMPOSE_PROJECT_NAME must differ from the staging project (hive)');
      if (mode === 'production' && (api.HIVE_DOCKER_SUBNET || '10.250.70.0/24') === '10.250.70.0/24')
        errors.push('Shared edge production: HIVE_DOCKER_SUBNET must differ from staging (10.250.70.0/24)');
    } else if (mode === 'production' && api.SITE_DOMAIN && !api.SITE_DOMAIN.startsWith('www.') &&
      !aliases.includes('www.' + api.SITE_DOMAIN))
      errors.push('Production SITE_REDIRECT_FROM must include www.' + api.SITE_DOMAIN);
  }
  for (const origin of (api.ALLOWED_ORIGINS || '').split(',')) {
    if (!httpsOrigin(origin.trim())) errors.push('ALLOWED_ORIGINS must contain exact HTTPS origins');
  }
  if (mode === 'staging') {
    if (api.PUBLIC_APP_URL === 'https://thehivesociety.ae')
      errors.push('Staging email links must not point to the production site');
    if (api.API_DOMAIN === 'api.thehivesociety.ae')
      errors.push('Staging API_DOMAIN must not be the production hostname');
  }
  if (api.MAIL_PROVIDER === 'resend') {
    required('RESEND_API_KEY');
    required('MAIL_FROM');
    if (api.MAIL_FROM && !/^[^<]*<[^\s<>@]+@[^\s<>@]+>$/.test(api.MAIL_FROM))
      errors.push('MAIL_FROM must contain a sender email address');
  } else if (api.MAIL_PROVIDER === 'log') {
    if (mode !== 'staging') errors.push('MAIL_PROVIDER=log cannot deliver production email');
    notes.push('Log mail provider does not deliver emails; final staging email test needs Resend.');
  } else if (api.MAIL_PROVIDER) errors.push('MAIL_PROVIDER must be resend or staging-only log');
  const r2Fields = ['R2_ENDPOINT','R2_BUCKET','R2_ACCESS_KEY_ID','R2_SECRET_ACCESS_KEY'];
  if (r2Fields.some(k => !!api[k])) {
    for (const key of r2Fields) required(key);
    if (api.R2_ENDPOINT && !api.R2_ENDPOINT.startsWith('https://'))
      errors.push('R2_ENDPOINT must be HTTPS');
  } else notes.push('Private photo storage is not configured; profile uploads will be disabled.');
  if (site) {
    for (const key of ['HIVE_API_INTERNAL_URL','HIVE_PROXY_SHARED_SECRET','NEXT_PUBLIC_SITE_URL']) {
      if (missing(site[key])) errors.push('Frontend ' + key + ' missing or placeholder');
    }
    if (site.HIVE_PROXY_SHARED_SECRET !== api.HIVE_PROXY_SHARED_SECRET)
      errors.push('Frontend/backend HIVE_PROXY_SHARED_SECRET mismatch');
    if (site.NEXT_PUBLIC_SITE_URL !== api.PUBLIC_APP_URL)
      errors.push('Frontend NEXT_PUBLIC_SITE_URL does not match API PUBLIC_APP_URL');
    if (site.HIVE_API_INTERNAL_URL !== 'https://' + (api.API_DOMAIN || ''))
      errors.push('Frontend API URL does not match API_DOMAIN');
  } else if (api.SITE_DOMAIN !== undefined)
    notes.push('Hostinger overlay supplies frontend environment inside Compose; no separate site-env file.');
  else notes.push('Netlify environment not inspected; supply --site-env to validate proxy settings.');
  notes.push('DNS, delivery, staging backup restore, real browsers and legal approvals are outside offline preflight.');
  return { errors, notes };
}

function run() {
  const args = process.argv.slice(2);
  const arg = name => { const i=args.indexOf(name); return i>=0 ? args[i+1] : undefined; };
  const mode=arg('--mode') || 'staging', apiPath=arg('--api-env'), sitePath=arg('--site-env');
  if (!apiPath || !['staging','production'].includes(mode)) {
    console.error('Usage: node scripts/staging/config-preflight.mjs --mode staging|production --api-env PATH [--site-env PATH]');
    process.exitCode=2; return;
  }
  try {
    const api=readEnv(readFileSync(apiPath,'utf8'));
    const site=sitePath ? readEnv(readFileSync(sitePath,'utf8')) : null;
    const result=validateEnv(api,site,mode);
    for (const error of result.errors) console.error('ERROR: '+error);
    for (const note of result.notes) console.log('NOTE: '+note);
    if (result.errors.length) { console.error('Configuration preflight FAILED ('+result.errors.length+' issue(s)).'); process.exitCode=1; }
    else console.log('Offline '+mode+' configuration preflight PASS (no service contacts, credentials never displayed).');
  } catch {
    // No file content or secrets are printed even on a parse/read failure.
    console.error('Cannot read one or more specified environment files.');
    process.exitCode=2;
  }
}
if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) run();
