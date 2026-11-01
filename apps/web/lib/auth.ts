"use client";

import useSWR, { SWRConfig, type SWRConfiguration } from "swr";
import {
  ResponseError,
  type AuthSession,
} from "../../../contracts/generated/typescript";

import { authApi, retryAfterSeconds } from "@/lib/api";

export const AUTH_SESSION_KEY = "auth:session";
/** 站内本地账号登录页。 */
export const LOGIN_PATH = "/login";

export function isUnauthorized(error: unknown): error is ResponseError {
  return error instanceof ResponseError && error.response.status === 401;
}

const sessionErrorRetry: NonNullable<SWRConfiguration["onErrorRetry"]> = (
  error, key, config, revalidate, options,
) => {
  if (isUnauthorized(error)) return;
  if (error instanceof ResponseError && error.response.status === 429) {
    setTimeout(() => revalidate(options), retryAfterSeconds(error.response) * 1000);
    return;
  }
  SWRConfig.defaultValue.onErrorRetry(error, key, config, revalidate, options);
};

export function useSession(): {
  session: AuthSession | undefined;
  isLoading: boolean;
  isAuthenticated: boolean;
  error: unknown;
  mutate: () => Promise<AuthSession | undefined>;
} {
  const { data, error, isLoading, mutate } = useSWR(
    AUTH_SESSION_KEY,
    () => authApi().getSession(),
    {
      shouldRetryOnError: (candidate) => !isUnauthorized(candidate),
      onErrorRetry: sessionErrorRetry,
      revalidateOnFocus: true,
    },
  );
  // A transient failure keeps the last verified session; a real 401 clears it.
  const session = isUnauthorized(error) ? undefined : data;
  return {
    session,
    isLoading,
    isAuthenticated: Boolean(session),
    error: isUnauthorized(error) ? undefined : error,
    mutate: () => mutate(),
  };
}
