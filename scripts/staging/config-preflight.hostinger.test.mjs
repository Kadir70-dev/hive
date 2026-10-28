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

const edgeProd = () => ({...production(), SITE_REDIRECT_FROM: '', HIVE_EDGE_ALIAS: 'hive-prod-web',
  HIVE_EDGE_NETWORK: 'hive-edge-prod', HIVE_WEB_IMAGE_SUFFIX: '-production', COMPOSE_PROJECT_NAME: 'hive-prod',
  HIVE_DOCKER_SUBNET: '10.250.71.0/24'});

test('Shared edge production: own network, alias, image, project and subnet pass', () => {
  assert.deepEqual(validateEnv(edgeProd(), null, 'production').errors, []);
});

test('Shared edge production: staging network/alias/image/project/subnet are rejected', () => {
  const {errors} = validateEnv({...edgeProd(), HIVE_EDGE_ALIAS: 'hive-staging-web', HIVE_EDGE_NETWORK: 'hive-edge-staging',
    HIVE_WEB_IMAGE_SUFFIX: '', COMPOSE_PROJECT_NAME: 'hive', HIVE_DOCKER_SUBNET: undefined}, null, 'production');
  for (const k of ['HIVE_EDGE_ALIAS must be hive-prod-web', 'HIVE_EDGE_NETWORK must be hive-edge-prod',
    'HIVE_WEB_IMAGE_SUFFIX', 'COMPOSE_PROJECT_NAME', 'HIVE_DOCKER_SUBNET'])
    assert.ok(errors.some(e => e.includes(k)), k);
});

test('Shared edge staging: must use the staging network and alias', () => {
  const ok = {...base(), HIVE_EDGE_ALIAS: 'hive-staging-web', HIVE_EDGE_NETWORK: 'hive-edge-staging'};
  assert.deepEqual(validateEnv(ok, null, 'staging').errors, []);
  const {errors} = validateEnv({...ok, HIVE_EDGE_NETWORK: 'hive-edge-prod'}, null, 'staging');
  assert.ok(errors.some(e => e.includes('HIVE_EDGE_NETWORK must be hive-edge-staging')));
});
