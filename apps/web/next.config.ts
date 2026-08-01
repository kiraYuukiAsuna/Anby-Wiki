import path from "node:path";
import type { NextConfig } from "next";

const isDevelopment = process.env.NODE_ENV === "development";
const contentSecurityPolicy = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline'${isDevelopment ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  `connect-src 'self'${isDevelopment ? " ws: wss:" : ""}`,
  "worker-src 'self' blob:",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

const securityHeaders = [
  { key: "Content-Security-Policy", value: contentSecurityPolicy },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
  },
];

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  allowedDevOrigins: isDevelopment ? ["127.0.0.1"] : undefined,
  turbopack: {
    // 生成客户端位于仓库根的 contracts/generated/typescript（app 根之外）。
    // Turbopack 默认不解析项目根以外的文件，需把 root 提升到仓库根（monorepo 布局）。
    root: path.join(__dirname, "../.."),
  },
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
  async rewrites() {
    // Next.js 会在构建期把外部 rewrite 写入 routes-manifest。生产镜像必须
    // 通过 Docker build arg 传入容器网络地址；运行时 API_BASE_URL 仍供 SSR
    // 生成客户端使用。两者由 compose.production.yml 保持一致。
    const apiBaseUrl = process.env.API_BASE_URL ?? "http://localhost:8080";
    return [
      {
        source: "/healthz",
        destination: `${apiBaseUrl}/healthz`,
      },
      {
        source: "/readyz",
        destination: `${apiBaseUrl}/readyz`,
      },
      {
        source: "/api/:path*",
        destination: `${apiBaseUrl}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
