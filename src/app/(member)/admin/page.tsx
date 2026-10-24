import type { Metadata } from 'next';
import Screen from '@/components/screens/AdminScreen';
export const metadata:Metadata={title:'Admin',robots:{index:false,follow:false}};
export const dynamic='force-dynamic';
export default function Page(){return <Screen/>;}
