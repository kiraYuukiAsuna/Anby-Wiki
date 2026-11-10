import type { ClaimDetail, EntityDetail, MergeConflict, Proposal } from "../../../contracts/generated/typescript";

export const ENTITY_STATUS_LABELS: Record<EntityDetail["status"], string> = {
  active: "可用", merged: "已合并", deleted: "已删除",
};
export const CLAIM_STATUS_LABELS: Record<ClaimDetail["status"], string> = {
  proposed: "待审核", published: "已发布", rejected: "已拒绝", superseded: "已被替代", deprecated: "已弃用",
};
export const CLAIM_VERIFICATION_LABELS: Record<ClaimDetail["verificationStatus"], string> = {
  unverified: "未核验", ai_checked: "AI 已核对", human_verified: "人工已核验", disputed: "存在争议",
};
export const CLAIM_RANK_LABELS: Record<ClaimDetail["rank"], string> = {
  preferred: "首选", normal: "普通", deprecated: "已弃用",
};
export const CLAIM_ORIGIN_LABELS: Record<ClaimDetail["originType"], string> = {
  human: "人工录入", ai: "AI 整理", import: "资料导入",
};
export const PROPOSAL_STATUS_LABELS: Record<Proposal["status"], string> = {
  draft: "草稿", submitted: "已提交", in_review: "审核中", approved: "已批准", rejected: "已拒绝",
  conflicted: "待解决冲突", applying: "正在处理", applied: "已生效", failed: "生效失败", rolled_back: "已撤销",
};
export const PROPOSAL_TARGET_LABELS: Record<Proposal["targetType"], string> = {
  wiki: "Wiki", page: "页面", entity: "实体", claim: "事实", collection: "合集", external_resource: "外部资源",
};
export const RISK_LABELS: Record<Proposal["riskLevel"], string> = {
  low: "低风险", medium: "中风险", high: "高风险", critical: "极高风险",
};
export const CONFLICT_STATUS_LABELS: Record<MergeConflict["status"], string> = {
  open: "待处理", resolved: "已解决", dismissed: "已忽略",
};
export const CONFLICT_TYPE_LABELS: Record<MergeConflict["conflictType"], string> = {
  revision: "页面版本冲突", block_hash: "内容块已变化", claim_state: "事实状态冲突", semantic: "内容或名称冲突",
};
