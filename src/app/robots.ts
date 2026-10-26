import type { MetadataRoute } from 'next';
export default function robots():MetadataRoute.Robots{
 const base=(process.env.NEXT_PUBLIC_SITE_URL || 'https://thehivesociety.ae').replace(/\/$/,'');
 // Robots is generated at build time; staging must not leak production URLs.
 if(process.env.HIVE_ENVIRONMENT === 'staging'){
   return {rules:[{userAgent:'*',disallow:'/'}]};
 }
 return {rules:[{userAgent:'*',allow:'/',disallow:['/admin','/members','/profile','/gatherings','/auth','/reset-password','/api']}],sitemap:`${base}/sitemap.xml`};
}
