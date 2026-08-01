/** Governance 页面服务端数据入口；远端 DTO 全部来自生成客户端。 */
import {
  ResponseError,
  type Proposal,
  type ProposalPreview,
} from "../../../contracts/generated/typescript";

import { governanceApi } from "./api";
import { serverApiOptions } from "./server-api";

export type ProposalWorkspaceResult =
  | { kind: "ok"; proposal: Proposal; preview: ProposalPreview | null }
  | { kind: "not_found" };

export async function fetchProposalWorkspace(
  id: string,
): Promise<ProposalWorkspaceResult> {
  try {
    const api = governanceApi(await serverApiOptions());
    const proposal = await api.getProposal({ id });
    const hasContentOperations = proposal.operations.some((record) =>
      [
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
      ].includes(record.operation.operationType),
    );
    const preview = proposal.targetType === "page" && hasContentOperations
      ? await api.previewProposal({ id })
      : null;
    return { kind: "ok", proposal, preview };
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      return { kind: "not_found" };
    }
    throw error;
  }
}
