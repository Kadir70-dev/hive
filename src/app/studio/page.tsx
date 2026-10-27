import type { Metadata } from 'next';
import StudioApp from '@/components/studio/StudioApp';

export const metadata: Metadata = { title: 'Studio', robots: { index: false, follow: false } };
export const dynamic = 'force-dynamic';

/** Website Studio: admins edit the homepage copy, images and SEO (draft → publish). */
export default function Page() { return <StudioApp />; }
