import type { ReactNode } from 'react';
import { MemberGate } from '@/components/hive/MemberGate';
export default function MemberLayout({children}:{children:ReactNode}){return <MemberGate>{children}</MemberGate>;}
