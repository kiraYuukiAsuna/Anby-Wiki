import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { Toaster } from "sonner";

import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { SiteNavigation } from "@/components/site-navigation";
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
        <a
          href="#main-content"
          className="fixed top-2 left-2 z-50 -translate-y-16 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-transform focus:translate-y-0"
        >
          跳到正文
        </a>
        <SiteHeader />
        <div className="flex min-h-0 w-full min-w-0 flex-1">
          <SiteNavigation />
          <div className="flex min-w-0 flex-1 flex-col">
            <main id="main-content" className="flex min-w-0 flex-1 flex-col">
              {children}
            </main>
            <SiteFooter />
          </div>
        </div>
        <Toaster richColors position="top-center" />
      </body>
    </html>
  );
}
