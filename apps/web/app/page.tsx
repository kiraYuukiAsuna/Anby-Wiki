import Link from "next/link";
import {
  ArrowRight,
  Bot,
  BookOpenText,
  Braces,
  Check,
  History,
  Layers3,
  Network,
  ShieldCheck,
  Sparkles,
  SquarePen,
  Waypoints,
} from "lucide-react";

import { Button } from "@/components/ui/button";

const PORTALS = [
  {
    title: "专题合集",
    description: "沿主题、实体类型和事实规则组织知识。",
    href: "/collections",
    action: "浏览合集",
    icon: Layers3,
    tone: "text-violet-700 bg-violet-100",
  },
  {
    title: "实体与知识",
    description: "浏览稳定身份、结构化事实与跨页面关系。",
    href: "/entities",
    action: "打开实体库",
    icon: Waypoints,
    tone: "text-indigo-700 bg-indigo-100",
  },
  {
    title: "创建百科页面",
    description: "从稳定页面身份开始，持续积累可回滚版本。",
    href: "/new",
    action: "开始撰写",
    icon: SquarePen,
    tone: "text-sky-700 bg-sky-100",
  },
  {
    title: "AI 知识导入",
    description: "从网页和文档抽取实体、事实与原始证据。",
    href: "/imports",
    action: "打开导入中心",
    icon: Bot,
    tone: "text-emerald-700 bg-emerald-100",
  },
  {
    title: "共同审核",
    description: "逐项比较、解决冲突，再让修改正式生效。",
    href: "/governance",
    action: "打开治理中心",
    icon: ShieldCheck,
    tone: "text-amber-700 bg-amber-100",
  },
] as const;

const PRINCIPLES = [
  {
    title: "内容有版本",
    description: "发布即不可变；编辑与回滚都产生新的正式 Revision。",
    icon: History,
  },
  {
    title: "事实有证据",
    description: "Claim 可以定位到来源版本、页码、片段与引用位置。",
    icon: Network,
  },
  {
    title: "AI 有边界",
    description: "模型只提交结构化 Proposal，不静默覆盖正式知识。",
    icon: Braces,
  },
] as const;

