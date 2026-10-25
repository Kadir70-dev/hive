import { describe, expect, it } from 'vitest';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { readEnv, validateEnv } from '../../scripts/staging/config-preflight.mjs';

function example() {
  const shared = 'shared-secret-not-real-but-more-than-32-chars';
  const siteURL = 'https://staging--hive-example.netlify.app';
  const apiURL = 'api-staging.example.test';
  const api = {
    POSTGRES_PASSWORD: 'postgres-example-not-secret-long-enough',
    REDIS_PASSWORD: 'redis-example-not-secret-long-enough-123',
    HIVE_PROXY_SHARED_SECRET: shared,
    MFA_ENCRYPTION_KEY: Buffer.alloc(32, 2).toString('base64'),
    BACKUP_AGE_RECIPIENT: 'age1' + 'a'.repeat(32),
    API_DOMAIN: apiURL, PUBLIC_APP_URL: siteURL,
    ALLOWED_ORIGINS: siteURL,
    MAIL_PROVIDER: 'log',
  };
  const site = {
    HIVE_API_INTERNAL_URL: 'https://' + apiURL,
    HIVE_PROXY_SHARED_SECRET: shared,
    NEXT_PUBLIC_SITE_URL: siteURL,
  };
  return { api, site };
}

describe('release configuration preflight', () => {
  it('accepts a coherent staging config with log-only email caveat', () => {
    const { api, site } = example();
    const result = validateEnv(api, site);
    expect(result.errors).toEqual([]);
    expect(result.notes.join(' ')).toContain('does not deliver emails');
  });
  it('rejects production settings without delivered email', () => {
    const { api, site } = example();
    expect(validateEnv(api, site, 'production').errors).toContain('MAIL_PROVIDER=log cannot deliver production email');
  });
  it('rejects placeholder secrets, wrong cross-service API URL and root production domain in staging', () => {
    const { api, site } = example();
    const result = validateEnv({
      ...api, POSTGRES_PASSWORD: 'change-this-now', API_DOMAIN: 'api.thehivesociety.ae',
      PUBLIC_APP_URL: 'https://thehivesociety.ae',
    }, site);
    expect(result.errors.join(' ')).toMatch(/POSTGRES_PASSWORD|Staging API_DOMAIN|Staging email links|does not match/);
  });
  it('never evals shell instructions or emits secret values in its CLI error output', () => {
    const dir=mkdtempSync(join(tmpdir(), 'hive-preflight-'));
    try {
      const path=join(dir, 'env');
      const secret='private-secret-must-not-appear-in-logs';
      writeFileSync(path, '# test\nPOSTGRES_PASSWORD=change-this-now\nINJECTED=$(echo ' + secret + ')\n');
      const run=spawnSync(process.execPath, [resolve('scripts/staging/config-preflight.mjs'),'--api-env',path], { encoding:'utf8' });
      expect(run.status).toBe(1);
      expect((run.stderr || '') + (run.stdout || '')).not.toContain(secret);
    } finally { rmSync(dir, { recursive:true, force:true }); }
  });
  it('parses quoted sender values without executing dotenv', () => {
    const parsed=readEnv('MAIL_FROM="The Hive Society <hello@mailer.example.test>"\n# comment\nHIVE_PROXY_SHARED_SECRET=abc\n');
    expect(parsed.MAIL_FROM).toBe('The Hive Society <hello@mailer.example.test>');
    expect(parsed.HIVE_PROXY_SHARED_SECRET).toBe('abc');
  });
});
