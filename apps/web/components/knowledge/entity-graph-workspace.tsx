"use client";

import {
  ArrowDownLeft,
  ArrowLeftRight,
  ArrowUpRight,
  Braces,
  CheckCircle2,
  CircleAlert,
  ExternalLink,
  GitBranch,
  LoaderCircle,
  Network,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  type KeyboardEvent,
  useEffect,
  useMemo,
  useState,
} from "react";
import { toast } from "sonner";
import useSWR from "swr";

import {
  ResponseError,
  type EntityCatalogItem,
  type EntityGraph,
  type EntityGraphEdge,
  type EntityGraphNode,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

type Direction = "outbound" | "inbound" | "both";

const DIRECTION_META: Record<
  Direction,
  { label: string; detail: string; icon: typeof ArrowLeftRight }
> = {
  outbound: {
    label: "向外",
    detail: "该 Entity 指向谁",
    icon: ArrowUpRight,
  },
  inbound: {
    label: "向内",
    detail: "谁指向该 Entity",
    icon: ArrowDownLeft,
  },
  both: {
    label: "双向",
    detail: "同时探索上下游",
    icon: ArrowLeftRight,
  },
};

const GRAPH_WIDTH = 1120;
const GRAPH_HEIGHT = 660;
const GRAPH_CENTER_X = GRAPH_WIDTH / 2;
const GRAPH_CENTER_Y = GRAPH_HEIGHT / 2;
const RING_RADIUS = [0, 118, 218, 300];
const GRAPH_COLORS = [
  "#4f46e5",
  "#0f766e",
  "#b45309",
  "#be185d",
  "#7c3aed",
  "#0369a1",
];

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

function propertyColor(propertyKey: string) {
  let hash = 0;
  for (const char of propertyKey) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }
  return GRAPH_COLORS[hash % GRAPH_COLORS.length];
}

type PositionedNode = EntityGraphNode & { x: number; y: number };

function positionNodes(nodes: Array<EntityGraphNode>): Array<PositionedNode> {
  const byDepth = new Map<number, Array<EntityGraphNode>>();
  for (const node of nodes) {
    const group = byDepth.get(node.depth) ?? [];
    group.push(node);
    byDepth.set(node.depth, group);
  }
  const positioned: Array<PositionedNode> = [];
  for (const [depth, group] of byDepth) {
    if (depth === 0) {
      for (const node of group) {
        positioned.push({
          ...node,
          x: GRAPH_CENTER_X,
          y: GRAPH_CENTER_Y,
        });
      }
      continue;
    }
    const radius = RING_RADIUS[Math.min(depth, RING_RADIUS.length - 1)];
    group.forEach((node, index) => {
      const angle =
        -Math.PI / 2 +
        (index * Math.PI * 2) / Math.max(1, group.length) +
        (depth % 2 === 0 ? Math.PI / Math.max(4, group.length) : 0);
      positioned.push({
        ...node,
        x: GRAPH_CENTER_X + Math.cos(angle) * radius,
        y: GRAPH_CENTER_Y + Math.sin(angle) * radius,
      });
    });
  }
  return positioned;
}

function shortLabel(value: string, maximum = 11) {
  return value.length > maximum ? `${value.slice(0, maximum)}…` : value;
}

