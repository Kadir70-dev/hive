import test from 'node:test';
import assert from 'node:assert/strict';
import { validateEnv } from './config-preflight.mjs';

const base = () => ({
  POSTGRES_PASSWORD: 'a'.repeat(40),
  REDIS_PASSWORD: 'b'.repeat(40),
  HIVE_PROXY_SHARED_SECRET: 'c'.repeat(40),
  MFA_ENCRYPTION_KEY: Buffer.alloc(32, 7).toString('base64'),
  BACKUP_AGE_RECIPIENT: 'age1' + 's'.repeat(55),
  API_DOMAIN: 'api-staging.thehivesociety.ae',
  SITE_DOMAIN: 'staging.thehivesociety.ae',
  PUBLIC_APP_URL: 'https://staging.thehivesociety.ae',
  ALLOWED_ORIGINS: 'https://staging.thehivesociety.ae',
  MAIL_PROVIDER: 'log',
});

test('Hostinger staging: valid matching site and origin', () => {
  const {errors} = validateEnv(base(), null, 'staging');
  assert.deepEqual(errors, []);
});

test('Hostinger staging: reject mismatched public site and allowed origins', () => {
  const config = {...base(), SITE_DOMAIN: 'other.example.test'};
  const {errors} = validateEnv(config, null, 'staging');
  assert.ok(errors.some(e => e.includes('SITE_DOMAIN must match PUBLIC_APP_URL')));
  assert.ok(errors.some(e => e.includes('SITE_DOMAIN must be present in ALLOWED_ORIGINS')));
});

test('Hostinger staging: reject domain specified as a URL', () => {
  const config = {...base(), SITE_DOMAIN: 'https://staging.thehivesociety.ae'};
  const {errors} = validateEnv(config, null, 'staging');
  assert.ok(errors.some(e => e.includes('SITE_DOMAIN must be a hostname')));
});

test('Production cannot use log-only email provider', () => {
  const config = {...base(), MAIL_PROVIDER: 'log'};
  const {errors} = validateEnv(config, null, 'production');
  assert.ok(errors.some(e => e.includes('cannot deliver production email')));
});
