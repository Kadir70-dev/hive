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

const production = () => ({...base(), SITE_DOMAIN: 'thehivesociety.ae', PUBLIC_APP_URL: 'https://thehivesociety.ae',
  ALLOWED_ORIGINS: 'https://thehivesociety.ae', API_DOMAIN: 'api.thehivesociety.ae', HIVE_ENVIRONMENT: 'production',
  SITE_REDIRECT_FROM: 'www.thehivesociety.ae', MAIL_PROVIDER: 'resend', RESEND_API_KEY: 're_' + 'k'.repeat(30),
  MAIL_FROM: 'The Hive Society <hello@mailer.thehivesociety.ae>'});

test('Hostinger production: apex site with www redirect passes', () => {
  assert.deepEqual(validateEnv(production(), null, 'production').errors, []);
});

test('Hostinger production: staging noindex environment is rejected', () => {
  for (const HIVE_ENVIRONMENT of ['staging', undefined]) {
    const {errors} = validateEnv({...production(), HIVE_ENVIRONMENT}, null, 'production');
    assert.ok(errors.some(e => e.includes('HIVE_ENVIRONMENT must be production')));
  }
});

test('Hostinger production: www must redirect to the apex site', () => {
  const {errors} = validateEnv({...production(), SITE_REDIRECT_FROM: ''}, null, 'production');
  assert.ok(errors.some(e => e.includes('must include www.thehivesociety.ae')));
});

test('Hostinger redirect hosts must be hostnames other than the site', () => {
  let {errors} = validateEnv({...production(), SITE_REDIRECT_FROM: 'https://www.thehivesociety.ae'}, null, 'production');
  assert.ok(errors.some(e => e.includes('comma-separated hostnames')));
  ({errors} = validateEnv({...production(), SITE_REDIRECT_FROM: 'www.thehivesociety.ae,thehivesociety.ae'}, null, 'production'));
  assert.ok(errors.some(e => e.includes('must not include SITE_DOMAIN')));
});

test('Hostinger staging: production environment flag is rejected', () => {
  const {errors} = validateEnv({...base(), HIVE_ENVIRONMENT: 'production'}, null, 'staging');
  assert.ok(errors.some(e => e.includes('HIVE_ENVIRONMENT must be staging')));
});
