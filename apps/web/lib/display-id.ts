/**
 * Compact an opaque identifier without relying on its shared leading prefix.
 *
 * UUIDv7 values created close together often have identical first bytes, so a
 * prefix-only label can make different records look duplicated. Keeping the
 * random tail makes compact labels distinguishable while the full value stays
 * available through links or title attributes.
 */
export function compactId(id: string): string {
  if (id.length <= 16) return id;
  return `${id.slice(0, 4)}…${id.slice(-8)}`;
}
