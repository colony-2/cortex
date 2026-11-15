export function searchTerms(query: string): string[] {
  return [...new Set(query.toLocaleLowerCase().split(/\s+/).filter(Boolean))];
}

/** Every term must occur in this field; callers keep candidate fields independent. */
export function matchesSearchTerms(terms: string[], text?: string): boolean {
  if (!terms.length) return true;
  if (!text) return false;
  const candidate = text.toLocaleLowerCase();
  return terms.every(term => candidate.includes(term));
}
