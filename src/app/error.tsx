'use client';
import { useEffect } from 'react';
import Link from 'next/link';
export default function ErrorPage({error,reset}:{error:Error & {digest?:string};reset:()=>void}){
  useEffect(()=>{console.error('Page error', error.digest ?? '');},[error]);
  return <main className="min-h-screen grid place-content-center p-8 text-center"><h1>We couldn&apos;t load this page</h1><p>Please try again.</p><button className="hv-btn" onClick={reset}>Try again</button><Link href="/" className="hv-btn ghost">Go home</Link></main>;
}