function GraphCanvas({
  graph,
  onChooseRoot,
}: {
  graph: EntityGraph;
  onChooseRoot: (node: EntityGraphNode) => void;
}) {
  const nodes = useMemo(() => positionNodes(graph.nodes), [graph.nodes]);
  const nodeById = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );

  const activateNode = (
    event: KeyboardEvent<SVGGElement>,
    node: EntityGraphNode,
  ) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onChooseRoot(node);
    }
  };

  return (
    <div className="relative overflow-hidden rounded-2xl border bg-[radial-gradient(circle_at_center,color-mix(in_oklch,var(--primary),transparent_93%),transparent_50%),linear-gradient(to_right,color-mix(in_oklch,var(--border),transparent_70%)_1px,transparent_1px),linear-gradient(to_bottom,color-mix(in_oklch,var(--border),transparent_70%)_1px,transparent_1px)] bg-[size:auto,28px_28px,28px_28px]">
      <svg
        viewBox={`0 0 ${GRAPH_WIDTH} ${GRAPH_HEIGHT}`}
        className="aspect-[1120/660] min-h-[32rem] w-full"
        role="img"
        aria-label={`Entity 关系图，共 ${graph.nodes.length} 个节点和 ${graph.edges.length} 条边`}
      >
        <defs>
          <marker
            id="graph-arrow"
            markerWidth="7"
            markerHeight="7"
            refX="18"
            refY="3.5"
            orient="auto"
            markerUnits="strokeWidth"
          >
            <path d="M0,0 L0,7 L7,3.5 z" fill="currentColor" />
          </marker>
          <filter id="node-shadow" x="-30%" y="-30%" width="160%" height="160%">
            <feDropShadow dx="0" dy="2" stdDeviation="3" floodOpacity="0.16" />
          </filter>
        </defs>
        {Array.from({ length: graph.requestedDepth }, (_, index) => (
          <circle
            key={index}
            cx={GRAPH_CENTER_X}
            cy={GRAPH_CENTER_Y}
            r={RING_RADIUS[index + 1]}
            fill="none"
            stroke="currentColor"
            strokeOpacity="0.08"
            strokeDasharray="5 8"
          />
        ))}
        <g aria-label="关系边">
          {graph.edges.map((edge) => {
            const source = nodeById.get(edge.subjectEntityId);
            const target = nodeById.get(edge.targetEntityId);
            if (!source || !target) return null;
            const color = propertyColor(edge.propertyKey);
            const midX = (source.x + target.x) / 2;
            const midY = (source.y + target.y) / 2;
            return (
              <g key={edge.claimId}>
                <title>
                  {source.label} — {edge.propertyName} → {target.label}
                </title>
                <line
                  x1={source.x}
                  y1={source.y}
                  x2={target.x}
                  y2={target.y}
                  stroke={color}
                  strokeWidth={edge.rank === "preferred" ? 2.4 : 1.5}
                  strokeOpacity={edge.verificationStatus === "disputed" ? 0.45 : 0.68}
                  strokeDasharray={
                    edge.verificationStatus === "disputed" ? "5 5" : undefined
                  }
                  markerEnd="url(#graph-arrow)"
                />
                {graph.edges.length <= 24 ? (
                  <text
                    x={midX}
                    y={midY - 5}
                    textAnchor="middle"
                    fontSize="9"
                    fill={color}
                    className="select-none font-medium"
                    paintOrder="stroke"
                    stroke="var(--background)"
                    strokeWidth="4"
                  >
                    {shortLabel(edge.propertyName, 9)}
                  </text>
                ) : null}
              </g>
            );
          })}
        </g>
        <g aria-label="Entity 节点">
          {nodes.map((node) => {
            const root = node.id === graph.rootId;
            const radius = root ? 34 : node.depth === 1 ? 26 : 21;
            return (
              <g
                key={node.id}
                role="link"
                tabIndex={0}
                aria-label={`${node.label}，${node.entityTypeName}，深度 ${node.depth}`}
                transform={`translate(${node.x},${node.y})`}
                onClick={() => onChooseRoot(node)}
                onKeyDown={(event) => activateNode(event, node)}
                className="cursor-pointer outline-none"
              >
                <title>
                  {node.label} · {node.entityTypeName}
                  {node.description ? ` · ${node.description}` : ""}
                </title>
                <circle
                  r={radius + 5}
                  fill="var(--background)"
                  fillOpacity="0.92"
                  stroke="var(--border)"
                  strokeWidth="1"
                />
                <circle
                  r={radius}
                  fill={root ? "var(--primary)" : "var(--card)"}
                  stroke={
                    node.status === "merged"
                      ? "var(--destructive)"
                      : root
                        ? "var(--primary)"
                        : "color-mix(in oklch, var(--primary), var(--border) 66%)"
                  }
                  strokeWidth={root ? 3 : 1.5}
                  filter="url(#node-shadow)"
                />
                <text
                  y={root ? 3 : 2}
                  textAnchor="middle"
                  fontSize={root ? 11 : 9}
                  fontWeight="650"
                  fill={root ? "var(--primary-foreground)" : "var(--foreground)"}
                  className="pointer-events-none select-none"
                >
                  {shortLabel(node.label, root ? 12 : 8)}
                </text>
                {!root ? (
                  <text
                    y={radius + 16}
                    textAnchor="middle"
                    fontSize="8"
                    fill="var(--muted-foreground)"
                    className="pointer-events-none select-none"
                  >
                    {shortLabel(node.entityTypeName, 10)}
                  </text>
                ) : null}
              </g>
            );
          })}
        </g>
      </svg>
      <div className="pointer-events-none absolute bottom-3 left-3 rounded-lg border bg-background/85 px-2.5 py-1.5 text-[10px] text-muted-foreground backdrop-blur">
        点击节点以它为新的中心 · 虚线边表示争议 Claim
      </div>
    </div>
  );
}

