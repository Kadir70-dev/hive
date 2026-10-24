import type { MetadataRoute } from 'next';
export default function sitemap():MetadataRoute.Sitemap{
 const base=(process.env.NEXT_PUBLIC_SITE_URL || 'https://thehivesociety.ae').replace(/\/$/,'');
 return ['/','/privacy','/guidelines'].map(path=>({url:`${base}${path}`,changeFrequency:'monthly' as const,priority:path==='/'?1:0.4}));
}
