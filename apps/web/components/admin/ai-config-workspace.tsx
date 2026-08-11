"use client";

import {
  Activity,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  Network,
  Save,
  ShieldAlert,
  TestTube2,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type AIConfig,
  type UpdateAIConfigRequest,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";

const configSchema = z
  .object({
    enabled: z.boolean(),
    provider: z.enum(["openai-compatible", "deepseek"]),
    baseUrl: z
      .url("请输入合法的 HTTP(S) API 根地址")
      .refine((value) => value.startsWith("https://") || value.startsWith("http://"), {
        message: "API 根地址必须使用 HTTP 或 HTTPS",
      }),
    model: z.string().trim().min(1, "请输入模型 ID").max(256),
    responseFormat: z.enum(["json_object", "json_schema"]),
    requestTimeoutSeconds: z.coerce.number().int().min(5).max(300),
    maxAttempts: z.coerce.number().int().min(1).max(5),
    apiKey: z.string(),
    apiKeyConfigured: z.boolean(),
  })
  .superRefine((value, context) => {
    if (!value.apiKeyConfigured && value.apiKey.trim() === "") {
      context.addIssue({
        code: "custom",
        path: ["apiKey"],
        message: "首次保存必须填写 API Key",
      });
    }
  });

function ConfigForm({
  config,
  onSaved,
}: {
  config: AIConfig;
  onSaved: (value: AIConfig) => Promise<void>;
}) {
  const [enabled, setEnabled] = useState(config.enabled);
  const [provider, setProvider] = useState(config.provider);
  const [baseUrl, setBaseUrl] = useState(config.baseUrl);
  const [model, setModel] = useState(config.model);
  const [responseFormat, setResponseFormat] = useState(config.responseFormat);
  const [requestTimeoutSeconds, setRequestTimeoutSeconds] = useState(
    String(config.requestTimeoutSeconds),
  );
  const [maxAttempts, setMaxAttempts] = useState(String(config.maxAttempts));
  const [apiKey, setAPIKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  const save = async () => {
    const parsed = configSchema.safeParse({
      enabled,
      provider,
      baseUrl,
      model,
      responseFormat,
      requestTimeoutSeconds,
      maxAttempts,
      apiKey,
      apiKeyConfigured: config.apiKeyConfigured,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查模型配置");
      return;
    }
    const request: UpdateAIConfigRequest = {
      enabled: parsed.data.enabled,
      provider: parsed.data.provider,
      baseUrl: parsed.data.baseUrl.replace(/\/$/, ""),
      model: parsed.data.model,
      responseFormat: parsed.data.responseFormat,
      requestTimeoutSeconds: parsed.data.requestTimeoutSeconds,
      maxAttempts: parsed.data.maxAttempts,
      ...(parsed.data.apiKey.trim()
        ? { apiKey: parsed.data.apiKey.trim() }
        : {}),
    };
    setSaving(true);
    try {
      const value = await adminApi().updateAIConfig({
        updateAIConfigRequest: request,
      });
      setAPIKey("");
      await onSaved(value);
      toast.success("AI 模型配置已保存", {
        description: value.enabled
          ? "Worker 将开始领取排队中的导入任务。"
          : "配置已保存，导入消费者保持暂停。",
      });
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("只有站点管理员可以修改 AI 配置");
      } else {
        toast.error("AI 配置保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    setTesting(true);
    try {
      const result = await adminApi().testAIConfig();
      toast.success("模型连接与结构化输出正常", {
        description: `${result.provider} · ${result.model} · ${result.latencyMs} ms`,
      });
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("请先保存并启用 AI 配置");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 504
      ) {
        toast.error("模型配置测试超时", {
          description: "请检查供应商地址、模型 ID、超时设置和上游状态。",
        });
      } else {
        toast.error("模型配置测试失败", {
          description: "密钥不会显示在错误信息中，请重新填写后保存再试。",
        });
      }
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <section className="rounded-3xl border bg-card p-6 shadow-[0_18px_50px_-42px_rgb(15_23_42/0.55)] sm:p-7">
        <div className="flex flex-wrap items-start justify-between gap-4 border-b pb-5">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">抽取模型</h2>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              Semantic Kernel 负责供应商调用；Go 服务继续校验 Prompt 输出 Schema。
            </p>
          </div>
          <label className="flex cursor-pointer items-center gap-2 rounded-full border bg-muted/30 px-3 py-2 text-xs font-medium">
            <Checkbox
              checked={enabled}
              onCheckedChange={(value) => setEnabled(value === true)}
            />
            启用 AI 导入
          </label>
        </div>

        <div className="mt-6 grid gap-5 sm:grid-cols-2">
          <div>
            <Label htmlFor="ai-provider">供应商适配</Label>
            <select
              id="ai-provider"
              value={provider}
              onChange={(event) => {
                const next = event.target.value as typeof provider;
                setProvider(next);
                if (next === "deepseek" && !baseUrl.trim()) {
                  setBaseUrl("https://api.deepseek.com");
                }
              }}
              className="mt-2 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="deepseek">DeepSeek</option>
              <option value="openai-compatible">OpenAI compatible</option>
            </select>
          </div>
          <div>
            <Label htmlFor="ai-model">模型 ID</Label>
            <Input
              id="ai-model"
              className="mt-2 h-9"
              value={model}
              onChange={(event) => setModel(event.target.value)}
              placeholder="例如 deepseek-v4-flash"
              autoComplete="off"
            />
          </div>
          <div className="sm:col-span-2">
            <Label htmlFor="ai-base-url">API 根地址</Label>
            <Input
              id="ai-base-url"
              className="mt-2 h-9 font-mono text-xs"
              value={baseUrl}
              onChange={(event) => setBaseUrl(event.target.value)}
              placeholder="https://api.deepseek.com"
              inputMode="url"
              autoComplete="url"
            />
          </div>
          <div className="sm:col-span-2">
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="ai-api-key">API Key</Label>
              <span className="text-[10px] font-medium text-muted-foreground">
                {config.apiKeyConfigured ? "已加密保存 · 留空保持不变" : "尚未配置"}
              </span>
            </div>
            <Input
              id="ai-api-key"
              className="mt-2 h-9 font-mono"
              type="password"
              value={apiKey}
              onChange={(event) => setAPIKey(event.target.value)}
              placeholder={config.apiKeyConfigured ? "••••••••••••••••" : "输入供应商 API Key"}
              autoComplete="new-password"
            />
          </div>
          <div>
            <Label htmlFor="ai-response-format">结构化输出模式</Label>
            <select
              id="ai-response-format"
              value={responseFormat}
              onChange={(event) =>
                setResponseFormat(event.target.value as typeof responseFormat)
              }
              className="mt-2 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="json_object">JSON object（兼容性更好）</option>
              <option value="json_schema">Strict JSON Schema</option>
            </select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="ai-timeout">单次超时（秒）</Label>
              <Input
                id="ai-timeout"
                className="mt-2 h-9"
                type="number"
                min={5}
                max={300}
                value={requestTimeoutSeconds}
                onChange={(event) => setRequestTimeoutSeconds(event.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="ai-attempts">最多尝试</Label>
              <Input
                id="ai-attempts"
                className="mt-2 h-9"
                type="number"
                min={1}
                max={5}
                value={maxAttempts}
                onChange={(event) => setMaxAttempts(event.target.value)}
              />
            </div>
          </div>
        </div>

        <div className="mt-7 flex flex-wrap justify-end gap-3 border-t pt-5">
          <Button
            type="button"
            variant="outline"
            disabled={!config.apiKeyConfigured || !config.enabled || saving || testing}
            onClick={() => void test()}
          >
            {testing ? <LoaderCircle className="animate-spin" aria-hidden /> : <TestTube2 aria-hidden />}
            {testing ? "测试中…" : "测试已保存配置"}
          </Button>
          <Button type="button" disabled={saving || testing} onClick={() => void save()}>
            {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Save aria-hidden />}
            {saving ? "保存中…" : "保存配置"}
          </Button>
        </div>
      </section>

      <aside className="space-y-3">
        {[
          {
            icon: Network,
            title: "Semantic Kernel",
            detail: "独立内网 Sidecar，不接触数据库或站点会话。",
          },
          {
            icon: KeyRound,
            title: config.apiKeyConfigured ? "密钥已配置" : "等待密钥",
            detail: "AES-256-GCM 加密落库；读取接口永不返回明文。",
          },
          {
            icon: Activity,
            title: enabled ? "导入消费者启用" : "导入消费者暂停",
            detail: enabled
              ? "保存后 Worker 会领取队列任务。"
              : "排队任务保留，启用后继续处理。",
          },
        ].map(({ icon: Icon, title, detail }) => (
          <div key={title} className="rounded-2xl border bg-card p-4">
            <Icon className="size-4 text-violet-700" aria-hidden />
            <h3 className="mt-3 text-sm font-semibold">{title}</h3>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</p>
          </div>
        ))}
        {config.updatedAt ? (
          <p className="px-1 text-[10px] leading-5 text-muted-foreground">
            最近保存：{new Intl.DateTimeFormat("zh-CN", {
              dateStyle: "medium",
              timeStyle: "short",
            }).format(config.updatedAt)}
          </p>
        ) : null}
      </aside>
    </div>
  );
}

export function AIConfigWorkspace() {
  const session = useSession();
  const config = useSWR(
    session.isAuthenticated ? "admin:ai-config" : null,
    () => adminApi().getAIConfig(),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  if (session.isLoading || config.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在读取管理员配置…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理 AI 配置</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          供应商密钥和导入运行策略只向站点管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (config.error instanceof ResponseError && config.error.response.status === 403) {
    return (
      <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">需要管理员权限</h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          编辑者可以发起 AI 导入，但只有 admin 角色能修改模型、密钥和超时策略。
        </p>
      </div>
    );
  }
  if (config.error || !config.data) {
    return (
      <div className="rounded-3xl border border-destructive/25 bg-destructive/5 p-8">
        <ShieldAlert className="size-7 text-destructive" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">AI 配置服务不可用</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          请检查 API 的加密主密钥与 Semantic Kernel 内网配置。
        </p>
      </div>
    );
  }

  return (
    <ConfigForm
      key={`${config.data.updatedAt?.toISOString() ?? "default"}:${config.data.apiKeyConfigured}`}
      config={config.data}
      onSaved={async (value) => {
        await config.mutate(value, { revalidate: false });
      }}
    />
  );
}
