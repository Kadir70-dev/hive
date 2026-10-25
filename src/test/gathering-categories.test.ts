import { describe, expect, it } from 'vitest';
import { gatheringCategory } from '@/components/screens/GatheringsScreen';

describe('gathering categories', () => {
  it('recognises the two current gathering types', () => {
    expect(gatheringCategory('mixed')).toBe('mixed');
    expect(gatheringCategory('mixed-gender')).toBe('mixed');
    expect(gatheringCategory('women-only')).toBe('women-only');
    expect(gatheringCategory('women only')).toBe('women-only');
  });

  it('does not silently relabel old activity themes as mixed-gender', () => {
    expect(gatheringCategory('majlis')).toBe('other');
    expect(gatheringCategory('community')).toBe('other');
  });
});
