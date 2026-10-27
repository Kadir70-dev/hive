/**
 * Website Studio registry: the homepage split into the sections an editor
 * recognises, top to bottom, and how each copy key is shown on the canvas.
 * Keys are the homepage's own copy keys (data-t="…" in public/hive.html).
 */
export type Role = 'eyebrow' | 'heading' | 'lead' | 'text' | 'quote' | 'button' | 'small' | 'tag' | 'name' | 'image';
export type FieldDef = { key: string; label: string; role: Role; alt?: string };
export type PairsDef = { key: string; label: string; item: [string, string] };
export type StudioSection = {
  id: string; name: string; tone: 'sand' | 'paper' | 'deep';
  fields: FieldDef[]; cards?: FieldDef[][]; pairs?: PairsDef; after?: FieldDef[]; seo?: boolean;
};
export type StudioPage = { slug: string; name: string; path: string; sections: StudioSection[] };

const f = (key: string, label: string, role: Role, alt?: string): FieldDef => ({ key, label, role, alt });

const HOME: StudioSection[] = [
  { id: 'menu', name: 'Header & Menu', tone: 'paper', fields: [
    f('navWhy', 'Menu link: why we exist', 'tag'), f('navWay', 'Menu link: The Hive Way', 'tag'), f('navGather', 'Menu link: gatherings', 'tag'),
    f('navJoin', 'Join button', 'button'), f('navLogin', 'Log in link', 'tag'), f('navProfile', 'My profile link (members)', 'tag')] },
  { id: 'hero', name: 'Hero', tone: 'sand', fields: [
    f('heroTitle', 'Hero title', 'heading'), f('heroLine', 'Tagline', 'lead'), f('heroIntro', 'Introduction', 'text'),
    f('ctaJoin', 'Main button', 'button'), f('ctaWay', 'Second button', 'button')] },
  { id: 'why', name: 'Why we exist', tone: 'paper', fields: [f('whyEyebrow', 'Small heading', 'eyebrow'), f('whyTitle', 'Title', 'heading'), f('whyText', 'Text', 'text')] },
  { id: 'about', name: 'About', tone: 'paper', fields: [
    f('aboutEyebrow', 'Small heading', 'eyebrow'), f('aboutTitle', 'Title', 'heading'), f('aboutLead', 'Opening line', 'lead'), f('aboutText', 'Text', 'text')],
    cards: [1, 2, 3].map((n) => [f(`p${n}t`, `Pillar ${n} title`, 'name'), f(`p${n}`, `Pillar ${n} text`, 'text')]) },
  { id: 'who', name: "Who it's for", tone: 'sand', fields: [
    f('quote', 'Quote', 'quote'), f('whoEyebrow', 'Small heading', 'eyebrow'), f('whoTitle', 'Title', 'heading'), f('whoText', 'Text', 'text')],
    after: [1, 2, 3, 4, 5].map((n) => f(`t${n}`, `Tag ${n}`, 'tag')) },
  { id: 'founder', name: 'Founder', tone: 'paper', fields: [
    f('fdEyebrow', 'Small heading', 'eyebrow'), f('fdQuote', 'Founder quote', 'quote'), f('fdBody', 'Founder note', 'text'), f('fdName', 'Name', 'name'), f('fdRole', 'Role', 'small')] },
  { id: 'gatherings', name: 'Gatherings', tone: 'paper', fields: [f('gEyebrow', 'Small heading', 'eyebrow'), f('gTitle', 'Title', 'heading'), f('gText', 'Text', 'text')],
    pairs: { key: 'gcats', label: 'Gathering categories', item: ['Category', 'Description'] },
    cards: [['mixed', 'Mixed-gender'], ['women', 'Women-only']].map(([k, n]) => [f(`${k}Kicker`, `${n} label`, 'eyebrow'), f(`${k}Title`, `${n} title`, 'name'), f(`${k}Desc`, `${n} text`, 'text')]),
    after: [f('gatheringFreeNote', 'Note about free places', 'small'), f('gNote', 'Season note', 'small')] },
  { id: 'vision', name: 'Vision', tone: 'sand', fields: [f('visionEyebrow', 'Small heading', 'eyebrow'), f('visionTitle', 'Title', 'heading'), f('visionLead', 'Text', 'text')],
    cards: [1, 2, 3].map((n) => [f(`visionImg${n}`, `Card ${n} image`, 'image', `visionAlt${n}`), f(`visionCard${n}`, `Card ${n} caption`, 'name')]) },
  { id: 'evening', name: 'Evening flow', tone: 'paper', fields: [f('flEyebrow', 'Small heading', 'eyebrow'), f('flTitle', 'Title', 'heading'), f('flText', 'Text', 'text')],
    pairs: { key: 'flow', label: 'Steps of the evening', item: ['Step', 'Description'] } },
  { id: 'way', name: 'The Hive Way', tone: 'deep', fields: [
    f('wayEyebrow', 'Small heading', 'eyebrow'), f('wayTitle2', 'Title', 'heading'), f('wayText', 'Text', 'text'), f('honest', 'Promise line', 'small')],
    pairs: { key: 'practices', label: 'Practices', item: ['Practice', 'Description'] },
    after: [f('zayed', 'Ghaf quote', 'quote'), f('zayedBy', 'Quote credit', 'small')] },
  { id: 'impact', name: 'Impact', tone: 'paper', fields: [f('impactEyebrow', 'Small heading', 'eyebrow'), f('growthTitle', 'Title', 'heading'), f('growthIntro', 'Text', 'text')],
    cards: ['Friendships', 'Land', 'Women'].map((k) => [f(`growth${k}Title`, `${k} title`, 'name'), f(`growth${k}Text`, `${k} text`, 'text')]),
    after: [f('growthNote', 'Note', 'small')] },
  { id: 'faq', name: 'FAQ', tone: 'paper', fields: [f('fqEyebrow', 'Small heading', 'eyebrow'), f('fqTitle', 'Title', 'heading'), f('fqText', 'Text', 'text')],
    pairs: { key: 'faq', label: 'Questions', item: ['Question', 'Answer'] } },
  { id: 'partners', name: 'Partners', tone: 'sand', fields: [
    f('partEyebrow', 'Small heading', 'eyebrow'), f('partTitle', 'Title', 'heading'), f('partText', 'Text', 'text'), f('partContact', 'Contact label', 'small'), f('copy', 'Copy button', 'button')] },
  { id: 'footer', name: 'Footer', tone: 'deep', fields: [f('footLine', 'Footer tagline', 'lead'), f('footMotto', 'Motto', 'small')],
    cards: [
      [f('fExplore', 'Explore heading', 'name'), f('fPrivacy', 'Privacy link', 'small')],
      [f('fGuide', 'Guidelines heading', 'name'), f('g1', 'Guideline 1', 'small'), f('g2', 'Guideline 2', 'small'), f('g3', 'Guideline 3', 'small'), f('fGuideLink', 'Guidelines link', 'small')],
      [f('fContact', 'Contact heading', 'name'), f('footPlace', 'Location', 'small')]] },
  { id: 'seo', name: 'SEO', tone: 'paper', fields: [], seo: true },
];

export const STUDIO_PAGES: StudioPage[] = [{ slug: 'home', name: 'Home', path: '/', sections: HOME }];

/** Every copy key the Studio can edit (used to check the registry against the homepage). */
export const studioKeys = (page: StudioPage) => page.sections.flatMap((s) => [
  ...s.fields, ...(s.cards ?? []).flat(), ...(s.after ?? []),
].flatMap((x) => (x.alt ? [x.key, x.alt] : [x.key])).concat(s.pairs ? [s.pairs.key] : []));