function EntityCandidate({
  entity,
  onSelect,
}: {
  entity: EntityCatalogItem;
  onSelect: (entity: EntityCatalogItem) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onSelect(entity)}
      className="flex w-full items-start gap-3 rounded-xl px-3 py-2.5 text-left hover:bg-accent"
    >
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-indigo-100 text-indigo-700">
        <Waypoints className="size-4" aria-hidden />
      </span>
      <span className="min-w-0">
        <span className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">
            {entity.displayLabel}
          </span>
          <span className="shrink-0 text-[10px] text-muted-foreground">
            {entity.entityType.name}
          </span>
        </span>
        <span className="block truncate font-mono text-[10px] text-muted-foreground">
          {entity.canonicalKey}
        </span>
      </span>
    </button>
  );
}

function EdgeRow({
  edge,
  nodes,
}: {
  edge: EntityGraphEdge;
  nodes: Map<string, EntityGraphNode>;
}) {
  const source = nodes.get(edge.subjectEntityId);
  const target = nodes.get(edge.targetEntityId);
  const verified =
    edge.verificationStatus === "human_verified" ||
    edge.verificationStatus === "ai_checked";
  return (
    <li className="rounded-xl border bg-card p-3">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Link
          href={`/entities/${edge.subjectEntityId}`}
          className="font-medium hover:text-primary"
        >
          {source?.label ?? edge.subjectEntityId}
        </Link>
        <span
          className="rounded-md px-2 py-1 font-medium"
          style={{
            backgroundColor: `color-mix(in oklch, ${propertyColor(edge.propertyKey)}, transparent 88%)`,
            color: propertyColor(edge.propertyKey),
          }}
        >
          {edge.propertyName}
        </span>
        <span aria-hidden>→</span>
        <Link
          href={`/entities/${edge.targetEntityId}`}
          className="font-medium hover:text-primary"
        >
          {target?.label ?? edge.targetEntityId}
        </Link>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2 text-[10px] text-muted-foreground">
        {verified ? (
          <span className="inline-flex items-center gap-1 text-emerald-700">
            <ShieldCheck className="size-3" aria-hidden />
            {edge.verificationStatus === "human_verified" ? "人工已核验" : "AI 已核对"}
          </span>
        ) : (
          <span>{edge.verificationStatus}</span>
        )}
        <span>·</span>
        <span>{edge.rank}</span>
        <span>·</span>
        <Link
          href={`/claims/${edge.claimId}`}
          className="inline-flex items-center gap-1 hover:text-primary"
        >
          Claim
          <ExternalLink className="size-2.5" aria-hidden />
        </Link>
      </div>
    </li>
  );
}

