/**
 * Proxies an avatar URL through wsrv.nl to prevent IP leakage.
 * Returns undefined if the input is falsy so callers can fall back to placeholders.
 */
export function avatarUrl(
  raw: string | null | undefined,
  size = 256,
): string | undefined {
  if (!raw) return undefined;
  return `https://wsrv.nl/?url=${encodeURIComponent(raw)}&w=${size}&h=${size}&fit=cover&a=attention`;
}
