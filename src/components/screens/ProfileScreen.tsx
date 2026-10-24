'use client';
import {useState,type FormEvent} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {HiveShell,useMe,type Me} from '@/components/hive/HiveShell';
import {api,send} from '@/lib/hive-api';

/** Form state is seeded from `me` once; remounting (key) re-seeds after a reload of `me`. */
function ProfileForm({me}:{me:Me}){
  const qc=useQueryClient(),router=useRouter();
  const [msg,setMsg]=useState('');
  const [name,setName]=useState(me.first_name),[display,setDisplay]=useState(me.display_name||''),[phone,setPhone]=useState(me.phone||''),[area,setArea]=useState(me.area||''),[interests,setInterests]=useState((me.interests||[]).join(', ')),[updates,setUpdates]=useState(!!me.email_updates);
  async function save(e:FormEvent){e.preventDefault();try{await send('/me',{firstName:name,displayName:display,phone,area,interests:interests.split(',').map(s=>s.trim()).filter(Boolean),emailUpdates:updates},'PATCH');await qc.invalidateQueries({queryKey:['me']});setMsg('Saved')}catch(e){setMsg(e instanceof Error?e.message:'Failed')}}
  async function del(){if(!confirm('Permanently delete your account and personal data?'))return;try{await api('/me',{method:'DELETE'});qc.clear();router.replace('/auth')}catch{setMsg('Unable to delete account')}}
  return <div className="hv-card"><p>Membership status: <strong>{me.status}</strong></p><p>Email (private): {me.email}</p><form className="hv-stack" onSubmit={e=>void save(e)}><label>First name<input required value={name} onChange={e=>setName(e.target.value)}/></label><label>Display name<input value={display} onChange={e=>setDisplay(e.target.value)}/></label><label>Phone (private)<input value={phone} onChange={e=>setPhone(e.target.value)}/></label><label>Area<input value={area} onChange={e=>setArea(e.target.value)}/></label><label>Interests (comma separated)<input value={interests} onChange={e=>setInterests(e.target.value)}/></label><label><input type="checkbox" checked={updates} onChange={e=>setUpdates(e.target.checked)}/> Receive optional email updates</label><button className="hv-btn">Save profile</button></form><p role="status">{msg}</p><hr/><button className="hv-btn ghost" onClick={()=>void del()}>Delete my account</button></div>;
}

export default function ProfileScreen(){const{data:me}=useMe();return <HiveShell member><div className="hv-hero"><h1>My profile</h1><p>Your personal details are visible to admins only.</p></div>{me&&<ProfileForm key={me.id} me={me}/>}</HiveShell>}
