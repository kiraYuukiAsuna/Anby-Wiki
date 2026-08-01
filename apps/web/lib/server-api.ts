import "server-only";

import { cookies } from "next/headers";

import type { ConfigurationParameters } from "../../../contracts/generated/typescript";

/**
 * Forward the browser session when a Server Component calls the Go API
 * directly through API_BASE_URL. Browser-side requests already use the
 * same-origin Next.js proxy and therefore do not need this adapter.
 */
export async function serverApiOptions(): Promise<ConfigurationParameters> {
  const cookieHeader = (await cookies()).toString();
  if (!cookieHeader) {
    return {};
  }
  return { headers: { Cookie: cookieHeader } };
}
