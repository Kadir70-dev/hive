'use client';
import type { Lang, Pair } from '@/lib/cms-home';
import type { FieldDef, PairsDef, StudioSection } from '@/lib/studio-sections';
import { Editable, ImageSlot } from './Editable';

export type CanvasProps = {
  section: StudioSection; lang: Lang;
  text: (key: string) => string; pairs: (key: string) => Pair[];
  onText: (key: string, v: string) => void; onPairs: (key: string, v: Pair[]) => void;
  onPickImage: (key: string, altKey?: string) => void;
  /** Keys the homepage renders with <em> emphasis. */
  isHtml: (key: string) => boolean;
};

const CLASS: Record<FieldDef['role'], string> = {
  eyebrow: 'sc-eyebrow', heading: 'sc-heading', lead: 'sc-lead', text: 'sc-text', quote: 'sc-quote',
  button: 'sc-btn', small: 'sc-small', tag: 'sc-tag', name: 'sc-name', image: '',
};
const MULTILINE = new Set<FieldDef['role']>(['text', 'quote', 'lead']);

function Field({ def, p, hero }: { def: FieldDef; p: CanvasProps; hero?: boolean }) {
  const value = p.text(def.key);
  if (def.role === 'image') {
    const alt = def.alt ? p.text(def.alt) : '';
    return <div className="sc-figure">
      <ImageSlot src={value} label={def.label} alt={alt} className="sc-image" onPick={() => p.onPickImage(def.key, def.alt)} />
      {def.alt && <label className="sc-alt"><small>Image description (alt text)</small>
        <Editable value={alt} label={`${def.label} description`} onChange={(v) => p.onText(def.alt!, v)} maxLength={300} /></label>}
    </div>;
  }
  const cls = CLASS[def.role] + (hero && def.role === 'heading' ? ' sc-hero-title' : '');
  return <Editable value={value} label={def.label} className={cls} multiline={MULTILINE.has(def.role)} html={p.isHtml(def.key)} onChange={(v) => p.onText(def.key, v)} />;
}

function Fields({ defs, p, hero }: { defs: FieldDef[]; p: CanvasProps; hero?: boolean }) {
  const buttons = defs.filter((d) => d.role === 'button'), tags = defs.filter((d) => d.role === 'tag');
  const rest = defs.filter((d) => d.role !== 'button' && d.role !== 'tag');
  return <>
    {rest.map((d) => <Field key={d.key} def={d} p={p} hero={hero} />)}
    {tags.length > 0 && <div className="sc-tags">{tags.map((d) => <Field key={d.key} def={d} p={p} />)}</div>}
    {buttons.length > 0 && <div className="sc-buttons">{buttons.map((d, i) => <span key={d.key} className={i ? 'sc-btn-outline' : ''}><Field def={d} p={p} /></span>)}</div>}
  </>;
}

const MAX_ITEMS = 20;
function PairList({ def, p }: { def: PairsDef; p: CanvasProps }) {
  const items = p.pairs(def.key);
  const set = (i: number, j: 0 | 1, v: string) => p.onPairs(def.key, items.map((it, k) => (k === i ? (j ? [it[0], v] : [v, it[1]]) as Pair : it)));
  return <div className="sc-pairs" aria-label={def.label}>
    {items.map((it, i) => <article key={i} className="sc-card sc-pair">
      <span className="sc-num">{String(i + 1).padStart(2, '0')}</span>
      <Editable value={it[0]} label={`${def.item[0]} ${i + 1}`} className="sc-name" maxLength={1500} onChange={(v) => set(i, 0, v)} />
      <Editable value={it[1]} label={`${def.item[1]} ${i + 1}`} className="sc-text" multiline maxLength={1500} onChange={(v) => set(i, 1, v)} />
      <button type="button" className="sc-remove" aria-label={`Remove ${def.item[0].toLowerCase()} ${i + 1}`}
        onClick={() => { if (window.confirm(`Remove "${it[0] || def.item[0]}"?`)) p.onPairs(def.key, items.filter((_, k) => k !== i)); }}>Remove</button>
    </article>)}
    {items.length < MAX_ITEMS && <button type="button" className="sc-add" onClick={() => p.onPairs(def.key, [...items, ['', '']])}>+ Add {def.item[0].toLowerCase()}</button>}
  </div>;
}

/** One homepage section, drawn to look like the live website and editable in place. */
export default function SectionCanvas(p: CanvasProps) {
  const { section, lang } = p;
  return <section className={`sc-canvas sc-${section.tone}`} dir={lang === 'ar' ? 'rtl' : 'ltr'} lang={lang} aria-label={`${section.name} preview`}>
    <p className="sc-kicker" dir="ltr">HOME · {section.name.toUpperCase()} · {lang === 'ar' ? 'العربية' : 'ENGLISH'}</p>
    <div className={section.id === 'hero' ? 'sc-hero' : 'sc-head'}><Fields defs={section.fields} p={p} hero={section.id === 'hero'} /></div>
    {section.pairs && <PairList def={section.pairs} p={p} />}
    {section.cards && <div className={`sc-cards sc-cards-${section.cards.length}`}>
      {section.cards.map((card, i) => <article key={i} className="sc-card"><Fields defs={card} p={p} /></article>)}
    </div>}
    {section.after && <div className="sc-after"><Fields defs={section.after} p={p} /></div>}
  </section>;
}
