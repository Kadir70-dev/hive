'use client';
import { useEffect } from 'react';
import { reportLovableError } from '@/lib/lovable-error-reporting';
export default function ErrorPage({error,reset}:{error:Error & {digest?:string};reset:()=>void}){
  useEffect(()=>{reportLovableError(error,{boundary:'next_app_error'});},[error]);
  return <main className="min-h-screen grid place-content-center p-8 text-center"><h1>We couldn't load this page</h1><p>Please try again.</p><button className="hv-btn" onClick={reset}>Try again</button><a href="/" className="hv-btn ghost">Go home</a></main>;
}
