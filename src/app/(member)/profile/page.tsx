import type { Metadata } from 'next';
import Screen from '@/components/screens/ProfileScreen';
export const metadata:Metadata={title:'My profile',robots:{index:false,follow:false}};
export const dynamic='force-dynamic';
export default function Page(){return <Screen/>;}
