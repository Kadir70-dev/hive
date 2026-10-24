import { z } from 'zod';

export const interestSchema = z.object({
  interests: z.string().trim().min(1).max(600),
  lang: z.enum(['en', 'ar']).default('en'),
}).strict();

export const gatheringOptions = [
  { name: 'Wellness', description: 'Movement, breath and slow mornings.' },
  { name: 'Majlis', description: 'Emirati hospitality, qahwa, dates and meaningful conversation.' },
  { name: 'Culture', description: 'Galleries, heritage and crafts, including calligraphy and palm weaving; creativity such as pottery, writing and photography.' },
  { name: 'Wander', description: 'Unhurried walks through the city, coast and desert.' },
  { name: 'Social', description: 'Warm evenings where strangers become friends.' },
] as const;

export type RecommendationEvent =
  | { type: 'text' | 'reasoning'; text: string }
  | { type: 'done' | 'stopped' }
  | { type: 'error'; message: string; status: number };