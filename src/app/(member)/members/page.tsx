import type { Metadata } from 'next';
import Screen from '@/components/screens/MembersScreen';
export const metadata:Metadata={title:'Members',robots:{index:false,follow:false}};
export const dynamic='force-dynamic';
export default function Page(){return <Screen/>;}
