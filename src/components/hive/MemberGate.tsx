'use client';
import { useEffect, useState, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/hive-api';
export function MemberGate({ children }: { children: ReactNode }) {
 const router = useRouter();const [ready,setReady]=useState(false);
 useEffect(()=>{let active=true;void api('/me').then(()=>{if(active)setReady(true)}).catch(()=>{if(active)router.replace('/auth')});return()=>{active=false}},[router]);
 return ready?<>{children}</>:<div className="hv hv-wrap" role="status">Checking your account…</div>;
}
