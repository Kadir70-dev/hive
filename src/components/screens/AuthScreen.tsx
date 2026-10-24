'use client';
import { useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, send } from '@/lib/hive-api';
import { HiveShell } from '@/components/hive/HiveShell';
export default function AuthScreen(){
 const router=useRouter(),qc=useQueryClient(),params=useSearchParams();
 // /auth?mode=signup (the homepage Join buttons) opens the sign-up form.
 const[signup,setSignup]=useState(()=>params.get('mode')==='signup'),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const[sentTo,setSentTo]=useState(''),[unverified,setUnverified]=useState(''),[notice,setNotice]=useState('');
 async function submit(e:FormEvent<HTMLFormElement>){e.preventDefault();setBusy(true);setError('');setNotice('');setUnverified('');const f=new FormData(e.currentTarget);
 const email=String(f.get('email')||'').trim(),password=String(f.get('password')||'');
 try{
  if(signup){
   await send('/signup',{email,password,firstName:String(f.get('first_name')||''),displayName:String(f.get('display_name')||''),phone:String(f.get('phone')||''),interests:[],area:''});
   setSentTo(email);return;
  }
  const res=await send<{mfa_required?:boolean}>('/login',{email,password});
  await qc.invalidateQueries({queryKey:['me']});router.replace(res.mfa_required?'/admin':'/members');
 }catch(err){
  if(err instanceof ApiError&&err.code==='email_unverified'){setUnverified(email);setError(err.message)}
  else setError(err instanceof Error?err.message:'Unable to continue');
 }finally{setBusy(false)}}
 async function resend(){setBusy(true);try{await send('/verify-email/resend',{email:unverified});setNotice('If that address needs confirming, a new link is on its way.')}catch(err){setError(err instanceof Error?err.message:'Unable to send')}finally{setBusy(false)}}
 if(sentTo)return <HiveShell><div className="hv-hero"><h1>Check your email</h1><p>We sent a confirmation link to <strong>{sentTo}</strong>. Open it to confirm your address, then log in. The link expires in 24 hours.</p></div><div className="hv-card" style={{maxWidth:540,marginInline:'auto'}}><p>Nothing arrived? Check your spam folder, or <button type="button" className="hv-link" onClick={()=>{setUnverified(sentTo);setSentTo('');setSignup(false)}}>request a new link</button>.</p></div></HiveShell>;
 return <HiveShell><div className="hv-hero"><h1>{signup?'Join The Hive Society':'Welcome back'}</h1><p>Private community membership. New accounts are reviewed before approval.</p></div><div className="hv-card" style={{maxWidth:540,marginInline:'auto'}}><div className="hv-actions"><button className="hv-btn ghost" type="button" onClick={()=>setSignup(false)}>Log in</button><button className="hv-btn ghost" type="button" onClick={()=>setSignup(true)}>Sign up</button></div><form className="hv-stack" onSubmit={submit}>{signup&&<><label>First name *<input name="first_name" required maxLength={60}/></label><label>Display name (optional)<input name="display_name" maxLength={60}/></label><label>Phone (optional)<input name="phone" maxLength={30}/></label></>}<label>Email *<input name="email" required type="email" autoComplete="email" defaultValue={unverified}/></label><label>Password *<input name="password" required type="password" minLength={signup?12:1} autoComplete={signup?'new-password':'current-password'}/></label>{signup&&<label><input type="checkbox" required/> I agree to the <Link href="/terms">Terms</Link> and <Link href="/privacy">Privacy Policy</Link>.</label>}{error&&<p role="alert" className="hv-msg err">{error}</p>}{notice&&<p role="status" className="hv-msg">{notice}</p>}{unverified&&<button type="button" className="hv-btn ghost" disabled={busy} onClick={()=>void resend()}>Send a new confirmation link</button>}<button className="hv-btn" disabled={busy}>{busy?'Please wait…':signup?'Create account':'Log in'}</button>{!signup&&<p><Link href="/reset-password">Forgot your password?</Link></p>}</form></div></HiveShell>
}
