import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

const AUTH_PATHS = new Set(["/login", "/register"]);
const SESSION_COOKIE_NAME =
  process.env.SESSION_COOKIE_NAME?.trim() || "anby_session";

export function proxy(request: NextRequest) {
  if (
    AUTH_PATHS.has(request.nextUrl.pathname) ||
    request.cookies.has(SESSION_COOKIE_NAME)
  ) {
    return NextResponse.next();
  }

  const login = new URL("/login", request.url);
  login.searchParams.set(
    "next",
    `${request.nextUrl.pathname}${request.nextUrl.search}`,
  );
  return NextResponse.redirect(login);
}

export const config = {
  matcher: [
    "/((?!api|healthz|readyz|_next/static|_next/image|favicon.ico|.*\\..*).*)",
  ],
};
