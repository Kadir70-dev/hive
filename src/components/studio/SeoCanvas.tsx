'use client';
import type { CmsSEO } from '@/lib/cms-home';
import { Editable, ImageSlot } from './Editable';

export const SEO_DEFAULTS = {
  title: 'The Hive Society',
  description: 'An Abu Dhabi community for all nationalities and genders, with mixed-gender and women-only curated gatherings.',
};

type Props = { seo: CmsSEO; host: string; onSeo: (key: keyof CmsSEO, v: string) => void; onPickImage: () => void };

/** Google result preview, social share image and indexing switch for the homepage. */
export default function SeoCanvas({ seo, host, onSeo, onPickImage }: Props) {
  const robots = seo.robots || 'index';
  return <section className="sc-canvas sc-paper sc-seo" aria-label="SEO settings">
    <p className="sc-kicker">HOME · GOOGLE &amp; SOCIAL PREVIEW</p>
    <div className="sc-serp">
      <small>{host}{seo.canonical || '/'}</small>
      <Editable value={seo.title || SEO_DEFAULTS.title} label="Search result title" className="sc-serp-title" maxLength={140}
        onChange={(v) => onSeo('title', v === SEO_DEFAULTS.title ? '' : v.replace(/\n/g, ' '))} />
      <Editable value={seo.description || SEO_DEFAULTS.description} label="Search result description" className="sc-serp-desc" multiline maxLength={400}
        onChange={(v) => onSeo('description', v === SEO_DEFAULTS.description ? '' : v)} />
      <p className="sc-small">{(seo.title || SEO_DEFAULTS.title).length}/60 title · {(seo.description || SEO_DEFAULTS.description).length}/160 description characters recommended</p>
    </div>
    <div className="sc-seo-grid">
      <figure>
        <ImageSlot src={seo.og_image} label="Social share image" className="sc-image sc-og" onPick={onPickImage} />
        <figcaption>Share image for WhatsApp, Instagram and LinkedIn previews (1200×630 works best)</figcaption>
        {seo.og_image && <button type="button" className="sc-remove" onClick={() => onSeo('og_image', '')}>Remove image</button>}
      </figure>
      <div className="sc-seo-side">
        <span className="sc-small">Search engines</span>
        <div className="st-toggle" role="group" aria-label="Search engines">
          {(['index', 'noindex'] as const).map((r) => <button key={r} type="button" aria-pressed={robots === r} onClick={() => onSeo('robots', r)}>
            {r === 'index' ? 'Show in Google' : 'Hide from Google'}</button>)}
        </div>
        <label className="sc-alt"><small>Canonical path</small>
          <Editable value={seo.canonical || '/'} label="Canonical path" maxLength={200} onChange={(v) => onSeo('canonical', v.trim() === '/' ? '' : v.trim())} />
        </label>
        {seo.canonical && !/^\/(?!\/)[^\s"'()<>\\]*$/.test(seo.canonical) && <p className="st-error" role="alert">Use a path on this site that starts with /, for example /.</p>}
      </div>
    </div>
  </section>;
}
