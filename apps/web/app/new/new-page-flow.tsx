// 创建页面流程（M2-T03）：/new?title=...&namespace=main。
//
// 流程：无 title → 标题输入表单；有 title → pagesApi.createPage →
// 成功跳 /pages/[id]/edit；标题冲突（409 conflict）→ toast + 跳既有页面阅读页；
// 其他错误保留在本页可重试；401 引导 OIDC 登录。
"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { pagesApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

/** 与 lib/reading.DEFAULT_NAMESPACE 一致；本组件为客户端组件，不引入服务端阅读模块。 */
const DEFAULT_NAMESPACE = "main";

type Status = "creating" | "unauthenticated" | "error";

export function NewPageFlow() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const title = (searchParams.get("title") ?? "").trim();
  const namespace = searchParams.get("namespace") ?? DEFAULT_NAMESPACE;

  const [status, setStatus] = useState<Status>("creating");
  const [titleInput, setTitleInput] = useState("");
  // 重试计数：每次 attempt 变化触发一次创建；ref 防止同一 attempt 重复执行
  // （StrictMode 双调用 effect）。
  const [attempt, setAttempt] = useState(0);
  const ranAttemptRef = useRef(-1);

  useEffect(() => {
    if (!title || ranAttemptRef.current === attempt) return;
    ranAttemptRef.current = attempt;

    pagesApi()
      .createPage({
        createPageRequest: { namespace, title },
      })
      .then((page) => {
        toast.success(`页面「${page.displayTitle}」已创建`);
        router.replace(`/pages/${page.id}/edit`);
      })
      .catch((error: unknown) => {
        if (error instanceof ResponseError) {
          if (error.response.status === 409) {
            // 标题冲突（含与别名冲突）：跳既有页面阅读页，由用户从那里进入编辑。
            toast.info("页面已存在", {
              description: "该标题（或其别名）已被占用，已为你跳转到既有页面。",
            });
            router.replace(`/wiki/${encodeURIComponent(title)}`);
            return;
          }
          if (isUnauthorized(error)) {
            setStatus("unauthenticated");
            return;
          }
        }
        setStatus("error");
        toast.error("创建页面失败", {
          description: "网络或服务端错误，可重试。",
        });
      });
  }, [title, namespace, attempt, router]);

  // 无 title：标题输入表单（顶栏「新建页面」入口）。
  if (!title) {
    return (
      <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-4 px-4 py-16">
        <h1 className="text-2xl font-semibold">新建页面</h1>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            const nextTitle = titleInput.trim();
            if (!nextTitle) {
              toast.error("请输入页面标题");
              return;
            }
            router.push(
              `/new?title=${encodeURIComponent(nextTitle)}&namespace=${DEFAULT_NAMESPACE}`,
            );
          }}
        >
          <Label htmlFor="new-page-title">页面标题</Label>
          <Input
            id="new-page-title"
            value={titleInput}
            onChange={(event) => setTitleInput(event.target.value)}
            placeholder="输入新页面的标题"
            autoFocus
          />
          <Button type="submit">创建并编辑</Button>
        </form>
      </div>
    );
  }

  if (status === "unauthenticated") {
    return (
      <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-4 px-4 py-16">
        <h1 className="text-2xl font-semibold">需要登录</h1>
        <p className="text-sm text-muted-foreground">
          创建页面前需要先登录。登录完成后可返回本页重试。
        </p>
        <Button asChild>
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-4 px-4 py-16">
        <h1 className="text-2xl font-semibold">创建失败</h1>
        <p className="text-sm text-muted-foreground">
          创建「{title}」时出错，请检查网络与 API 服务后重试。
        </p>
        <div className="flex gap-2">
          <Button onClick={() => setAttempt((n) => n + 1)}>重试</Button>
          <Button variant="outline" asChild>
            <Link href="/">返回首页</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-4 px-4 py-16">
      <h1 className="text-2xl font-semibold">正在创建「{title}」…</h1>
      <p className="text-sm text-muted-foreground">
        创建成功后将自动进入编辑页。
      </p>
    </div>
  );
}
