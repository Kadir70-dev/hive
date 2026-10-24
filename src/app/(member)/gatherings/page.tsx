import type { Metadata } from 'next';
import Screen from '@/components/screens/GatheringsScreen';
export const metadata:Metadata={title:'Gatherings',robots:{index:false,follow:false}};
export const dynamic='force-dynamic';
export default function Page(){return <Screen/>;}
