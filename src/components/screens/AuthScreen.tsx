'use client';
import { useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import { send } from '@/lib/hive-api';
import { HiveShell } from '@/components/hive/HiveShell';
export default function AuthScreen(){
 const router=useRouter(),qc=useQueryClient(),params=useSearchParams();
 // /auth?mode=signup (the homepage Join buttons) opens the sign-up form.
 const[signup,setSignup]=useState(()=>params.get('mode')==='signup'),[busy,setBusy]=useState(false),[error,setError]=useState('');
 async function submit(e:FormEvent<HTMLFormElement>){e.preventDefault();setBusy(true);setError('');const f=new FormData(e.currentTarget);
 try{ const email=String(f.get('email')||'').trim(),password=String(f.get('password')||'');
 if(signup){await send('/signup',{email,password,firstName:String(f.get('first_name')||''),displayName:String(f.get('display_name')||''),phone:String(f.get('phone')||''),interests:[],area:''})}
 else {await send('/login',{email,password})}
 await qc.invalidateQueries({queryKey:['me']});router.replace('/members');
 }catch(err){setError(err instanceof Error?err.message:'Unable to continue')}finally{setBusy(false)}}
 return <HiveShell><div className="hv-hero"><h1>{signup?'Join The Hive Society':'Welcome back'}</h1><p>Private community membership. New accounts are reviewed before approval.</p></div><div className="hv-card" style={{maxWidth:540,marginInline:'auto'}}><div className="hv-actions"><button className="hv-btn ghost" type="button" onClick={()=>setSignup(false)}>Log in</button><button className="hv-btn ghost" type="button" onClick={()=>setSignup(true)}>Sign up</button></div><form className="hv-stack" onSubmit={submit}>{signup&&<><label>First name *<input name="first_name" required maxLength={60}/></label><label>Display name (optional)<input name="display_name" maxLength={60}/></label><label>Phone (optional)<input name="phone" maxLength={30}/></label></>}<label>Email *<input name="email" required type="email" autoComplete="email"/></label><label>Password *<input name="password" required type="password" minLength={signup?12:1} autoComplete={signup?'new-password':'current-password'}/></label>{signup&&<label><input type="checkbox" required/> I agree to the <Link href="/terms">Terms</Link> and <Link href="/privacy">Privacy Policy</Link>.</label>}{error&&<p role="alert" className="hv-msg err">{error}</p>}<button className="hv-btn" disabled={busy}>{busy?'Please wait…':signup?'Create account':'Log in'}</button></form></div></HiveShell>
}
