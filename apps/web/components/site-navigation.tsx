"use client";

import Link from "next/link";
import {
  Archive,
  BadgeCheck,
  BookOpenText,
  Bot,
  BrainCircuit,
  Boxes,
  House,
  Images,
  LibraryBig,
  Globe2,
  Layers3,
  LockKeyhole,
  ShieldCheck,
  ScrollText,
  ShieldAlert,
  SquarePen,
  Search,
  TerminalSquare,
  UserRound,
  UsersRound,
  Waypoints,
} from "lucide-react";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";

const GROUPS = [
  {
    label: "知识",
    items: [
      { href: "/", label: "首页", icon: House, exact: true },
      { href: "/pages", label: "页面目录", icon: BookOpenText, exact: true },
      { href: "/collections", label: "专题合集", icon: Layers3 },
      { href: "/explore", label: "搜索", icon: Search },
      { href: "/components", label: "组件中心", icon: Boxes },
      { href: "/assets", label: "媒体与附件", icon: Images },
      { href: "/sources", label: "来源与证据", icon: LibraryBig },
      { href: "/entities", label: "实体与知识", icon: Waypoints },
      { href: "/federation", label: "跨 Wiki 联邦", icon: Globe2 },
    ],
  },
  {
    label: "共建",
    items: [
      { href: "/new", label: "创建页面", icon: SquarePen },
      { href: "/imports", label: "AI 导入中心", icon: Bot },
    ],
  },
  {
    label: "治理",
    items: [
      {
        href: "/governance/review",
        label: "审核队列",
        icon: ShieldCheck,
      },
      {
        href: "/governance/apply",
        label: "待应用",
        icon: BadgeCheck,
      },
      {
        href: "/governance/activity",
        label: "审计与标签",
        icon: ScrollText,
      },
      {
        href: "/governance/protections",
        label: "页面保护",
        icon: LockKeyhole,
      },
      {
        href: "/governance/fact-check",
        label: "事实一致性",
        icon: ShieldAlert,
      },
      {
        href: "/governance/ai-trust",
        label: "AI 信任策略",
        icon: BrainCircuit,
      },
      {
        href: "/governance/revision-storage",
        label: "历史版本存储",
        icon: Archive,
      },
    ],
  },
  {
    label: "管理",
    items: [
      {
        href: "/account",
        label: "个人中心",
        icon: UserRound,
      },
      {
        href: "/settings/cli",
        label: "Agent CLI 授权",
        icon: TerminalSquare,
      },
      {
        href: "/admin/members",
        label: "成员与角色",
        icon: UsersRound,
      },
    ],
  },
] as const;

function isActive(pathname: string, href: string, exact?: boolean) {
  if (exact) return pathname === href;
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function SiteNavigation() {
  const pathname = usePathname();

  return (
    <aside className="hidden w-56 shrink-0 border-r border-sidebar-border bg-sidebar/70 lg:block">
      <div className="sticky top-16 flex h-[calc(100vh-4rem)] flex-col overflow-y-auto px-3 py-6">
        <nav aria-label="主要导航" className="space-y-6">
          {GROUPS.map((group) => (
            <div key={group.label}>
              <p className="mb-2 px-2 text-[11px] font-semibold tracking-[0.14em] text-muted-foreground uppercase">
                {group.label}
              </p>
              <ul className="space-y-1">
                {group.items.map((item) => {
                  const active = isActive(pathname, item.href, "exact" in item && item.exact);
                  const Icon = item.icon;
                  return (
                    <li key={item.href}>
                      <Link
                        href={item.href}
                        aria-current={active ? "page" : undefined}
                        className={cn(
                          "group flex h-9 items-center gap-2.5 rounded-xl px-2.5 text-sm font-medium text-sidebar-foreground/72 transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                          active &&
                            "bg-sidebar-primary/9 text-sidebar-primary shadow-[inset_3px_0_0_var(--sidebar-primary)]",
                        )}
                      >
                        <Icon
                          className={cn(
                            "size-4 text-muted-foreground transition-colors group-hover:text-current",
                            active && "text-sidebar-primary",
                          )}
                          aria-hidden
                        />
                        {item.label}
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>

      </div>
    </aside>
  );
}
