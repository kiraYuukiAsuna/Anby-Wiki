"use client";

import useSWR from "swr";
import {
  ResponseError,
  type AuthSession,
} from "../../../contracts/generated/typescript";

import { authApi } from "@/lib/api";

export const AUTH_SESSION_KEY = "auth:session";
export const LOGIN_PATH = "/api/v1/auth/login";

export function isUnauthorized(error: unknown): error is ResponseError {
  return error instanceof ResponseError && error.response.status === 401;
}

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
      revalidateOnFocus: true,
    },
  );
  return {
    session: data,
    isLoading,
    isAuthenticated: Boolean(data),
    error: isUnauthorized(error) ? undefined : error,
    mutate: () => mutate(),
  };
}
