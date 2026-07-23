/** Governance 页面服务端数据入口；远端 DTO 全部来自生成客户端。 */
import {
  ResponseError,
  type Proposal,
  type ProposalPreview,
} from "../../../contracts/generated/typescript";

import { governanceApi } from "./api";

export type ProposalWorkspaceResult =
  | { kind: "ok"; proposal: Proposal; preview: ProposalPreview | null }
  | { kind: "not_found" };

export async function fetchProposalWorkspace(
  id: string,
): Promise<ProposalWorkspaceResult> {
  try {
    const api = governanceApi();
    const proposal = await api.getProposal({ id });
    const preview = proposal.targetType === "page"
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
