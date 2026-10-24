import type { NextConfig } from 'next';
const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Retain existing image serving via public/; no experimental features required.
};
export default nextConfig;
