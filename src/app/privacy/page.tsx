import type { Metadata } from 'next';
import PrivacyScreen from '@/components/screens/PrivacyScreen';
export const metadata:Metadata={title:'Privacy',description:'How The Hive Society protects your personal information.'};
export default function Page(){return <PrivacyScreen/>;}
