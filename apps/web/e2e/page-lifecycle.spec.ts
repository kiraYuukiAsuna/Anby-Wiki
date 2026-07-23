import { expect, test, type Page } from "@playwright/test";

const ACTOR_ID = "00000000-0000-7000-8000-000000000201";
const TEST_AUTH_HEADERS = { "X-Actor-ID": ACTOR_ID };

async function createPage(page: Page, title: string): Promise<string> {
  await page.goto("/new");
  await page.getByLabel("页面标题").fill(title);
  await page.getByRole("button", { name: "创建并编辑" }).click();
  await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/edit$/);
  const match = new URL(page.url()).pathname.match(/^\/pages\/([^/]+)\/edit$/);
  if (!match) throw new Error(`无法从编辑路由解析 Page ID：${page.url()}`);
  return match[1];
}

async function publish(page: Page, summary: string) {
  await page.getByRole("button", { name: "发布…" }).click();
  const dialog = page.getByRole("dialog", { name: "发布修改" });
  await dialog.getByLabel("修改摘要").fill(summary);
  await dialog.getByRole("button", { name: "确认发布" }).click();
  await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+$/);
}

test("MT-M2-BROWSER-LIFECYCLE：创建、编辑、链接、改名、冲突、历史与回滚", async ({
  browser,
  page,
}) => {
  const suffix = `${Date.now()}-${test.info().workerIndex}`;
  const targetTitle = `M2 E2E 引用目标 ${suffix}`;
  const originalTitle = `M2 E2E 页面 ${suffix}`;
  const renamedTitle = `${originalTitle} 已改名`;
  const initialSummary = "M2 E2E 初次发布";

  // 引用目标同样只经浏览器创建，测试不调用内部 API 或写手工 AST。
  await createPage(page, targetTitle);
  const pageId = await createPage(page, originalTitle);

  const editor = page.locator(".bn-editor[contenteditable='true']");
  await editor.click();
  await page.keyboard.type("这是一段通过浏览器输入的正文。");
  await page.getByRole("button", { name: "插入标题" }).click();
  await page.getByRole("button", { name: "插入列表" }).click();
  await page.getByRole("button", { name: "插入表格" }).click();

  await page.getByRole("button", { name: /页面引用/ }).click();
  await page.getByPlaceholder("输入页面标题搜索…").fill(targetTitle);
  await page.getByText(targetTitle, { exact: true }).click();
  await page.getByLabel("显示文本").fill("内部引用目标");
  await page.getByRole("button", { name: "插入引用" }).click();

  await publish(page, initialSummary);
  await expect(page.getByRole("heading", { level: 1, name: originalTitle })).toBeVisible();
  await expect(page.getByText("这是一段通过浏览器输入的正文。")).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "章节标题" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "列表项" })).toBeVisible();
  await expect(page.getByRole("cell", { name: "单元格 1" })).toBeVisible();
  await expect(page.getByRole("link", { name: "内部引用目标" })).toHaveAttribute(
    "href",
    expect.stringContaining("/pages/"),
  );

  // 改名保留旧标题别名，Page ID 不变。
  await page.getByRole("button", { name: "改名" }).click();
  const renameDialog = page.getByRole("dialog", { name: "页面改名" });
  await renameDialog.getByLabel("新标题").fill(renamedTitle);
  await renameDialog.getByRole("button", { name: "确认改名" }).click();
  await expect(page.getByRole("heading", { level: 1, name: renamedTitle })).toBeVisible();
  await page.goto(`/wiki/${encodeURIComponent(originalTitle)}`);
  await expect(page.getByRole("status")).toContainText("已移动至");
  await expect(page.getByRole("heading", { level: 1, name: renamedTitle })).toBeVisible();

  // 两个完全独立的浏览器上下文同时基于同一 Revision 编辑。
  const contextA = await browser.newContext({ extraHTTPHeaders: TEST_AUTH_HEADERS });
  const contextB = await browser.newContext({ extraHTTPHeaders: TEST_AUTH_HEADERS });
  const pageA = await contextA.newPage();
  const pageB = await contextB.newPage();
  try {
    await Promise.all([
      pageA.goto(`/pages/${pageId}/edit`),
      pageB.goto(`/pages/${pageId}/edit`),
    ]);

    await pageA.getByRole("button", { name: "插入标题" }).click();
    await pageA.getByText("章节标题", { exact: true }).last().click();
    await pageA.keyboard.press("End");
    await pageA.keyboard.type(" 并发窗口 A");

    await pageB.getByRole("button", { name: "插入列表" }).click();
    await pageB.getByText("列表项", { exact: true }).last().click();
    await pageB.keyboard.press("End");
    await pageB.keyboard.type(" 并发窗口 B");

    await publish(pageA, "并发窗口 A 发布");

    await pageB.getByRole("button", { name: "发布…" }).click();
    const publishB = pageB.getByRole("dialog", { name: "发布修改" });
    await publishB.getByLabel("修改摘要").fill("并发窗口 B 发布");
    await publishB.getByRole("button", { name: "确认发布" }).click();

    const conflict = pageB
      .getByRole("alert")
      .filter({ hasText: "页面已被他人更新" });
    await expect(conflict).toContainText("页面已被他人更新");
    await expect(conflict).toContainText("你的编辑内容已保留");
    await conflict.getByRole("button", { name: "查看差异" }).click();
    await expect(pageB.getByRole("dialog", { name: /版本差异/ })).toBeVisible();
    await pageB.keyboard.press("Escape");
    await conflict.getByRole("button", { name: "以最新版为基继续编辑" }).click();
    await publish(pageB, "并发窗口 B 发布");
    await expect(pageB.getByText("列表项 并发窗口 B")).toBeVisible();

    // 历史页面可对比版本，并将初次发布快照追加为一个新的回滚 Revision。
    await pageB.getByRole("link", { name: "查看历史" }).click();
    const currentRow = pageB.locator("li").filter({ hasText: "当前版本" });
    await currentRow.getByRole("link", { name: "与上一版对比" }).click();
    await expect(pageB.getByRole("heading", { name: /版本对比/ })).toBeVisible();
    await pageB.getByRole("link", { name: "返回历史" }).click();

    const initialRow = pageB.locator("li").filter({ hasText: initialSummary });
    await initialRow.getByRole("button", { name: "回滚" }).click();
    await pageB
      .getByRole("dialog", { name: "回滚到此版本" })
      .getByRole("button", { name: "确认回滚" })
      .click();
    await expect(pageB).toHaveURL(`/pages/${pageId}`);
    await expect(pageB.getByText("这是一段通过浏览器输入的正文。")).toBeVisible();
    await expect(pageB.getByText("并发窗口 A")).toHaveCount(0);
    await expect(pageB.getByText("并发窗口 B")).toHaveCount(0);
  } finally {
    await contextA.close();
    await contextB.close();
  }
});
