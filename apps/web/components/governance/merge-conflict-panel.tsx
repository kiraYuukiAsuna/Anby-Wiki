"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import type { MergeConflict, Proposal } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";

export function MergeConflictPanel({ proposal }: { proposal: Proposal }) {
  const router = useRouter();
  const [resolving, setResolving] = useState<string | null>(null);
  const conflicts = proposal.conflicts;
  if (conflicts.length === 0) return null;

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
      toast.success("冲突决议已记录");
      router.refresh();
    } catch {
      toast.error("冲突决议失败", {
        description: "页面 Current 可能再次变化，请刷新后重新检查。",
      });
    } finally {
      setResolving(null);
    }
  };

  return (
    <section className="rounded-lg border border-amber-500/50 bg-amber-500/5 p-4">
      <h2 className="text-lg font-semibold">MergeConflict 决议</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        逐项比较 Base / Current / Proposed。全部解决后 Proposal 恢复 approved，
        Apply 时仍会重新检查 Current。
      </p>
      <div className="mt-4 space-y-4">
        {conflicts.map((conflict) => (
          <article key={conflict.id} className="rounded-lg border border-border bg-background p-3">
            <header className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="font-medium">
                  {conflict.conflictType} · {conflict.targetBlockId ?? conflict.targetClaimId ?? "全局"}
                </p>
                <p className="text-xs text-muted-foreground">{conflict.status}</p>
              </div>
              {conflict.status === "open" ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={resolving === conflict.id}
                    onClick={() => void resolve(conflict, "choose_current")}
                  >
                    保留 Current
                  </Button>
                  <Button
                    size="sm"
                    disabled={resolving === conflict.id}
                    onClick={() => void resolve(conflict, "choose_proposed")}
                  >
                    采用 Proposed
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
                  已决议：{resolutionChoice(conflict)}
                </span>
              )}
            </header>
            <div className="mt-3 grid gap-2 lg:grid-cols-3">
              <ConflictValue label="Base" value={conflict.baseValue} />
              <ConflictValue label="Current" value={conflict.currentValue} />
              <ConflictValue label="Proposed" value={conflict.proposedValue} />
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
      return "人工选择保留 Current";
    case "choose_proposed":
      return "人工选择采用 Proposed";
    default:
      return "人工忽略此变更";
  }
}
