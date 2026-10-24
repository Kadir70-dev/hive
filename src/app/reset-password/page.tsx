import type { Metadata } from 'next';
import ResetScreen from '@/components/screens/ResetScreen';
export const metadata:Metadata={title:'Reset password',robots:{index:false,follow:false}};
export default function Page(){return <ResetScreen/>;}
