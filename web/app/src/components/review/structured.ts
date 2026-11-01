// Validate JSON grammar while preserving every original token (including duplicate keys).
export function formatJSON(source: string): string {
  // Parsing is used only for syntax validation; parsed values never reach the view.
  JSON.parse(source);
  const tokens = source.match(/"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\],:]/g) || [];
  let depth = 0, result = '';
  const indent = () => '  '.repeat(depth);
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (t === '{' || t === '[') {
      if (++depth > 100) throw new Error('JSON nesting exceeds the formatted preview limit; use Source.');
      result += t + '\n' + indent();
    } else if (t === '}' || t === ']') { depth--; result += '\n' + indent() + t; }
    else if (t === ',') result += ',\n' + indent();
    else if (t === ':') result += ': ';
    else result += t;
    if (result.length > 2 * 1024 * 1024) throw new Error('Formatted output limit exceeded; use Source.');
  }
  return result;
}
export function jsonLineErrors(source: string): string[] {
  const errors: string[] = [];
  source.split('\n').forEach((line, i) => {
    if (!line.trim()) return;
    try { JSON.parse(line); } catch { errors.push(`Line ${i + 1}: invalid JSON`); }
  });
  return errors;
}
