/** Knowledge/Evidence 详情页的服务端数据获取。 */
import {
  ResponseError,
  type CitationDetail,
  type ClaimDetail,
  type EntityDetail,
  type ReferenceUsageListPage,
} from "../../../contracts/generated/typescript";
import { knowledgeApi, projectionApi } from "./api";

export type KnowledgeDetailResult<T> =
  | { kind: "ok"; detail: T; usages: ReferenceUsageListPage }
  | { kind: "not_found" };

async function fetchDetail<T>(
  detailRequest: () => Promise<T>,
  usagesRequest: () => Promise<ReferenceUsageListPage>,
): Promise<KnowledgeDetailResult<T>> {
  try {
    const [detail, usages] = await Promise.all([detailRequest(), usagesRequest()]);
    return { kind: "ok", detail, usages };
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      return { kind: "not_found" };
    }
    throw error;
  }
}

export function fetchEntityDetail(
  id: string,
): Promise<KnowledgeDetailResult<EntityDetail>> {
  return fetchDetail(
    () => knowledgeApi().getEntity({ id }),
    () => projectionApi().listEntityMentions({ id, pageSize: 100 }),
  );
}

export function fetchClaimDetail(
  id: string,
): Promise<KnowledgeDetailResult<ClaimDetail>> {
  return fetchDetail(
    () => knowledgeApi().getClaim({ id }),
    () => projectionApi().listClaimUsages({ id, pageSize: 100 }),
  );
}

export function fetchCitationDetail(
  id: string,
): Promise<KnowledgeDetailResult<CitationDetail>> {
  return fetchDetail(
    () => knowledgeApi().getCitation({ id }),
    () => projectionApi().listCitationUsages({ id, pageSize: 100 }),
  );
}
