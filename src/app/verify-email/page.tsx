import type { Metadata } from 'next';
import VerifyEmailScreen from '@/components/screens/VerifyEmailScreen';
export const metadata: Metadata = { title: 'Confirm your email', robots: { index: false, follow: false }, referrer: 'no-referrer' };
export default function Page() { return <VerifyEmailScreen />; }
