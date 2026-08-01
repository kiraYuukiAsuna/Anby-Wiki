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
    return "由维护者明确选择成员";
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
      ? `物化规则 · EntityType = ${collection.query.entityType ?? "未知"}`
      : `物化规则 · 存在 published Claim：${collection.query.property ?? "未知"}`;
  }
  return "版本化集合查询";
}
