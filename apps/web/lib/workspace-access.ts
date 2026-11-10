import type { AuthSession } from "../../../contracts/generated/typescript";

const WIKI_ROLES = ["editor", "reviewer", "applier", "admin"] as const;

const WORKSPACES = [
  { path: "/imports", title: "导入资料", homeOnly: true },
  { path: "/governance/proposals/new", title: "新建人工修改建议", homeOnly: true },
  { path: "/components", title: "组件中心", homeOnly: true },
  { path: "/assets", title: "媒体与附件", homeOnly: true },
  { path: "/sources", title: "来源与证据", homeOnly: true },
  { path: "/entities", title: "实体与知识", homeOnly: true },
  { path: "/federation", title: "跨 Wiki 关联", homeOnly: true },
  { path: "/governance/fact-check", title: "事实一致性检查", homeOnly: true },
  { path: "/account", title: "个人中心", homeOnly: true },
  { path: "/settings/cli", title: "Agent CLI 授权", homeOnly: true },
  { path: "/new", title: "创建页面", roles: ["editor", "admin"], permission: "编辑者或管理员" },
  { path: "/collections", title: "专题合集", roles: WIKI_ROLES, permission: "Wiki 成员" },
  { path: "/governance/review", title: "待审核", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/bulk", title: "批量审核", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/activity", title: "操作记录与标签", roles: ["reviewer", "admin"], permission: "审核者或管理员" },
  { path: "/governance/apply", title: "待生效", roles: ["applier", "admin"], permission: "发布者或管理员" },
  { path: "/governance/protections", title: "页面保护策略", roles: ["admin"], permission: "管理员" },
  { path: "/governance/ai-trust", title: "AI 审核设置", roles: ["admin"], permission: "管理员" },
  { path: "/admin/members", title: "成员与角色", roles: ["admin"], permission: "管理员" },
  { path: "/admin/users", title: "成员与角色", roles: ["admin"], permission: "管理员" },
  { path: "/governance/revision-storage", title: "历史版本存储", homeOnly: true, roles: ["admin"], permission: "管理员", accountScope: true },
] as const;

export function workspaceAccessNotice(href: string, session: AuthSession | undefined) {
  const pathname = href.split(/[?#]/, 1)[0];
  const workspace = WORKSPACES.find(
    ({ path }) => pathname === path || pathname.startsWith(`${path}/`),
  );
  if (!workspace) return undefined;
  if (!session) {
    return { title: workspace.title, heading: "需要登录", message: "请先登录，再使用此功能。" };
  }
  if (session.actorType === "system") return undefined;
  if ("homeOnly" in workspace && session.homeWikiId !== session.wikiId) {
    return {
      title: workspace.title,
      heading: "请切换到自己的 Wiki",
      message: `“${workspace.title}”仅用于你自己的 Wiki，请先切换后再使用。`,
    };
  }
  if (!("roles" in workspace)) return undefined;
  const wikiId = "accountScope" in workspace ? session.homeWikiId : session.wikiId;
  const roles = session.wikis.find((wiki) => wiki.wikiId === wikiId)?.roles ?? [];
  if (workspace.roles.some((role) => roles.includes(role))) return undefined;
  return {
    title: workspace.title,
    heading: `需要${workspace.permission}权限`,
    message: "accountScope" in workspace
      ? "需要你自己的 Wiki 的管理员权限，才能查看存储用量或归档历史版本。"
      : `当前账户没有此 Wiki 的${workspace.permission}权限，无法访问${workspace.title}。`,
  };
}
