"use client";

import {
  ArrowRight,
  Braces,
  CheckCircle2,
  FileJson2,
  LoaderCircle,
  Plus,
  ShieldCheck,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

import {
  ProposalOperationV1FromJSON,
  ResponseError,
  type CreateProposalRequestRiskLevelEnum,
  type CreateProposalRequestTargetTypeEnum,
  type ProposalOperationV1,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { governanceApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { clientUUID } from "@/lib/client-uuid";
import { cn } from "@/lib/utils";

const UUID = "00000000-0000-4000-8000-000000000001";
const SITE_ID = "00000000-0000-7000-8000-000000000001";
const MAIN_NAMESPACE_ID = "00000000-0000-7000-8000-000000000101";

const OPERATION_TYPES = [
  "create_page",
  "rename_page",
  "create_redirect",
  "insert_block",
  "delete_block",
  "move_block",
  "replace_block",
  "insert_page_reference",
  "retarget_page_reference",
  "insert_entity_reference",
  "retarget_entity_reference",
  "insert_claim_reference",
  "retarget_claim_reference",
  "insert_citation_reference",
  "retarget_citation_reference",
  "retarget_external_link",
  "create_entity",
  "merge_entity",
  "create_claim",
  "supersede_claim",
  "add_claim_source",
  "add_collection_membership",
  "remove_collection_membership",
] as const;

type OperationType = (typeof OPERATION_TYPES)[number];

const OPERATION_LABEL: Record<OperationType, string> = {
  create_page: "创建页面",
  rename_page: "页面改名",
  create_redirect: "创建重定向",
  insert_block: "插入 Block",
  delete_block: "删除 Block",
  move_block: "移动 Block",
  replace_block: "替换 Block",
  insert_page_reference: "插入页面引用",
  retarget_page_reference: "重定向页面引用",
  insert_entity_reference: "插入实体引用",
  retarget_entity_reference: "重定向实体引用",
  insert_claim_reference: "插入 Claim 引用",
  retarget_claim_reference: "重定向 Claim 引用",
  insert_citation_reference: "插入 Citation 引用",
  retarget_citation_reference: "重定向 Citation 引用",
  retarget_external_link: "重定向外部链接",
  create_entity: "创建实体",
  merge_entity: "合并实体",
  create_claim: "创建 Claim",
  supersede_claim: "取代 Claim",
  add_claim_source: "绑定 Claim 来源",
  add_collection_membership: "加入合集",
  remove_collection_membership: "移出合集",
};

const proposalSchema = z.object({
  targetType: z.enum([
    "page",
    "entity",
    "claim",
    "collection",
    "external_resource",
  ]),
  targetId: z.union([z.literal(""), z.string().uuid("目标 ID 不是合法 UUID")]),
  baseRevisionId: z.union([
    z.literal(""),
    z.string().uuid("基线 Revision ID 不是合法 UUID"),
  ]),
  baseStateVersion: z.union([
    z.literal(""),
    z.coerce.number().int().min(0, "状态版本不能小于 0"),
  ]),
  riskLevel: z.enum(["low", "medium", "high", "critical"]),
});

const evidenceSchema = z
  .object({
    citation_id: z.string().uuid().optional(),
    source_chunk_id: z.string().uuid().optional(),
    note: z.string().min(1).max(1000).optional(),
  })
  .strict()
  .refine(
    (value) => value.citation_id || value.source_chunk_id || value.note,
    "证据至少需要 Citation、SourceChunk 或说明",
  );

const operationSchema = z
  .object({
    schema_version: z.literal(1),
    operation_type: z.enum(OPERATION_TYPES),
    base: z
      .record(z.string(), z.unknown())
      .refine((value) => Object.keys(value).length > 0, "base 不能为空"),
    target: z
      .record(z.string(), z.unknown())
      .refine((value) => Object.keys(value).length > 0, "target 不能为空"),
    expected_hash: z.union([
      z.null(),
      z.string().regex(/^[0-9a-f]{64}$/, "expected_hash 必须是 64 位小写十六进制"),
    ]),
    evidence: z.array(evidenceSchema),
    risk: z
      .object({
        level: z.enum(["low", "medium", "high", "critical"]),
        reasons: z.array(z.string().min(1)).min(1),
      })
      .strict(),
    payload: z.record(z.string(), z.unknown()),
  })
  .strict();

type DraftOperation = {
  id: string;
  type: OperationType;
  json: string;
};

function common(type: OperationType, target: Record<string, unknown>) {
  return {
    schema_version: 1,
    operation_type: type,
    base: { state_version: 0 },
    target,
    expected_hash: null,
    evidence: [{ note: "人工提案：请在提交审核前补充可验证依据" }],
    risk: { level: "medium", reasons: ["manual operation requires review"] },
    payload: {},
  };
}

function template(type: OperationType): Record<string, unknown> {
  const blockId = clientUUID();
  switch (type) {
    case "create_page":
      return {
        ...common(type, {
          wiki_id: SITE_ID,
          namespace_id: MAIN_NAMESPACE_ID,
        }),
        payload: {
          title: "新页面标题",
          language: "zh-Hans",
          content_model: "typed-block-ast",
          initial_ast: { schema_version: 1, children: [] },
          summary: "创建页面",
        },
      };
    case "rename_page":
      return { ...common(type, { page_id: UUID }), payload: { new_title: "新标题" } };
    case "create_redirect":
      return {
        ...common(type, { page_id: UUID }),
        payload: { target_kind: "page", target_page_id: UUID },
      };
    case "insert_block":
      return {
        ...common(type, { page_id: UUID }),
        payload: {
          index: 0,
          block: {
            id: blockId,
            type: "paragraph",
            content: [{ type: "text", text: "新增段落" }],
          },
        },
      };
    case "delete_block":
      return { ...common(type, { page_id: UUID, block_id: UUID }), payload: {} };
    case "move_block":
      return {
        ...common(type, { page_id: UUID, block_id: UUID }),
        payload: { parent_block_id: null, index: 0 },
      };
    case "replace_block":
      return {
        ...common(type, { page_id: UUID, block_id: UUID }),
        payload: {
          block: {
            id: UUID,
            type: "paragraph",
            content: [{ type: "text", text: "替换后的段落" }],
          },
        },
      };
    case "insert_page_reference":
      return {
        ...common(type, { page_id: UUID, block_id: UUID }),
        payload: { index: 0, target_page_id: UUID, display_text: "页面名称" },
      };
    case "retarget_page_reference":
      return {
        ...common(type, { page_id: UUID, block_id: UUID, node_id: "0" }),
        payload: { target_page_id: UUID, display_text: "页面名称" },
      };
    case "insert_entity_reference":
    case "insert_claim_reference":
    case "insert_citation_reference": {
      const key =
        type === "insert_entity_reference"
          ? "entity_id"
          : type === "insert_claim_reference"
            ? "claim_id"
            : "citation_id";
      return {
        ...common(type, { page_id: UUID, block_id: UUID, [key]: UUID }),
        payload: { index: 0, display_text: "引用文本" },
      };
    }
    case "retarget_entity_reference":
    case "retarget_claim_reference":
    case "retarget_citation_reference": {
      const key =
        type === "retarget_entity_reference"
          ? "entity_id"
          : type === "retarget_claim_reference"
            ? "claim_id"
            : "citation_id";
      return {
        ...common(type, {
          page_id: UUID,
          block_id: UUID,
          node_id: "0",
          [key]: UUID,
        }),
        payload: { display_text: "更新后的引用文本" },
      };
    }
    case "retarget_external_link":
      return {
        ...common(type, {
          page_id: UUID,
          block_id: UUID,
          node_id: "0",
          external_resource_id: UUID,
        }),
        payload: { url: "https://example.com", display_text: "外部资料" },
      };
    case "create_entity":
      return {
        ...common(type, { wiki_id: SITE_ID }),
        payload: {
          type_key: "concept",
          canonical_key: "new-concept",
          labels: [
            {
              language: "zh-Hans",
              label: "新概念",
              description: "",
              is_primary: true,
            },
          ],
        },
      };
    case "merge_entity":
      return {
        ...common(type, { entity_id: UUID }),
        payload: { target_entity_id: UUID, reason: "重复实体" },
      };
    case "create_claim":
      return {
        ...common(type, { entity_id: UUID }),
        payload: {
          property_key: "release_date",
          value: { date: "2026-01-01" },
          qualifiers: {},
          rank: "normal",
          origin_type: "human",
          citation_ids: [],
        },
      };
    case "supersede_claim":
      return {
        ...common(type, { claim_id: UUID }),
        payload: {
          subject_entity_id: UUID,
          property_key: "release_date",
          value: { date: "2026-01-01" },
          qualifiers: {},
          rank: "normal",
          origin_type: "human",
          citation_ids: [],
        },
      };
    case "add_claim_source":
      return {
        ...common(type, { claim_id: UUID, citation_id: UUID }),
        payload: { support_type: "supports" },
      };
    case "add_collection_membership":
      return {
        ...common(type, { collection_id: UUID, page_id: UUID }),
        payload: { sort_key: "0001", source_revision_id: UUID },
      };
    case "remove_collection_membership":
      return {
        ...common(type, { collection_id: UUID, page_id: UUID }),
        payload: {},
      };
  }
}

function suggestedProposal(type: OperationType) {
  const raw = template(type);
  const target = raw.target as Record<string, string>;
  if (type === "create_page") return { targetType: "page" as const, targetId: "" };
  if (type === "create_entity") return { targetType: "entity" as const, targetId: "" };
  if (type === "merge_entity")
    return { targetType: "entity" as const, targetId: target.entity_id };
  if (type === "create_claim")
    return { targetType: "entity" as const, targetId: target.entity_id };
  if (type === "supersede_claim" || type === "add_claim_source")
    return { targetType: "claim" as const, targetId: target.claim_id };
  if (
    type === "add_collection_membership" ||
    type === "remove_collection_membership"
  )
    return {
      targetType: "collection" as const,
      targetId: target.collection_id,
    };
  return { targetType: "page" as const, targetId: target.page_id };
}

function newDraftOperation(type: OperationType): DraftOperation {
  return {
    id: clientUUID(),
    type,
    json: JSON.stringify(template(type), null, 2),
  };
}

export function ProposalOperationStudio() {
  const router = useRouter();
  const session = useSession();
  const idempotencyKey = useRef(clientUUID());
  const [targetType, setTargetType] =
    useState<CreateProposalRequestTargetTypeEnum>("page");
  const [targetId, setTargetId] = useState("");
  const [baseRevisionId, setBaseRevisionId] = useState("");
  const [baseStateVersion, setBaseStateVersion] = useState("");
  const [riskLevel, setRiskLevel] =
    useState<CreateProposalRequestRiskLevelEnum>("medium");
  const [selectedType, setSelectedType] =
    useState<OperationType>("insert_block");
  const [operations, setOperations] = useState<DraftOperation[]>([]);
  const [createdProposalId, setCreatedProposalId] = useState<string>();
  const [appendedCount, setAppendedCount] = useState(0);
  const [saving, setSaving] = useState(false);

  const addTemplate = () => {
    if (createdProposalId && appendedCount === operations.length) {
      toast.message("可继续向当前草稿追加 Operation");
    }
    if (operations.length === 0 && !createdProposalId) {
      const suggestion = suggestedProposal(selectedType);
      setTargetType(suggestion.targetType);
      setTargetId(suggestion.targetId ?? "");
    }
    setOperations((current) => [...current, newDraftOperation(selectedType)]);
  };

  const updateOperation = (id: string, json: string) => {
    setOperations((current) =>
      current.map((operation) =>
        operation.id === id ? { ...operation, json } : operation,
      ),
    );
  };

  const removeOperation = (index: number) => {
    if (index < appendedCount) return;
    setOperations((current) => current.filter((_, itemIndex) => itemIndex !== index));
  };

  const save = async () => {
    if (!session.isAuthenticated) {
      toast.error("请先登录后创建人工提案");
      router.push(LOGIN_PATH);
      return;
    }
    const proposalInput = proposalSchema.safeParse({
      targetType,
      targetId: targetId.trim(),
      baseRevisionId: baseRevisionId.trim(),
      baseStateVersion: baseStateVersion.trim(),
      riskLevel,
    });
    if (!proposalInput.success) {
      toast.error(proposalInput.error.issues[0]?.message ?? "请检查提案目标");
      return;
    }
    if (operations.length === 0) {
      toast.error("至少添加一个 Operation");
      return;
    }

    const parsedOperations: ProposalOperationV1[] = [];
    for (let index = appendedCount; index < operations.length; index += 1) {
      const draft = operations[index];
      let raw: unknown;
      try {
        raw = JSON.parse(draft.json);
      } catch {
        toast.error(`Operation ${index + 1} 不是合法 JSON`);
        return;
      }
      const parsed = operationSchema.safeParse(raw);
      if (!parsed.success) {
        toast.error(`Operation ${index + 1} 边界校验失败`, {
          description: parsed.error.issues[0]?.message,
        });
        return;
      }
      parsedOperations.push(ProposalOperationV1FromJSON(parsed.data));
    }

    setSaving(true);
    let proposalId = createdProposalId;
    try {
      if (!proposalId) {
        const data = proposalInput.data;
        const proposal = await governanceApi().createProposal({
          idempotencyKey: idempotencyKey.current,
          createProposalRequest: {
            targetType: data.targetType,
            targetId: data.targetId || undefined,
            baseRevisionId: data.baseRevisionId || undefined,
            baseStateVersion:
              data.baseStateVersion === "" ? undefined : data.baseStateVersion,
            riskLevel: data.riskLevel,
          },
        });
        proposalId = proposal.id;
        setCreatedProposalId(proposal.id);
      }

      for (let index = 0; index < parsedOperations.length; index += 1) {
        await governanceApi().addProposalOperation({
          id: proposalId,
          proposalOperationV1: parsedOperations[index],
        });
        setAppendedCount((current) => current + 1);
      }
      toast.success("人工提案草稿已创建", {
        description: `${operations.length} 个 Operation 已通过权威 Schema。`,
      });
      router.push(`/governance/proposals/${proposalId}`);
      router.refresh();
    } catch (error) {
      if (error instanceof ResponseError) {
        const status = error.response.status;
        toast.error(
          status === 422
            ? "Operation 未通过服务端权威 Schema"
            : status === 403
              ? "当前账号没有创建该提案的权限"
              : "提案草稿尚未完整保存",
          {
            description: proposalId
              ? "已成功写入的步骤不会丢失；修正当前 JSON 后可从断点继续。"
              : `HTTP ${status}`,
          },
        );
      } else {
        toast.error("提案草稿尚未完整保存", {
          description: proposalId
            ? "草稿已保留，可从当前断点继续。"
            : "请检查网络后重试。",
        });
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_21rem]">
      <div className="space-y-6">
        <section className="rounded-3xl border bg-card p-6">
          <div className="flex items-start gap-4">
            <span className="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <ShieldCheck className="size-5" aria-hidden />
            </span>
            <div>
              <h2 className="text-xl font-semibold">提案边界</h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                目标与基线在草稿创建后冻结；所有 Operation 只追加到治理域，不直接写入权威知识。
              </p>
            </div>
          </div>

          <div className="mt-6 grid gap-5 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="proposal-target-type">目标类型</Label>
              <select
                id="proposal-target-type"
                value={targetType}
                disabled={Boolean(createdProposalId)}
                onChange={(event) =>
                  setTargetType(
                    event.target.value as CreateProposalRequestTargetTypeEnum,
                  )
                }
                className="h-9 w-full rounded-lg border bg-transparent px-2.5 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                <option value="page">Page</option>
                <option value="entity">Entity</option>
                <option value="claim">Claim</option>
                <option value="collection">Collection</option>
                <option value="external_resource">External resource</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="proposal-target-id">目标稳定 ID（新建时留空）</Label>
              <Input
                id="proposal-target-id"
                value={targetId}
                disabled={Boolean(createdProposalId)}
                onChange={(event) => setTargetId(event.target.value)}
                placeholder="UUID"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="proposal-base-revision">基线 Revision（可选）</Label>
              <Input
                id="proposal-base-revision"
                value={baseRevisionId}
                disabled={Boolean(createdProposalId)}
                onChange={(event) => setBaseRevisionId(event.target.value)}
                placeholder="页面内容操作建议填写"
                className="font-mono text-xs"
              />
            </div>
            <div className="grid grid-cols-[minmax(0,1fr)_9rem] gap-3">
              <div className="space-y-2">
                <Label htmlFor="proposal-base-state">状态版本（可选）</Label>
                <Input
                  id="proposal-base-state"
                  type="number"
                  min={0}
                  value={baseStateVersion}
                  disabled={Boolean(createdProposalId)}
                  onChange={(event) => setBaseStateVersion(event.target.value)}
                  placeholder="0"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="proposal-risk">初始风险</Label>
                <select
                  id="proposal-risk"
                  value={riskLevel}
                  disabled={Boolean(createdProposalId)}
                  onChange={(event) =>
                    setRiskLevel(
                      event.target.value as CreateProposalRequestRiskLevelEnum,
                    )
                  }
                  className="h-9 w-full rounded-lg border bg-transparent px-2.5 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                  <option value="low">低</option>
                  <option value="medium">中</option>
                  <option value="high">高</option>
                  <option value="critical">关键</option>
                </select>
              </div>
            </div>
          </div>
          {createdProposalId ? (
            <div className="mt-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-200 bg-emerald-50/60 p-3 text-xs text-emerald-900">
              <span className="flex items-center gap-2">
                <CheckCircle2 className="size-4" aria-hidden />
                草稿 {createdProposalId} 已创建，当前从第 {appendedCount + 1} 个 Operation 继续。
              </span>
              <Link
                href={`/governance/proposals/${createdProposalId}`}
                className="font-semibold hover:underline"
              >
                打开已保存草稿
              </Link>
            </div>
          ) : null}
        </section>

        <section className="rounded-3xl border bg-card p-6">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Operation 序列</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                按顺序原子应用；模板使用契约原生 snake_case JSON。
              </p>
            </div>
            <div className="flex gap-2">
              <select
                value={selectedType}
                onChange={(event) =>
                  setSelectedType(event.target.value as OperationType)
                }
                className="h-9 max-w-52 rounded-lg border bg-transparent px-2.5 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
                aria-label="Operation 模板"
              >
                {OPERATION_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {OPERATION_LABEL[type]}
                  </option>
                ))}
              </select>
              <Button type="button" variant="outline" onClick={addTemplate}>
                <Plus aria-hidden />
                添加模板
              </Button>
            </div>
          </div>

          {operations.length ? (
            <ol className="mt-6 space-y-4">
              {operations.map((operation, index) => {
                const persisted = index < appendedCount;
                return (
                  <li
                    key={operation.id}
                    className={cn(
                      "overflow-hidden rounded-2xl border",
                      persisted && "border-emerald-200 bg-emerald-50/25",
                    )}
                  >
                    <div className="flex items-center justify-between gap-3 border-b bg-muted/35 px-4 py-3">
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="flex size-7 items-center justify-center rounded-lg bg-background font-mono text-xs shadow-sm">
                          {index + 1}
                        </span>
                        <div className="min-w-0">
                          <p className="truncate text-sm font-semibold">
                            {OPERATION_LABEL[operation.type]}
                          </p>
                          <p className="font-mono text-[10px] text-muted-foreground">
                            {operation.type}
                          </p>
                        </div>
                      </div>
                      {persisted ? (
                        <span className="flex items-center gap-1 text-[11px] font-semibold text-emerald-700">
                          <CheckCircle2 className="size-3.5" aria-hidden />
                          已写入
                        </span>
                      ) : (
                        <Button
                          type="button"
                          size="icon-sm"
                          variant="ghost"
                          onClick={() => removeOperation(index)}
                          aria-label={`删除 Operation ${index + 1}`}
                        >
                          <Trash2 aria-hidden />
                        </Button>
                      )}
                    </div>
                    <Textarea
                      value={operation.json}
                      disabled={persisted || saving}
                      onChange={(event) =>
                        updateOperation(operation.id, event.target.value)
                      }
                      spellCheck={false}
                      aria-label={`Operation ${index + 1} JSON`}
                      className="min-h-72 resize-y rounded-none border-0 bg-slate-950 p-4 font-mono text-xs leading-5 text-slate-100 focus-visible:ring-0 dark:bg-black"
                    />
                  </li>
                );
              })}
            </ol>
          ) : (
            <div className="mt-6 rounded-2xl border border-dashed bg-muted/20 px-6 py-14 text-center">
              <FileJson2 className="mx-auto size-8 text-muted-foreground" aria-hidden />
              <p className="mt-4 font-semibold">从一个契约模板开始</p>
              <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
                选择上方操作类型。模板会预填结构与稳定 ID 位置，提交前请替换示例 UUID。
              </p>
            </div>
          )}
        </section>

        <Button
          type="button"
          size="lg"
          className="w-full"
          disabled={saving || session.isLoading}
          onClick={() => void save()}
        >
          {saving ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <ShieldCheck aria-hidden />
          )}
          {saving
            ? `正在写入 Operation ${appendedCount + 1}…`
            : createdProposalId
              ? "从断点继续保存"
              : "创建治理草稿"}
        </Button>
      </div>

      <aside className="space-y-4 xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border bg-card p-5">
          <Braces className="size-5 text-primary" aria-hidden />
          <h2 className="mt-4 font-semibold">双层校验</h2>
          <ol className="mt-4 space-y-3 text-xs leading-5 text-muted-foreground">
            <li>
              <span className="font-semibold text-foreground">1. 浏览器边界</span>
              <br />
              Zod 检查 envelope、风险、证据与 JSON 形态。
            </li>
            <li>
              <span className="font-semibold text-foreground">2. 权威契约</span>
              <br />
              Go 服务用 ProposalOperation v1 JSON Schema 校验每种目标组合。
            </li>
            <li>
              <span className="font-semibold text-foreground">3. 领域应用</span>
              <br />
              审核通过后仍由 Page / Knowledge / Collection 服务执行不变量。
            </li>
          </ol>
        </div>
        <div className="rounded-2xl border border-amber-200 bg-amber-50/60 p-5">
          <TriangleAlert className="size-5 text-amber-700" aria-hidden />
          <p className="mt-3 text-sm font-semibold text-amber-950">
            示例 UUID 不是实际对象
          </p>
          <p className="mt-1 text-xs leading-5 text-amber-900/75">
            站点与主命名空间已使用预发布种子 ID；其余以 0000 开头的 UUID 必须替换。
          </p>
        </div>
        <Link
          href="/governance"
          className="group flex items-center justify-between rounded-2xl border bg-muted/30 p-4 text-sm font-semibold transition hover:border-primary/20 hover:bg-primary/5"
        >
          返回提案工作台
          <ArrowRight
            className="size-4 transition-transform group-hover:translate-x-0.5"
            aria-hidden
          />
        </Link>
      </aside>
    </div>
  );
}
