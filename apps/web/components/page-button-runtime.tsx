"use client";

import { useEffect } from "react";

import { toast } from "sonner";

import {
  datasetsApi,
  governanceApi,
  pagesApi,
  readingApi,
} from "@/lib/api";
import {
  buttonActionSchema,
  parseDocument,
  type Block,
  type ButtonAction,
} from "@/lib/ast/schema";
import { clientUUID } from "@/lib/client-uuid";
import { newBlockId } from "@/lib/editor/ids";
import type { ProposalOperationV1 } from "../../../contracts/generated/typescript";

function decodeActions(button: HTMLButtonElement): ButtonAction[] {
  const rawJSON = button.dataset.actionsJson;
  let value: unknown;
  if (rawJSON) {
    value = JSON.parse(rawJSON);
  } else {
    const encoded = button.dataset.actions;
    if (!encoded) throw new Error("按钮缺少动作");
    const normalized = encoded.replace(/-/g, "+").replace(/_/g, "/");
    const bytes = Uint8Array.from(atob(normalized), (character) =>
      character.charCodeAt(0),
    );
    value = JSON.parse(new TextDecoder().decode(bytes));
  }
  return buttonActionSchema.array().min(1).max(10).parse(value);
}

function cloneBlockWithNewIDs(source: Block): Block {
  const block = structuredClone(source) as Block;
  const remap = (value: Block): void => {
    value.id = newBlockId();
    if ("children" in value && Array.isArray(value.children)) {
      for (const child of value.children) remap(child);
    }
    if (value.type === "button") {
      for (const action of value.actions) {
        if (action.type === "insert_blocks") {
          for (const child of action.blocks) remap(child);
        }
      }
    }
  };
  remap(block);
  return block;
}

async function insertBlocks(action: Extract<ButtonAction, { type: "insert_blocks" }>) {
  const current = await readingApi().getPageByID({
    id: action.target_page_id,
    contentMode: "full",
  });
  const content = current.content ?? null;
  const document = content?.astJson
    ? parseDocument(content.astJson)
    : { type: "document" as const, schema_version: 1 as const, children: [] };
  const api = governanceApi();
  const proposal = await api.createProposal({
    idempotencyKey: clientUUID(),
    createProposalRequest: {
      targetType: "page",
      targetId: action.target_page_id,
      ...(content?.revision.id
        ? { baseRevisionId: content.revision.id }
        : { baseStateVersion: 0 }),
      riskLevel: "medium",
    },
  });
  for (const [offset, source] of action.blocks.entries()) {
    const block = cloneBlockWithNewIDs(source);
    const operation = {
      schemaVersion: 1,
      operationType: "insert_block",
      base: content?.revision.id
        ? { revisionId: content.revision.id }
        : { stateVersion: 0 },
      target: { pageId: action.target_page_id },
      evidence: [],
      risk: {
        level: "medium",
        reasons: new Set(["confirmed page button inserts reviewed blocks"]),
      },
      payload: {
        parent_block_id: null,
        index: document.children.length + offset,
        block,
      },
    } as unknown as ProposalOperationV1;
    await api.addProposalOperation({
      id: proposal.id,
      proposalOperationV1: operation,
    });
  }
  await api.submitProposal({ id: proposal.id });
  toast.success("插入内容已提交治理审核");
}

async function updateRecord(
  action: Extract<ButtonAction, { type: "update_dataset_record" }>,
) {
  await datasetsApi().updateDatasetRecord({
    id: action.record_id,
    writeDatasetRecordRequest: {
      values: action.values,
      entityId: action.entity_id ?? undefined,
    },
  });
  toast.success("数据记录已更新");
}

function showNotification(
  action: Extract<ButtonAction, { type: "notification" }>,
) {
  switch (action.tone) {
    case "success":
      toast.success(action.message);
      break;
    case "info":
      toast.info(action.message);
      break;
    case "warning":
      toast.warning(action.message);
      break;
    case "error":
      toast.error(action.message);
      break;
    default:
      toast(action.message);
  }
}

async function executeActions(
  pageId: string,
  blockId: string,
  actions: ButtonAction[],
) {
  for (const [actionIndex, action] of actions.entries()) {
    switch (action.type) {
      case "insert_blocks":
        await insertBlocks(action);
        break;
      case "update_dataset_record":
        await updateRecord(action);
        break;
      case "notification":
        showNotification(action);
        break;
      case "webhook":
        await pagesApi().executePageButtonWebhook({
          id: pageId,
          blockId,
          executePageButtonWebhookRequest: {
            actionIndex,
            executionId: clientUUID(),
          },
        });
        toast.success("Webhook 已送达");
        break;
    }
  }
}

export function PageButtonRuntime({ pageId }: { pageId: string }) {
  useEffect(() => {
    const onClick = async (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) return;
      const button = target.closest<HTMLButtonElement>("[data-page-button]");
      if (!button || button.disabled) return;
      let actions: ButtonAction[];
      try {
        actions = decodeActions(button);
      } catch {
        toast.error("页面按钮配置无效");
        return;
      }
      const prompt =
        button.dataset.confirmation || "确定执行这个页面按钮的操作吗？";
      if (!window.confirm(prompt)) return;
      const label = button.textContent;
      button.disabled = true;
      button.setAttribute("aria-busy", "true");
      try {
        await executeActions(pageId, button.dataset.blockId ?? "", actions);
      } catch {
        toast.error("页面按钮执行失败", {
          description: "操作已停止，后续动作未执行。",
        });
      } finally {
        button.disabled = false;
        button.removeAttribute("aria-busy");
        button.textContent = label;
      }
    };
    document.addEventListener("click", onClick);
    return () => {
      document.removeEventListener("click", onClick);
    };
  }, [pageId]);

  return null;
}
