import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { Toaster } from "sonner";

import { SessionShell } from "@/components/session-shell";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: {
    default: "Anby Wiki",
    template: "%s · Anby Wiki",
  },
  description:
    "一个由人工与 AI 共同维护、事实可验证、修改可审核的现代百科平台。",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="zh-CN"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full w-full min-w-0 flex-col overflow-x-clip">
        <SessionShell>{children}</SessionShell>
        <Toaster richColors position="top-center" />
      </body>
    </html>
  );
}
