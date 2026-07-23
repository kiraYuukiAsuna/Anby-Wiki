// @vitest-environment node
/**
 * Web → Go API 端到端冒烟（M0-T06 验收）。
 *
 * 默认跳过；启动后端后通过环境变量启用：
 *   SMOKE_API_URL=http://localhost:8080 npm run test
 */
import { describe, expect, it } from "vitest";
import { metaApi } from "./api";

const baseUrl = process.env.SMOKE_API_URL;
const run = baseUrl ? it : it.skip;

describe("web → api smoke (生成客户端)", () => {
  run("getHealthz 返回 service/version", async () => {
    const res = await metaApi({ basePath: baseUrl }).getHealthz();
    expect(res.service).toBeTruthy();
    expect(res.version).toBeTruthy();
  });

  run("getReadyz 返回结构化依赖检查", async () => {
    const api = metaApi({ basePath: baseUrl });
    try {
      const res = await api.getReadyz();
      expect(res.status).toBe("ok");
    } catch (e) {
      // 依赖未配置时 readyz 返回 503，生成客户端抛 ResponseError，属预期路径
      const err = e as { response?: Response };
      expect(err.response?.status).toBe(503);
      const body = (await err.response?.json()) as { status: string };
      expect(body.status).toBe("unavailable");
    }
  });
});
