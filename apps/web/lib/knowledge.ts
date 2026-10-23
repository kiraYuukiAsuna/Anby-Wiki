/** Knowledge/Evidence 详情页的服务端数据获取。 */
import {
  ResponseError,
  type CitationDetail,
  type ClaimDetail,
  type EntityDetail,
  type ReferenceUsageListPage,
} from "../../../contracts/generated/typescript";
import { knowledgeApi, projectionApi } from "./api";
import { serverApiOptions } from "./server-api";

const REFERENCE_USAGE_PAGE_SIZE = 50;

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

export async function fetchEntityDetail(
  id: string,
): Promise<KnowledgeDetailResult<EntityDetail>> {
  const options = await serverApiOptions();
  return fetchDetail(
    () => knowledgeApi(options).getEntity({ id }),
    () =>
      projectionApi(options).listEntityMentions({
        id,
        pageSize: REFERENCE_USAGE_PAGE_SIZE,
      }),
  );
}

export async function fetchClaimDetail(
  id: string,
): Promise<KnowledgeDetailResult<ClaimDetail>> {
  const options = await serverApiOptions();
  return fetchDetail(
    () => knowledgeApi(options).getClaim({ id }),
    () =>
      projectionApi(options).listClaimUsages({
        id,
        pageSize: REFERENCE_USAGE_PAGE_SIZE,
      }),
  );
}

export async function fetchCitationDetail(
  id: string,
): Promise<KnowledgeDetailResult<CitationDetail>> {
  const options = await serverApiOptions();
  return fetchDetail(
    () => knowledgeApi(options).getCitation({ id }),
    () =>
      projectionApi(options).listCitationUsages({
        id,
        pageSize: REFERENCE_USAGE_PAGE_SIZE,
      }),
  );
}
