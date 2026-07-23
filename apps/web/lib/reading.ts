/**
 * 阅读路径数据获取（服务端组件专用）。
 *
 * 服务端组件直接调用 lib/api.ts 的生成客户端工厂（readingApi），
 * 不在客户端组件里发阅读请求；客户端交互数据才走 SWR。
 */
import {
  ResponseError,
  type PageWithContent,
} from "../../../contracts/generated/typescript";
import { readingApi } from "./api";

/** 默认阅读命名空间。 */
export const DEFAULT_NAMESPACE = "main";

/** 读取结果：命中页面、未找到（404）、或已删除（410，重定向目标软删等）。 */
export type FetchPageResult =
  | { kind: "ok"; data: PageWithContent }
  | { kind: "not_found" }
  | { kind: "gone" };

async function fetchPage(
  request: () => Promise<PageWithContent>,
): Promise<FetchPageResult> {
  try {
    return { kind: "ok", data: await request() };
  } catch (error) {
    if (error instanceof ResponseError) {
      if (error.response.status === 404) return { kind: "not_found" };
      if (error.response.status === 410) return { kind: "gone" };
    }
    throw error;
  }
}

/** 按标题/别名读取页面当前版本（namespace=main）。 */
export function fetchPageByTitle(title: string): Promise<FetchPageResult> {
  return fetchPage(() =>
    readingApi().getPageByTitle({ namespace: DEFAULT_NAMESPACE, title }),
  );
}

/** 按页面 ID 读取当前版本。 */
export function fetchPageById(id: string): Promise<FetchPageResult> {
  return fetchPage(() => readingApi().getPageByID({ id }));
}