export default function Home() {
  return (
    <div className="w-full">
      <section className="relative isolate overflow-hidden border-b border-border/70">
        <div
          className="pointer-events-none absolute inset-0 -z-10 opacity-80"
          aria-hidden
        >
          <div className="absolute -top-44 left-[8%] size-96 rounded-full bg-primary/10 blur-3xl" />
          <div className="absolute top-16 right-[4%] size-80 rounded-full bg-cyan-300/18 blur-3xl" />
          <div className="wiki-grid absolute inset-0 opacity-45" />
        </div>

        <div className="mx-auto grid min-h-[34rem] w-full max-w-7xl items-center gap-10 px-5 py-16 lg:grid-cols-[minmax(0,1fr)_22rem] lg:px-8 lg:py-20 xl:gap-20">
          <div className="min-w-0 max-w-3xl">
            <p className="inline-flex items-center gap-2 rounded-full border border-primary/15 bg-background/70 px-3 py-1.5 text-xs font-semibold text-primary shadow-sm backdrop-blur">
              <Sparkles className="size-3.5" aria-hidden />
              人与 AI 共同维护的可信知识
            </p>
            <h1 className="mt-6 text-[clamp(2.8rem,6vw,5.4rem)] leading-[0.98] font-semibold tracking-[-0.065em] text-balance">
              让知识持续生长，
              <span className="mt-2 block bg-linear-to-r from-primary via-indigo-500 to-cyan-600 bg-clip-text text-transparent">
                也始终有据可查。
              </span>
            </h1>
            <p className="mt-7 max-w-2xl break-words text-base leading-8 text-muted-foreground sm:text-lg">
              Anby Wiki 把百科页面、结构化事实、来源证据与审核流程连在一起。
              你可以像使用维基百科一样阅读和共建，也可以让 AI
              安全地把散落资料转化为待审核知识。
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <Button size="lg" asChild className="rounded-xl px-4">
                <Link href="/collections">
                  探索知识
                  <ArrowRight aria-hidden />
                </Link>
              </Button>
              <Button
                size="lg"
                variant="outline"
                asChild
                className="rounded-xl bg-background/65 px-4 backdrop-blur"
              >
                <Link href="/new">
                  <SquarePen aria-hidden />
                  创建第一页
                </Link>
              </Button>
            </div>
          </div>

          <div className="relative hidden lg:block" aria-hidden>
            <div className="absolute inset-8 rounded-[2.5rem] bg-primary/15 blur-3xl" />
            <div className="relative rotate-2 rounded-[2rem] border border-white/80 bg-background/78 p-5 shadow-[0_34px_90px_rgb(30_41_59/0.16)] backdrop-blur-xl">
              <div className="flex items-center justify-between border-b pb-4">
                <span className="flex items-center gap-2 text-sm font-semibold">
                  <BookOpenText className="size-4 text-primary" />
                  一条可信事实
                </span>
                <span className="rounded-full bg-emerald-100 px-2 py-1 text-[10px] font-semibold text-emerald-700">
                  人工已核验
                </span>
              </div>
              <div className="py-5">
                <p className="text-[11px] font-semibold tracking-wider text-muted-foreground uppercase">
                  Claim
                </p>
                <p className="mt-2 text-lg font-semibold">发布日期 · 2026-07-31</p>
                <p className="mt-3 text-sm leading-6 text-muted-foreground">
                  不是正文里无法追踪的文字，而是独立、可验证、可被多个页面引用的事实。
                </p>
              </div>
              <div className="space-y-2 rounded-2xl bg-muted/65 p-3">
                {["来源版本已保存", "引用定位到原始片段", "修改需要审核"].map(
                  (item) => (
                    <div key={item} className="flex items-center gap-2 text-xs">
                      <span className="flex size-5 items-center justify-center rounded-full bg-emerald-100 text-emerald-700">
                        <Check className="size-3" />
                      </span>
                      {item}
                    </div>
                  ),
                )}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="mx-auto w-full max-w-7xl px-5 py-14 lg:px-8 lg:py-18">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="min-w-0">
            <p className="text-xs font-semibold tracking-[0.18em] text-primary uppercase">
              Knowledge portals
            </p>
            <h2 className="mt-2 text-2xl font-semibold tracking-[-0.035em] sm:text-3xl">
              从阅读到共建，一站完成
            </h2>
          </div>
          <p className="min-w-0 max-w-md text-sm leading-6 text-muted-foreground">
            每个入口都对应真实的领域能力；任务和审核不会因为离开当前页面而消失。
          </p>
        </div>

        <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
          {PORTALS.map((portal) => {
            const Icon = portal.icon;
            return (
              <Link
                key={portal.href}
                href={portal.href}
                className="group flex min-h-56 flex-col rounded-3xl border border-border/80 bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition duration-300 hover:-translate-y-1 hover:border-primary/20 hover:shadow-[0_22px_55px_rgb(15_23_42/0.09)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span
                  className={`flex size-10 items-center justify-center rounded-2xl ${portal.tone}`}
                >
                  <Icon className="size-4.5" aria-hidden />
                </span>
                <h3 className="mt-8 text-lg font-semibold">{portal.title}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {portal.description}
                </p>
                <span className="mt-auto flex items-center gap-1.5 pt-5 text-sm font-semibold text-primary">
                  {portal.action}
                  <ArrowRight
                    className="size-3.5 transition-transform group-hover:translate-x-1"
                    aria-hidden
                  />
                </span>
              </Link>
            );
          })}
        </div>
      </section>

      <section className="border-y border-border/70 bg-card/45">
        <div className="mx-auto grid w-full max-w-7xl gap-8 px-5 py-14 lg:grid-cols-[18rem_minmax(0,1fr)] lg:px-8">
          <div>
            <p className="text-xs font-semibold tracking-[0.18em] text-primary uppercase">
              Built for trust
            </p>
            <h2 className="mt-2 text-2xl font-semibold tracking-[-0.035em]">
              现代体验，维基原则
            </h2>
            <p className="mt-3 text-sm leading-7 text-muted-foreground">
              更顺畅的编辑和自动化，不以牺牲可追溯性为代价。
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            {PRINCIPLES.map((principle) => {
              const Icon = principle.icon;
              return (
                <article
                  key={principle.title}
                  className="rounded-2xl border border-border/75 bg-background/75 p-5"
                >
                  <Icon className="size-5 text-primary" aria-hidden />
                  <h3 className="mt-5 font-semibold">{principle.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {principle.description}
                  </p>
                </article>
              );
            })}
          </div>
        </div>
      </section>
    </div>
  );
}
