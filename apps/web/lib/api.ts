/**
 * API 客户端唯一入口。
 *
 * 客户端由 `npm run gen:client` 从 contracts/openapi/openapi.yaml 生成
 * （产物：contracts/generated/typescript，禁止手改）。
 * 页面与组件禁止手写 fetch URL 或 DTO 类型，一律经由本模块导出的客户端访问后端；
 * 远端数据缓存统一走 SWR。
 */
import {
  AuthApi,
  CollectionsApi,
  Configuration,
  GovernanceApi,
  HistoryApi,
  ImportsApi,
  KnowledgeApi,
  MetaApi,
  PagesApi,
  ProjectionApi,
  ReadingApi,
  SearchApi,
  type ConfigurationParameters,
  type Middleware,
} from "../../../contracts/generated/typescript";

/**
 * 后端 API 基础地址，本地开发默认指向 Go API 服务。
 *
 * 配置优先级（服务端）：API_BASE_URL > NEXT_PUBLIC_API_BASE_URL > 默认值。
 * 原因：NEXT_PUBLIC_* 变量在客户端构建期内联，无法支持运行时配置；
 * 服务端组件（SSR）允许读取运行时环境变量，因此部署时如需不改构建产物
 * 就切换后端地址，应设置 API_BASE_URL（仅服务端可见，不会泄漏到客户端）。
 * 客户端组件（SWR 等）只能使用构建期内联的 NEXT_PUBLIC_API_BASE_URL。
 */
export function getBaseUrl(): string {
  if (typeof window === "undefined") {
    return (
      process.env.API_BASE_URL ??
      process.env.NEXT_PUBLIC_API_BASE_URL ??
      "http://localhost:8080"
    );
  }
  // 浏览器默认走当前 Next.js origin；next.config.ts 将 /api 代理到 Go API。
  // 生产 Nginx 也采用同源 /api，避免本地开发和部署都依赖跨域放行。
  return process.env.NEXT_PUBLIC_API_BASE_URL ?? window.location.origin;
}

/** 生成请求 ID，用于全链路追踪（trace/log 关联）。 */
export function makeRequestId(): string {
  return globalThis.crypto.randomUUID();
}

/** 为每个请求注入 X-Request-ID（服务端透传或重新生成，见契约 components/headers）。 */
const requestIdMiddleware: Middleware = {
  pre: async (context) => ({
    url: context.url,
    init: {
      ...context.init,
      headers: new Headers({
        ...context.init.headers,
        "X-Request-ID": makeRequestId(),
      }),
    },
  }),
};

function makeConfig(overrides: ConfigurationParameters = {}): Configuration {
  return new Configuration({
    basePath: getBaseUrl(),
    credentials: "same-origin",
    middleware: [requestIdMiddleware],
    ...overrides,
  });
}

/** OIDC 浏览器登录与服务端 session 客户端。 */
export function authApi(overrides: ConfigurationParameters = {}): AuthApi {
  return new AuthApi(makeConfig(overrides));
}

/** 元信息与健康检查客户端。 */
export function metaApi(overrides: ConfigurationParameters = {}): MetaApi {
  return new MetaApi(makeConfig(overrides));
}

/** 页面写 API 客户端（创建/改名/发布 Revision）。 */
export function pagesApi(overrides: ConfigurationParameters = {}): PagesApi {
  return new PagesApi(makeConfig(overrides));
}

/** 页面阅读 API 客户端（按标题/别名/ID 读取当前 Revision 与渲染 HTML，匿名可读）。 */
export function readingApi(overrides: ConfigurationParameters = {}): ReadingApi {
  return new ReadingApi(makeConfig(overrides));
}

/** 页面历史 API 客户端（Revision 列表/详情/结构 Diff/回滚；读匿名，回滚需登录）。 */
export function historyApi(overrides: ConfigurationParameters = {}): HistoryApi {
  return new HistoryApi(makeConfig(overrides));
}

/** 投影查询 API 客户端（反向链接/文档目录，走投影表，匿名可读，最终一致）。 */
export function projectionApi(overrides: ConfigurationParameters = {}): ProjectionApi {
  return new ProjectionApi(makeConfig(overrides));
}

/** 全局页面搜索客户端（标题/别名/正文/Entity，支持过滤与高亮，匿名可读）。 */
export function searchApi(overrides: ConfigurationParameters = {}): SearchApi {
  return new SearchApi(makeConfig(overrides));
}

/** Knowledge 客户端：详情匿名可读，Entity 合并需要管理员会话。 */
export function knowledgeApi(
  overrides: ConfigurationParameters = {},
): KnowledgeApi {
  return new KnowledgeApi(makeConfig(overrides));
}

/** Manual/Rule Collection 定义与物化成员查询客户端（匿名只读）。 */
export function collectionsApi(
  overrides: ConfigurationParameters = {},
): CollectionsApi {
  return new CollectionsApi(makeConfig(overrides));
}

/** Proposal、Preview、Review、Apply 与 ChangeBatch 回滚客户端。 */
export function governanceApi(
  overrides: ConfigurationParameters = {},
): GovernanceApi {
  return new GovernanceApi(makeConfig(overrides));
}

/** AI 来源导入任务、阶段进度、取消与重试客户端。 */
export function importsApi(
  overrides: ConfigurationParameters = {},
): ImportsApi {
  return new ImportsApi(makeConfig(overrides));
}
