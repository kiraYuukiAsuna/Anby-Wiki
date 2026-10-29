import type { AuthSession } from "../../../contracts/generated/typescript";

const WIKI_ROLES = ["editor", "reviewer", "applier", "admin"] as const;

const WORKSPACES = [
  { path: "/imports", title: "导入中心", homeOnly: true },
  { path: "/governance/proposals/new", title: "新建人工提案", homeOnly: true },
  { path: "/new", title: "创建页面", roles: ["editor", "admin"], permission: "编辑者或管理员" },
  { path: "/collections", title: "专题合集", roles: WIKI_ROLES, permission: "Wiki 成员" },
  { path: "/governance/review", title: "人工审核队列", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/bulk", title: "批量审核", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/activity", title: "审计与变更标签", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/apply", title: "待应用", roles: ["applier", "admin"], permission: "应用者或管理员" },
  { path: "/governance/protections", title: "页面保护策略", roles: ["admin"], permission: "管理员" },
  { path: "/governance/ai-trust", title: "AI 信任与人工抽样", roles: ["admin"], permission: "管理员" },
  { path: "/admin/members", title: "成员与角色", roles: ["admin"], permission: "管理员" },
  { path: "/admin/users", title: "成员与角色", roles: ["admin"], permission: "管理员" },
  { path: "/governance/revision-storage", title: "历史版本存储", roles: ["admin"], permission: "管理员", accountScope: true },
] as const;

export function workspaceAccessNotice(href: string, session: AuthSession | undefined) {
  const pathname = href.split(/[?#]/, 1)[0];
  const workspace = WORKSPACES.find(
    ({ path }) => pathname === path || pathname.startsWith(`${path}/`),
  );
  if (!workspace) return undefined;
  if (!session) {
    return { title: workspace.title, heading: "需要登录", message: "登录后才能访问该工作台。" };
  }
  if (session.actorType === "system") return undefined;
  if ("homeOnly" in workspace) {
    if (session.homeWikiId === session.wikiId) return undefined;
    return {
      title: workspace.title,
      heading: "请切换到自己的 Wiki",
      message: `${workspace.title}仅支持当前账户自己的 Wiki。当前选中的是协作 Wiki，请切换到自己的 Wiki 后使用。`,
    };
  }
  const wikiId = "accountScope" in workspace ? session.homeWikiId : session.wikiId;
  const roles = session.wikis.find((wiki) => wiki.wikiId === wikiId)?.roles ?? [];
  if (workspace.roles.some((role) => roles.includes(role))) return undefined;
  return {
    title: workspace.title,
    heading: `需要${workspace.permission}权限`,
    message: "accountScope" in workspace
      ? "需要当前账户 Wiki 的管理员权限才能查看容量统计和执行手动归档。"
      : `当前账户没有此 Wiki 的${workspace.permission}权限，无法访问${workspace.title}。`,
  };
}
