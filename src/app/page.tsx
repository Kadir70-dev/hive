import type { Metadata } from 'next';
export const metadata: Metadata = {title:'The Hive Society',description:'An Abu Dhabi community for all nationalities and genders, with mixed-gender and women-only curated gatherings.'};
/** Preserve the supplied homepage verbatim. Existing API fetches resolve to Next route handlers. */
export default function HomePage(){return <iframe src="/hive.html" title="The Hive Society" className="fixed inset-0 h-screen w-screen border-0" style={{height:'100dvh'}} />;}
