"use client";

import Link from "next/link";
import type { ComponentProps } from "react";

import { useSession } from "@/lib/auth";
import { workspaceAccessNotice } from "@/lib/workspace-access";

export function WorkspaceLink(props: ComponentProps<typeof Link>) {
  const { session } = useSession();
  const href = typeof props.href === "string" ? props.href : props.href.pathname ?? "";
  if (workspaceAccessNotice(href, session)) return null;
  return <Link {...props} />;
}
