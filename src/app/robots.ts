import type { MetadataRoute } from 'next';
export default function robots():MetadataRoute.Robots{
 const base=(process.env.NEXT_PUBLIC_SITE_URL || 'https://thehivesociety.ae').replace(/\/$/,'');
 return {rules:[{userAgent:'*',allow:'/',disallow:['/admin','/members','/profile','/gatherings','/auth','/reset-password','/api']}],sitemap:`${base}/sitemap.xml`};
}
