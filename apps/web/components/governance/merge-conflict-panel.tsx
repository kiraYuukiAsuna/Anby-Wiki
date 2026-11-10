"use client";

import { WorkspaceLink } from "@/components/workspace-link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { z } from "zod";
import type { MergeConflict, Proposal } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { CONFLICT_STATUS_LABELS, CONFLICT_TYPE_LABELS } from "@/lib/display-labels";

export function MergeConflictPanel({ proposal }: { proposal: Proposal }) {
  const router = useRouter();
  const [resolving, setResolving] = useState<string | null>(null);
  const conflicts = proposal.conflicts;
  if (conflicts.length === 0) return null;
  const hasIdentityConflict = conflicts.some(isIdentityConflict);

  const resolve = async (
    conflict: MergeConflict,
    choice: "choose_current" | "choose_proposed" | "dismiss",
  ) => {
    setResolving(conflict.id);
    try {
      await governanceApi().resolveMergeConflict({
        id: proposal.id,
        conflictId: conflict.id,
        resolveMergeConflictRequest: {
          choice,
          reason: resolutionReason(choice),
        },
      });
      toast.success("冲突处理结果已保存");
      router.refresh();
    } catch {
      toast.error("冲突处理失败", {
        description: "页面可能又有更新，请刷新后重新检查。",
      });
    } finally {
      setResolving(null);
    }
  };

  return (
    <section className="rounded-lg border border-amber-500/50 bg-amber-500/5 p-4">
      <h2 className="text-lg font-semibold">解决冲突</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        {hasIdentityConflict
          ? "同名页面或实体已被其他修改创建。请根据最新内容重新导入，原修改建议会保留。"
          : "逐项对比原版本、当前版本和建议内容。解决全部冲突后可继续生效操作；执行时仍会检查是否有新修改。"}
      </p>
      {hasIdentityConflict && proposal.importJobId ? (
        <Button className="mt-3" size="sm" variant="outline" asChild>
          <WorkspaceLink href={`/imports/${proposal.importJobId}`}>查看原导入任务</WorkspaceLink>
        </Button>
      ) : null}
      <div className="mt-4 space-y-4">
        {conflicts.map((conflict) => (
          <article key={conflict.id} className="rounded-lg border border-border bg-background p-3">
            <header className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="font-medium">
                  {CONFLICT_TYPE_LABELS[conflict.conflictType]} · {conflict.targetBlockId ?? conflict.targetClaimId ?? "全局"}
                </p>
                <p className="text-xs text-muted-foreground">{CONFLICT_STATUS_LABELS[conflict.status]}</p>
              </div>
              {conflict.status === "open" && isIdentityConflict(conflict) ? (
                <span className="text-xs font-medium text-amber-800 dark:text-amber-300">
                  需重新导入，不能自动合并
                </span>
              ) : conflict.status === "open" ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={resolving === conflict.id}
                    onClick={() => void resolve(conflict, "choose_current")}
                  >
                    保留当前内容
                  </Button>
                  <Button
                    size="sm"
                    disabled={resolving === conflict.id}
                    onClick={() => void resolve(conflict, "choose_proposed")}
                  >
                    采用建议内容
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={resolving === conflict.id}
                    onClick={() => void resolve(conflict, "dismiss")}
                  >
                    忽略此变更
                  </Button>
                </div>
              ) : (
                <span className="text-xs text-muted-foreground">
                  处理结果：{resolutionChoice(conflict)}
                </span>
              )}
            </header>
            <div className="mt-3 grid gap-2 lg:grid-cols-3">
              <ConflictValue label="原版本" value={conflict.baseValue} />
              <ConflictValue label="当前版本" value={conflict.currentValue} />
              <ConflictValue label="建议内容" value={conflict.proposedValue} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function ConflictValue({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="min-w-0 rounded border border-border">
      <p className="border-b border-border px-2 py-1 text-xs font-medium">{label}</p>
      <pre className="max-h-52 overflow-auto p-2 text-xs">
        {value == null ? "null" : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function resolutionChoice(conflict: MergeConflict): string {
  const resolution = conflict.resolution as { choice?: string } | undefined;
  return resolution?.choice ?? conflict.status;
}

function resolutionReason(choice: string): string {
  switch (choice) {
    case "choose_current":
      return "已选择保留当前内容";
    case "choose_proposed":
      return "已选择采用建议内容";
    default:
      return "人工忽略此变更";
  }
}

const identityConflictValueSchema = z.object({
  kind: z.enum(["page_identity", "entity_identity"]),
});

function isIdentityConflict(conflict: MergeConflict): boolean {
  return (
    conflict.conflictType === "semantic" &&
    identityConflictValueSchema.safeParse(conflict.proposedValue).success
  );
}
