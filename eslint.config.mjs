import { defineConfig, globalIgnores } from 'eslint/config';
import nextVitals from 'eslint-config-next/core-web-vitals';
import nextTs from 'eslint-config-next/typescript';
export default defineConfig([
  ...nextVitals,
  ...nextTs,
  // public/vendor holds third-party minified builds (marked, DOMPurify) served as-is.
  globalIgnores(['.next/**', 'out/**', 'build/**', 'dist/**', 'next-env.d.ts', 'public/vendor/**']),
]);
