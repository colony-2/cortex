export function cellLabel(name?: string) {
  return name?.trim() ? name : 'Unknown cell';
}

// Missing and blank metadata use the same option as the displayed Unknown cell.
export function cellFilterValue(name?: string) {
  return name?.trim() ? name : '';
}
