import type {
  Collection,
  CollectionDynamicQuery,
  CollectionQuery,
  CollectionRule,
} from "../../../contracts/generated/typescript";

export function isDynamicCollectionQuery(
  query: CollectionQuery | null | undefined,
): query is CollectionDynamicQuery {
  return Boolean(query && "memberType" in query);
}

export function isRuleCollectionQuery(
  query: CollectionQuery | null | undefined,
): query is CollectionRule {
  return Boolean(query && "kind" in query);
}

export function collectionSummary(collection: Collection): string {
  if (collection.collectionType === "manual" || !collection.query) {
    return "手动添加合集成员";
  }
  if (isDynamicCollectionQuery(collection.query)) {
    const target = collection.query.memberType === "page" ? "页面" : "实体";
    const conditions = [
      collection.query.text ? `关键词“${collection.query.text}”` : "",
      collection.query.namespace
        ? `命名空间 ${collection.query.namespace}`
        : "",
      collection.query.entityType
        ? `类型 ${collection.query.entityType}`
        : "",
      collection.query.property ? `存在属性 ${collection.query.property}` : "",
    ].filter(Boolean);
    return `实时查询${target}${conditions.length ? ` · ${conditions.join(" · ")}` : ""}`;
  }
  if (isRuleCollectionQuery(collection.query)) {
    return collection.query.kind === "entity_type"
      ? `按实体类型筛选： ${collection.query.entityType ?? "未知"}`
      : `包含已发布属性：${collection.query.property ?? "未知"}`;
  }
  return "按保存的条件查询";
}
