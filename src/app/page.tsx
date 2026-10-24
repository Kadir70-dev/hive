import type { Metadata } from 'next';
export const metadata: Metadata = {title:'The Hive Society',description:'The Hive Society — a community gathering in a warm, welcoming space.'};
/** Preserve the supplied homepage verbatim. Existing API fetches resolve to Next route handlers. */
export default function HomePage(){return <iframe src="/hive.html" title="The Hive Society" className="fixed inset-0 h-screen w-screen border-0" style={{height:'100dvh'}} />;}
