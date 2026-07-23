/**
 * 历史路径数据获取（服务端组件专用，M2-T05）。
 *
 * 与 lib/reading.ts 同约定：服务端组件直接调 lib/api.ts 的 historyApi 工厂；
 * 客户端「加载更多」翻页与回滚写操作才在客户端走 SWR / 事件内调用。
 */
import {
  ResponseError,
  type DocumentDiff,
  type RevisionDetail,
  type RevisionListPage,
} from "../../../contracts/generated/typescript";
import { historyApi } from "./api";

/** 历史列表每页大小（服务端首屏与客户端翻页共用）。 */
export const HISTORY_PAGE_SIZE = 20;

/** 读取结果：命中或 404（页面/Revision 不存在，或 Revision 不属于该页面）。 */
export type FetchHistoryResult<T> =
  | { kind: "ok"; data: T }
  | { kind: "not_found" };

async function fetchHistory<T>(
  request: () => Promise<T>,
): Promise<FetchHistoryResult<T>> {
  try {
    return { kind: "ok", data: await request() };
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      return { kind: "not_found" };
    }
    throw error;
  }
}

/** 读取 Revision 历史第一页（created_at DESC, id DESC）。 */
export function fetchRevisionList(
  id: string,
): Promise<FetchHistoryResult<RevisionListPage>> {
  return fetchHistory(() =>
    historyApi().listRevisions({ id, pageSize: HISTORY_PAGE_SIZE }),
  );
}

/** 读取单版详情（Revision 元信息 + canonical AST）。 */
export function fetchRevisionDetail(
  id: string,
  rid: string,
): Promise<FetchHistoryResult<RevisionDetail>> {
  return fetchHistory(() => historyApi().getRevision({ id, rid }));
}

/** 读取两版结构 Diff（from=base，to=current）。 */
export function fetchRevisionDiff(
  id: string,
  from: string,
  to: string,
): Promise<FetchHistoryResult<DocumentDiff>> {
  return fetchHistory(() => historyApi().diffRevisions({ id, from, to }));
}
