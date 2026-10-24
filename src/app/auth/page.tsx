import type { Metadata } from 'next';
import { Suspense } from 'react';
import AuthScreen from '@/components/screens/AuthScreen';
export const metadata:Metadata={title:"Members' sign in",robots:{index:false,follow:false}};
export default function Page(){return <Suspense fallback={<div role="status">Loading sign in…</div>}><AuthScreen/></Suspense>}
