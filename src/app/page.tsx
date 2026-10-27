import CampaignCapture from '@/components/hive/CampaignCapture';
import type { Metadata } from 'next';
import { headers } from 'next/headers';
import { publishedDoc } from '@/lib/cms-server';

const defaults = { title: 'The Hive Society', description: 'An Abu Dhabi community for all nationalities and genders, with mixed-gender and women-only curated gatherings.' };
export const dynamic = 'force-dynamic';

/** Search and share details come from the Studio's published SEO settings, if any. */
export async function generateMetadata(): Promise<Metadata> {
  const { seo } = await publishedDoc(new Request('http://hive.internal/', { headers: await headers() }));
  const title = seo.title || defaults.title, description = seo.description || defaults.description;
  return {
    title: { absolute: title }, description,
    ...(seo.canonical ? { alternates: { canonical: seo.canonical } } : {}),
    ...(seo.robots === 'noindex' ? { robots: { index: false, follow: false } } : {}),
    openGraph: { type: 'website', title, description, ...(seo.og_image ? { images: [seo.og_image] } : {}) },
  };
}

/** The homepage document is served by /site/home with the published Studio copy merged in. */
export default function HomePage(){return <><CampaignCapture/><iframe src="/site/home" title="The Hive Society" className="fixed inset-0 h-screen w-screen border-0" style={{height:'100dvh'}} /></>;}
