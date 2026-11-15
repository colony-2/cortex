import { expect, it } from 'vitest';
import { matchesSearchTerms, searchTerms } from './searchTerms';

it.each(['foo bar', ' BAR\tFoO\n foo ', 'bar foo'])('matches every partial term: %s', query => {
  expect(matchesSearchTerms(searchTerms(query), 'food that i had at the bar')).toBe(true);
});
it.each(['foo baz', 'foo bar missing', '"foo bar"', 'f.o bar', 'foo.*'])('requires literal terms: %s', query => {
  expect(matchesSearchTerms(searchTerms(query), 'food that i had at the bar')).toBe(false);
});
it('handles blank and missing metadata without inventing a display fallback', () => {
  expect(matchesSearchTerms(searchTerms(' \t\n'), undefined)).toBe(true);
  expect(matchesSearchTerms(searchTerms('review'), undefined)).toBe(false);
  expect(matchesSearchTerms(searchTerms('a+b'), 'literal a+b')).toBe(true);
});
