import "server-only";

import { cookies } from "next/headers";

import type { ConfigurationParameters } from "../../../contracts/generated/typescript";
import {
  WIKI_CONTEXT_COOKIE_NAME,
  WIKI_CONTEXT_HEADER,
} from "@/lib/wiki-context";

/**
 * Forward the browser session when a Server Component calls the Go API
 * directly through API_BASE_URL. Browser-side requests already use the
 * same-origin Next.js proxy and therefore do not need this adapter.
 */
export async function serverApiOptions(): Promise<ConfigurationParameters> {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const wikiID = cookieStore.get(WIKI_CONTEXT_COOKIE_NAME)?.value.trim();
  const headers: Record<string, string> = {};
  if (cookieHeader) {
    headers.Cookie = cookieHeader;
  }
  if (wikiID) {
    headers[WIKI_CONTEXT_HEADER] = wikiID;
  }
  if (Object.keys(headers).length === 0) {
    return {};
  }
  return { headers };
}
