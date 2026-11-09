// Start `npm run dev -- --port 3101`; use an installed Playwright via PLAYWRIGHT_MODULE if needed.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3101';
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addCookies([{ name: 'anby_session', value: 'editor-fixture', url: baseURL }]);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== baseURL) return route.fulfill({ body: '' });
    if (!url.pathname.startsWith('/api/')) return route.continue();
    if (url.pathname.endsWith('/auth/session')) return route.fulfill({ json: {
      actor_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01',
      wiki_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02',
      home_wiki_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02',
      main_namespace_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b03',
      wiki_name: 'Editor fixture', actor_type: 'human', display_name: 'Editor fixture', method: 'session', wikis: [],
    } });
    return route.fulfill({ json: { items: [] } });
  });
  const check = async (name, callback) => { await callback(); console.log(`PASS ${name}`); };
  const eventually = async (callback) => {
    let lastError;
    for (let i = 0; i < 60; i++) {
      try { await callback(); return; } catch (error) { lastError = error; }
      await page.waitForTimeout(50);
    }
    throw lastError;
  };
  const ast = async () => JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const editor = page.locator('[data-block-editor]');
  const select = async (locator) => locator.evaluate((node) => {
    node.closest('[contenteditable="true"]').focus();
    const range = document.createRange(); range.selectNodeContents(node);
    const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
    document.dispatchEvent(new Event('selectionchange'));
  });
  const bounds = (locator) => locator.evaluateAll((nodes) => nodes.map((node) => {
    const { x, y, width } = node.getBoundingClientRect(); return { x, y, width };
  }));
  const empty = async () => { await page.getByRole('button', { name: '空白文档' }).click(); await editor.locator('.tiptap').waitFor(); await editor.locator('.tiptap').click(); };
  await page.goto(`${baseURL}/dev/editor`, { waitUntil: 'domcontentloaded' });
  await editor.locator('.tiptap').waitFor();
  await check('media uses reader previews and properties start collapsed', async () => {
    await editor.locator('.ast-editor-preview audio').waitFor();
    assert.equal(await editor.locator('details input:visible, details textarea:visible').count(), 0);
    assert.ok(await editor.locator('.ast-editor-preview iframe').count() >= 2);
    assert.match(await editor.locator('[data-editor-block-type="preformatted"] .ast-editor-preview').innerText(), /\+---\+/);
  });
  await check('table cells and weighted columns render horizontally', async () => {
    const cells = await bounds(editor.locator('.ast-table-cell'));
    const editorColor = await editor.locator('[data-text-color=red]').first().evaluate(n => getComputedStyle(n).color);
    const readerColor = await page.locator('[data-ast-document] [data-text-color=red]').first().evaluate(n => getComputedStyle(n).color);
    assert.equal(editorColor, readerColor);
    const editorBackground = await editor.locator('[data-background-color=yellow]').first().evaluate(n => getComputedStyle(n).backgroundColor);
    const readerBackground = await page.locator('[data-ast-document] [data-background-color=yellow]').first().evaluate(n => getComputedStyle(n).backgroundColor);
    assert.equal(editorBackground, readerBackground);
    assert.equal(cells[0].y, cells[1].y); assert.ok(cells[1].x > cells[0].x);
    const columns = await bounds(editor.locator('[data-editor-block-type="column"]'));
    assert.equal(columns[0].y, columns[1].y);
    assert.ok(Math.abs(columns[1].width / columns[0].width - 2) < 0.05);
  });
  await check('repeated table edits preserve cell text and heading hierarchy', async () => {
    await editor.locator('.ast-table-cell p').first().fill('编辑后的单元格');
    const table = editor.locator('[data-editor-block-type="astTable"]');
    await table.locator('summary').click();
    await table.getByRole('button', { name: '增加一行' }).click();
    await table.getByRole('button', { name: '增加一行' }).click();
    await table.getByRole('button', { name: '增加一列' }).click();
    const saved = await ast(); const tableAst = saved.children.find((value) => value.type === 'table');
    assert.equal(tableAst.children.length, 3);
    assert.ok(tableAst.children.every((row) => row.children.length === 3));
    assert.equal(tableAst.children[0].children[0].children[0].content[0].text, '编辑后的单元格');
    assert.equal(saved.children[0].is_toggleable, true);
    assert.equal(saved.children[0].children[0].content[0].text, '可折叠标题正文');
    await table.locator('summary').click();
  });
  await check('toggle summaries are directly editable and survive reopening', async () => {
    const toggle = editor.locator('[data-editor-block-type="toggleBlock"]');
    await toggle.locator('p').first().fill('修改后的折叠摘要');
    assert.equal((await ast()).children.find(b => b.type === 'toggle').content[0].text, '修改后的折叠摘要');
    await toggle.getByRole('button', { name: '折叠内容' }).click();
    assert.equal(await toggle.locator('p').nth(1).isVisible(), false);
    await toggle.getByRole('button', { name: '展开内容' }).click();
    const saved = await ast();
    await page.getByRole('button', { name: '重新载入当前内容' }).click();
    await editor.locator('.tiptap').waitFor();
    assert.match(await editor.locator('[data-editor-block-type="toggleBlock"]').innerText(), /修改后的折叠摘要/);
    await editor.locator('.ast-table-cell p').first().fill('编辑后的单元格');
    assert.deepEqual(await ast(), saved);
  });
  await check('property validation rejects invalid data and keeps editing usable', async () => {
    const column = editor.locator('[data-editor-block-type="column"]').first();
    await column.locator('summary').click();
    await column.getByLabel('宽度比例').fill('0');
    await column.getByLabel('宽度比例').blur();
    assert.equal((await ast()).children.find(b => b.type === 'column_list').children[0].width_ratio, 1);
    assert.equal(await column.getByLabel('宽度比例').inputValue(), '1');
    await column.getByLabel('宽度比例').fill('1.5'); await column.getByLabel('宽度比例').blur();
    assert.equal((await ast()).children.find(b => b.type === 'column_list').children[0].width_ratio, 1.5);
    await column.locator('summary').click();
  });
  await check('column controls stop at the supported maximum', async () => {
    const columns = editor.locator('[data-editor-block-type="columnList"]');
    await columns.locator(':scope > details > summary').click();
    for (let i = 0; i < 3; i++) await columns.getByRole('button', { name: '增加一栏' }).click();
    assert.equal((await ast()).children.find(b => b.type === 'column_list').children.length, 5);
    assert.equal(await columns.getByRole('button', { name: '增加一栏' }).isDisabled(), true);
    await columns.locator(':scope > details > summary').click();
  });
  await check('inline code formatting survives serialization', async () => {
    await select(editor.locator('.ast-inline-code').first());
    await page.keyboard.press('Meta+b');
    await eventually(async () => assert.ok((await ast()).children.some(b => b.content?.some(n => n.text === 'inline_code' && n.marks?.includes('bold') && n.marks?.includes('code')))));
  });
  await check('inline reference edits retain the target', async () => {
    const ref = editor.locator('.ast-page-ref').first();
    await ref.hover();
    await ref.getByLabel('显示文本', { exact: true }).fill('更新的引用名称');
    await ref.getByLabel('显示文本', { exact: true }).blur();
    const reference = (await ast()).children.flatMap(b => b.content ?? []).find(n => n.display_text === '更新的引用名称');
    assert.equal(reference.target_page_id, '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01');
  });
  await check('preview buttons cannot execute page actions', async () => {
    await editor.locator('.ast-editor-preview').getByRole('button', { name: '运行操作' }).click();
    assert.equal(await page.getByRole('dialog').count(), 0); assert.equal(page.url(), `${baseURL}/dev/editor`);
  });
  await check('Shift+Enter, paragraph splitting, undo and redo keep unique IDs', async () => {
    await empty(); await page.keyboard.insertText('第一行'); await page.keyboard.press('Shift+Enter'); await page.keyboard.insertText('第二行');
    assert.match(JSON.stringify(await ast()), /\\n/);
    assert.equal(await page.locator('[data-ast-document] p').first().evaluate(n => getComputedStyle(n).whiteSpace), 'pre-wrap');
    await page.keyboard.press('Enter'); await page.keyboard.insertText('新段落');
    const saved = await ast(); assert.equal(saved.children.length, 2); assert.notEqual(saved.children[0].id, saved.children[1].id);
    await page.keyboard.press('Meta+z'); await page.keyboard.press('Meta+Shift+z');
    assert.deepEqual(await ast(), saved);
  });
  await check('pasting a copied block preserves the original ID and assigns the copy a new ID', async () => {
    await empty(); await page.keyboard.insertText('插入位置'); await page.keyboard.press('Enter'); await page.keyboard.insertText('原始段落');
    const original = (await ast()).children[1].id;
    await select(editor.locator('.tiptap > p').first());
    await editor.locator('.tiptap').evaluate((node, id) => {
      const data = new DataTransfer();
      data.setData('text/html', `<p data-id="${id}">复制段落</p><p data-id="${id}">第二份</p>`);
      data.setData('text/plain', '复制段落');
      node.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
    }, original);
    const saved = await ast();
    const ids = []; const visit = blocks => blocks.forEach(b => { ids.push(b.id); if (b.children) visit(b.children); }); visit(saved.children);
    assert.ok(ids.length >= 3); assert.equal(ids.length, new Set(ids).size);
    assert.ok(saved.children.some(b => b.id === original && b.content.some(n => n.text?.includes('原始段落'))));
  });
  await check('slash insertion focuses new editable content', async () => {
    await empty(); await page.keyboard.insertText('/table');
    await page.getByRole('button', { name: /语义表格/ }).click();
    await page.keyboard.insertText('插入后直接输入');
    assert.match(JSON.stringify((await ast()).children.find(b => b.type === 'table')), /插入后直接输入/);
  });
  await check('editor shortcut opens only the page picker and respects IME', async () => {
    await page.keyboard.press('Meta+k');
    assert.equal(await page.getByRole('dialog').count(), 1);
    await page.keyboard.press('Escape');
    await eventually(async () => assert.equal(await page.getByRole('dialog').count(), 0));
    await editor.locator('.tiptap').focus();
    await editor.locator('.tiptap').dispatchEvent('keydown', { key: 'k', metaKey: true, isComposing: true, bubbles: true });
    assert.equal(await page.getByRole('dialog').count(), 0);
  });
  await check('undoing local input retains remote content', async () => {
    await empty(); await page.keyboard.insertText('本地输入');
    await page.getByRole('button', { name: '模拟远端追加' }).click();
    await editor.locator('.tiptap p').first().click(); await page.keyboard.press('Meta+z');
    const saved = JSON.stringify(await ast());
    assert.match(saved, /远端新增内容/); assert.doesNotMatch(saved, /本地输入/);
  });
  await check('mobile editing has no page-wide horizontal overflow', async () => {
    await page.reload(); await editor.locator('.tiptap').waitFor();
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
      const columns = await bounds(editor.locator('[data-editor-block-type="column"]'));
      assert.ok(columns[1].y > columns[0].y);
    }
  });
  assert.deepEqual(errors, []); console.log('PASS no browser runtime errors');
} finally { await browser.close(); }
