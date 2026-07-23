"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import type { Proposal, ProposalPreview } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { parseDocument } from "@/lib/ast/schema";
import {
  CollaborationClient,
  type CollaborationStatus,
} from "@/lib/collaboration/client";
import { applyPageProposalOperations } from "@/lib/governance-page-patch";

export function MergeToWorkingDocument({
  proposal,
  preview,
}: {
  proposal: Proposal;
  preview: ProposalPreview;
}) {
  const router = useRouter();
  const clientRef = useRef<CollaborationClient | null>(null);
  const [status, setStatus] = useState<CollaborationStatus>("connecting");
  const [merging, setMerging] = useState(false);
  const enabled =
    proposal.status === "approved" &&
    proposal.targetType === "page" &&
    Boolean(proposal.targetId);

  useEffect(() => {
    if (!enabled || !proposal.targetId) return;
    const initialAst = parseDocument(preview.current.ast);
    const client = new CollaborationClient({
      pageId: proposal.targetId,
      initialAst,
      onAst: () => undefined,
      onStatus: setStatus,
    });
    clientRef.current = client;
    client.connect();
    return () => {
      clientRef.current = null;
      client.close();
    };
    // The server component replaces this component when preview data changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, proposal.id, proposal.targetId]);

  if (!enabled) return null;

  const merge = async () => {
    const client = clientRef.current;
    const documentId = client?.getDocumentId();
    if (!client || status !== "online" || !documentId) {
      toast.warning("工作副本尚未同步完成");
      return;
    }
    setMerging(true);
    try {
      const current = client.getAst();
      const merged = applyPageProposalOperations(
        current,
        proposal.operations,
        proposal.conflicts,
      );
      const prepared = client.prepareAstUpdate(merged);
      await governanceApi().mergeProposalToWorkingDocument({
        id: proposal.id,
        mergeProposalToWorkingDocumentRequest: {
          workingDocumentId: documentId,
          expectedSequence: prepared.expectedSequence,
          clientId: client.getClientId(),
          clientUpdateId: crypto.randomUUID(),
          currentAst: current as unknown as Record<string, unknown>,
          mergedAst: merged as unknown as Record<string, unknown>,
          updateBase64: bytesToBase64(prepared.update),
        },
      });
      toast.success("提案已合并到工作副本", {
        description: "尚未创建正式 Revision，可在协作编辑器复核后发布。",
      });
      router.refresh();
    } catch (error) {
      toast.error("合并到工作副本失败", {
        description:
          error instanceof Error ? error.message : "工作副本可能已变化，请刷新后重试。",
      });
    } finally {
      setMerging(false);
    }
  };

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-semibold">协作工作副本</h2>
          <p className="text-sm text-muted-foreground">
            {statusLabel(status)}。合并不会直接发布 Revision。
          </p>
        </div>
        <Button
          type="button"
          onClick={() => void merge()}
          disabled={status !== "online" || merging}
        >
          {merging ? "合并中…" : "合并到工作副本"}
        </Button>
      </div>
    </section>
  );
}

function statusLabel(status: CollaborationStatus): string {
  switch (status) {
    case "connecting":
      return "正在连接";
    case "syncing":
      return "正在恢复工作副本";
    case "online":
      return "工作副本已同步";
    case "offline":
      return "连接已断开";
    case "closed":
      return "连接已关闭";
  }
}

function bytesToBase64(value: Uint8Array): string {
  let binary = "";
  for (const byte of value) binary += String.fromCharCode(byte);
  return window.btoa(binary);
}
