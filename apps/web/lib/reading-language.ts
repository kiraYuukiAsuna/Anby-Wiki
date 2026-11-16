import type { PageLanguageVariant } from "../../../contracts/generated/typescript";

export function preferredPageVariant(
  items: PageLanguageVariant[],
  currentPageId: string,
  language?: string,
  redirectSourceId?: string,
): PageLanguageVariant | undefined {
  return items.find((item) =>
    item.language.split("-")[0] === language?.split("-")[0] &&
    item.pageId !== currentPageId && item.pageId !== redirectSourceId,
  );
}