export function EntityGraphWorkspace({
  initialEntityId,
}: {
  initialEntityId: string;
}) {
  const router = useRouter();
  const session = useSession();
  const [entityId, setEntityId] = useState(initialEntityId);
  const [searchInput, setSearchInput] = useState("");
  const debouncedSearch = useDebouncedValue(searchInput.trim(), 220);
  const [direction, setDirection] = useState<Direction>("both");
  const [depth, setDepth] = useState(2);
  const [maxNodes, setMaxNodes] = useState(60);
  const [propertyKey, setPropertyKey] = useState("");
  const [rebuilding, setRebuilding] = useState(false);

  const candidates = useSWR(
    debouncedSearch.length >= 1
      ? ["entity-graph:candidates", debouncedSearch]
      : null,
    () =>
      knowledgeApi().listEntities({
        q: debouncedSearch,
        status: "active",
        pageSize: 12,
      }),
    { keepPreviousData: true },
  );

  const graph = useSWR(
    entityId
      ? [
          "entity-graph",
          entityId,
          direction,
          depth,
          maxNodes,
          propertyKey,
        ]
      : null,
    () =>
      knowledgeApi().getEntityGraph({
        id: entityId,
        direction,
        depth,
        maxNodes,
        propertyKey: propertyKey || undefined,
      }),
    { keepPreviousData: true },
  );

  const nodeById = useMemo(
    () => new Map((graph.data?.nodes ?? []).map((node) => [node.id, node])),
    [graph.data?.nodes],
  );
  const properties = useMemo(() => {
    const values = new Map<string, string>();
    for (const edge of graph.data?.edges ?? []) {
      values.set(edge.propertyKey, edge.propertyName);
    }
    if (propertyKey && !values.has(propertyKey)) values.set(propertyKey, propertyKey);
    return Array.from(values.entries()).sort((left, right) =>
      left[1].localeCompare(right[1], "zh-CN"),
    );
  }, [graph.data?.edges, propertyKey]);

  const chooseEntity = (entity: EntityCatalogItem) => {
    setEntityId(entity.id);
    setSearchInput("");
    setPropertyKey("");
    router.replace(`/explore/graph?entity_id=${encodeURIComponent(entity.id)}`);
  };

  const chooseNode = (node: EntityGraphNode) => {
    setEntityId(node.id);
    setPropertyKey("");
    router.replace(`/explore/graph?entity_id=${encodeURIComponent(node.id)}`);
  };

  const rebuild = async () => {
    if (!session.session) {
      router.push(
        `${LOGIN_PATH}?next=${encodeURIComponent(`/explore/graph?entity_id=${entityId}`)}`,
      );
      return;
    }
    setRebuilding(true);
    try {
      const result = await knowledgeApi().rebuildEntityGraph();
      await graph.mutate();
      toast.success(`关系图已重建：${result.subjects} 个主体，${result.edges} 条边`);
    } catch (error) {
      if (isUnauthorized(error)) {
        router.push(
          `${LOGIN_PATH}?next=${encodeURIComponent(`/explore/graph?entity_id=${entityId}`)}`,
        );
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("只有站点管理员可以全量重建关系图");
      } else {
        toast.error("关系图重建失败，请稍后重试");
      }
    } finally {
      setRebuilding(false);
    }
  };

  const rootNode = graph.data ? nodeById.get(graph.data.rootId) : undefined;

  return (
    <div className="space-y-5">
      <section className="grid gap-4 rounded-2xl border bg-card p-4 shadow-sm lg:grid-cols-[minmax(18rem,1fr)_auto] lg:items-end">
        <div className="relative">
          <label
            htmlFor="entity-graph-search"
            className="text-xs font-semibold text-muted-foreground"
          >
            选择中心 Entity
          </label>
          <div className="relative mt-2">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden
            />
            <Input
              id="entity-graph-search"
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder={
                rootNode
                  ? `当前：${rootNode.label}，输入以切换中心`
                  : "搜索名称、别名或 canonical key"
              }
              className="h-11 pl-9"
            />
          </div>
          {searchInput.trim() ? (
            <div className="absolute inset-x-0 top-full z-20 mt-2 max-h-80 overflow-y-auto rounded-xl border bg-popover p-1 shadow-xl">
              {candidates.isLoading ? (
                <p className="flex items-center gap-2 px-3 py-4 text-xs text-muted-foreground">
                  <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
                  查找 Entity…
                </p>
              ) : null}
              {candidates.data?.items.map((entity) => (
                <EntityCandidate
                  key={entity.id}
                  entity={entity}
                  onSelect={chooseEntity}
                />
              ))}
              {!candidates.isLoading &&
              candidates.data?.items.length === 0 ? (
                <p className="px-3 py-4 text-xs text-muted-foreground">
                  没有匹配的有效 Entity
                </p>
              ) : null}
            </div>
          ) : null}
        </div>

        <Button
          variant="outline"
          onClick={() => void rebuild()}
          disabled={rebuilding}
          className="lg:mb-0"
        >
          {rebuilding ? (
            <LoaderCircle className="size-4 animate-spin" aria-hidden />
          ) : (
            <RefreshCw className="size-4" aria-hidden />
          )}
          重建边投影
        </Button>
      </section>

      <section className="grid gap-4 rounded-2xl border bg-card p-4 shadow-sm md:grid-cols-2 xl:grid-cols-[1.3fr_0.7fr_0.8fr_1fr]">
        <div>
          <p className="text-xs font-semibold text-muted-foreground">遍历方向</p>
          <div className="mt-2 grid grid-cols-3 gap-1 rounded-xl bg-muted p-1">
            {(Object.keys(DIRECTION_META) as Array<Direction>).map((value) => {
              const meta = DIRECTION_META[value];
              const Icon = meta.icon;
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => setDirection(value)}
                  className={cn(
                    "rounded-lg px-2 py-2 text-left transition-colors",
                    direction === value
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                  title={meta.detail}
                >
                  <span className="flex items-center gap-1.5 text-xs font-medium">
                    <Icon className="size-3.5" aria-hidden />
                    {meta.label}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
        <label className="block">
          <span className="text-xs font-semibold text-muted-foreground">
            深度
          </span>
          <select
            value={depth}
            onChange={(event) => setDepth(Number(event.target.value))}
            className="mt-2 h-11 w-full rounded-xl border bg-background px-3 text-sm"
          >
            <option value={1}>1 层 · 直接关系</option>
            <option value={2}>2 层 · 邻接网络</option>
            <option value={3}>3 层 · 扩展探索</option>
          </select>
        </label>
        <label className="block">
          <span className="text-xs font-semibold text-muted-foreground">
            Property
          </span>
          <select
            value={propertyKey}
            onChange={(event) => setPropertyKey(event.target.value)}
            className="mt-2 h-11 w-full rounded-xl border bg-background px-3 text-sm"
          >
            <option value="">全部关系</option>
            {properties.map(([key, name]) => (
              <option key={key} value={key}>
                {name} · {key}
              </option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="flex items-center justify-between text-xs font-semibold text-muted-foreground">
            节点上限
            <span className="font-mono tabular-nums">{maxNodes}</span>
          </span>
          <input
            type="range"
            min="10"
            max="100"
            step="10"
            value={maxNodes}
            onChange={(event) => setMaxNodes(Number(event.target.value))}
            className="mt-4 w-full accent-primary"
          />
        </label>
      </section>

      {!entityId ? (
        <section className="rounded-3xl border border-dashed bg-muted/20 px-6 py-20 text-center">
          <span className="mx-auto flex size-14 items-center justify-center rounded-2xl bg-indigo-100 text-indigo-700">
            <Network className="size-6" aria-hidden />
          </span>
          <h2 className="mt-5 text-xl font-semibold">选择一个 Entity 开始</h2>
          <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
            关系图中的每条有向边都对应一个已发布 Claim；点击节点可以连续换中心探索。
          </p>
        </section>
      ) : null}

      {graph.isLoading && !graph.data ? (
        <section className="flex min-h-[36rem] items-center justify-center rounded-3xl border">
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" aria-hidden />
            正在展开关系图…
          </p>
        </section>
      ) : null}

      {graph.error ? (
        <section className="flex gap-3 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          <div>
            <p className="font-medium">关系图读取失败</p>
            <p className="mt-1 text-xs opacity-85">
              Entity 可能不存在，或边投影尚未完成；管理员可尝试重建投影。
            </p>
          </div>
        </section>
      ) : null}

      {graph.data ? (
        <>
          <section className="rounded-3xl border bg-card p-3 shadow-[0_16px_50px_rgb(15_23_42/0.05)]">
            <div className="flex flex-wrap items-center justify-between gap-3 px-2 pb-3">
              <div>
                <h2 className="flex items-center gap-2 text-sm font-semibold">
                  <GitBranch className="size-4 text-indigo-600" aria-hidden />
                  {rootNode?.label ?? graph.data.rootId}
                </h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  {graph.data.nodes.length} 个节点 · {graph.data.edges.length} 条边 ·
                  到达 {graph.data.reachedDepth} 层
                </p>
              </div>
              <div className="flex items-center gap-2">
                {graph.data.truncated ? (
                  <span className="rounded-full bg-amber-100 px-2.5 py-1 text-[10px] font-medium text-amber-800">
                    已达到安全上限
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2.5 py-1 text-[10px] font-medium text-emerald-800">
                    <CheckCircle2 className="size-3" aria-hidden />
                    完整子图
                  </span>
                )}
                {graph.isValidating ? (
                  <LoaderCircle
                    className="size-3.5 animate-spin text-muted-foreground"
                    aria-hidden
                  />
                ) : null}
              </div>
            </div>
            {graph.data.nodes.length > 0 ? (
              <GraphCanvas graph={graph.data} onChooseRoot={chooseNode} />
            ) : null}
          </section>

          <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
            <section>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold">关系明细</h2>
                <span className="text-xs text-muted-foreground">
                  Claim 是边的可追溯来源
                </span>
              </div>
              {graph.data.edges.length > 0 ? (
                <ul className="grid gap-2 lg:grid-cols-2">
                  {graph.data.edges.map((edge) => (
                    <EdgeRow
                      key={edge.claimId}
                      edge={edge}
                      nodes={nodeById}
                    />
                  ))}
                </ul>
              ) : (
                <div className="rounded-2xl border border-dashed px-5 py-10 text-center text-sm text-muted-foreground">
                  当前筛选下没有已发布的 Entity 关系。
                </div>
              )}
            </section>

            <aside className="rounded-2xl border bg-card p-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold">
                <Sparkles className="size-4 text-violet-600" aria-hidden />
                投影状态
              </h2>
              <dl className="mt-4 space-y-3 text-xs">
                <div className="flex justify-between gap-3">
                  <dt className="text-muted-foreground">查询方向</dt>
                  <dd>{DIRECTION_META[graph.data.direction].label}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-muted-foreground">请求 / 到达深度</dt>
                  <dd>
                    {graph.data.requestedDepth} / {graph.data.reachedDepth}
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-muted-foreground">节点 / 边</dt>
                  <dd>
                    {graph.data.nodes.length} / {graph.data.edges.length}
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-muted-foreground">最近投影</dt>
                  <dd className="text-right">
                    {graph.data.projectionUpdatedAt
                      ? DATE_FORMATTER.format(graph.data.projectionUpdatedAt)
                      : "暂无边"}
                  </dd>
                </div>
              </dl>
              {rootNode ? (
                <Button variant="outline" className="mt-5 w-full" asChild>
                  <Link href={`/entities/${rootNode.id}`}>
                    <Braces className="size-4" aria-hidden />
                    查看 Entity 详情
                  </Link>
                </Button>
              ) : null}
            </aside>
          </div>
        </>
      ) : null}
    </div>
  );
}
