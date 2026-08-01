"use client";

import {
  Activity,
  CircleAlert,
  Database,
  LoaderCircle,
  RefreshCw,
} from "lucide-react";
import useSWR from "swr";

import { Button } from "@/components/ui/button";
import { metaApi } from "@/lib/api";
import { cn } from "@/lib/utils";

async function readPlatformHealth() {
  const api = metaApi();
  const [health, readiness] = await Promise.all([
    api.getHealthz(),
    api.getReadyz(),
  ]);
  return { health, readiness };
}

export function PlatformHealthCard() {
  const { data, error, isLoading, isValidating, mutate } = useSWR(
    "platform:health-and-readiness",
    readPlatformHealth,
    {
      refreshInterval: 30_000,
      revalidateOnFocus: true,
    },
  );

  const ready = data?.readiness.status === "ok";

  return (
    <section
      className="min-w-0 rounded-2xl border border-border/75 bg-card p-4"
      aria-labelledby="platform-health-title"
      aria-live="polite"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p
            id="platform-health-title"
            className="flex items-center gap-2 text-sm font-semibold"
          >
            <Activity className="size-4 text-primary" aria-hidden />
            系统状态
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            API 存活与关键依赖就绪度
          </p>
        </div>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          disabled={isValidating}
          onClick={() => void mutate()}
          aria-label="刷新系统状态"
        >
          <RefreshCw
            className={cn("size-3.5", isValidating && "animate-spin")}
            aria-hidden
          />
        </Button>
      </div>

      {isLoading && !data ? (
        <div className="mt-4 flex items-center gap-2 text-xs text-muted-foreground">
          <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
          正在检查服务
        </div>
      ) : null}

      {error ? (
        <div className="mt-4 flex items-start gap-2 rounded-xl bg-destructive/6 p-3 text-xs text-destructive">
          <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>状态探针暂时不可达，请检查 API 网络与运行日志。</span>
        </div>
      ) : null}

      {data ? (
        <div className="mt-4 space-y-3">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <span className="min-w-0 truncate text-xs text-muted-foreground">
              {data.health.service} · {data.health.version}
            </span>
            <span
              className={cn(
                "inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-1 text-[11px] font-medium",
                ready
                  ? "bg-emerald-50 text-emerald-700"
                  : "bg-amber-50 text-amber-700",
              )}
            >
              <span
                className={cn(
                  "size-1.5 rounded-full",
                  ready ? "bg-emerald-500" : "bg-amber-500",
                )}
                aria-hidden
              />
              {ready ? "全部就绪" : "部分不可用"}
            </span>
          </div>
          <ul className="space-y-2 border-t border-border/70 pt-3">
            {Object.entries(data.readiness.checks).map(([name, status]) => (
              <li
                key={name}
                className="flex min-w-0 items-center justify-between gap-3 text-xs"
              >
                <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
                  <Database className="size-3 shrink-0" aria-hidden />
                  <span className="truncate">{name}</span>
                </span>
                <span
                  className={cn(
                    "max-w-32 truncate font-mono text-[10px]",
                    status === "ok"
                      ? "text-emerald-700"
                      : "text-amber-700",
                  )}
                  title={status}
                >
                  {status}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}
