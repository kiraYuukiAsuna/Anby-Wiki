export const WIKI_CONTEXT_COOKIE_NAME = "anby_wiki_id";
export const WIKI_CONTEXT_HEADER = "X-Wiki-ID";

const cookieMaxAgeSeconds = 60 * 60 * 24 * 365;

export function selectedWikiIDFromDocument(): string | undefined {
  if (typeof document === "undefined") return undefined;
  const prefix = `${WIKI_CONTEXT_COOKIE_NAME}=`;
  const found = document.cookie
    .split(";")
    .map((item) => item.trim())
    .find((item) => item.startsWith(prefix));
  if (!found) return undefined;
  const value = decodeURIComponent(found.slice(prefix.length)).trim();
  return value || undefined;
}

export function persistSelectedWikiID(wikiID: string) {
  document.cookie = `${WIKI_CONTEXT_COOKIE_NAME}=${encodeURIComponent(
    wikiID,
  )}; Path=/; Max-Age=${cookieMaxAgeSeconds}; SameSite=Lax`;
}

export function clearSelectedWikiID() {
  document.cookie = `${WIKI_CONTEXT_COOKIE_NAME}=; Path=/; Max-Age=0; SameSite=Lax`;
}
