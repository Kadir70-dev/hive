'use client';
import {useQuery} from '@tanstack/react-query';
import {api} from '@/lib/hive-api';
import {HiveShell} from '@/components/hive/HiveShell';
type Entry={id:string;title?:string;body?:string;name?:string;description?:string;logo_url?:string;website_url?:string};
export default function CommunityListScreen({kind}:{kind:'announcements'|'partners'}){const {data=[],error}=useQuery({queryKey:[kind],queryFn:()=>api<Entry[]>(`/${kind}`)});return <HiveShell member><div className="hv-hero"><h1>{kind==='partners'?'Community partners':'Announcements'}</h1></div><div className="hv-stack">{error&&<p role="alert">Unable to load content.</p>}{data.map(x=><article className="hv-card" key={x.id}><h3>{x.title||x.name}</h3><p>{x.body||x.description}</p>{x.website_url&&<a href={x.website_url} target="_blank" rel="noopener noreferrer">Partner website</a>}</article>)}</div></HiveShell>}
