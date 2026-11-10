// Run /dev/editor with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, hasTouch: true });
  await context.addCookies([{ name: 'anby_session', value: 'editor-fixture', url: baseURL }]);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== baseURL) return route.fulfill({ body: '' });
    if (!url.pathname.startsWith('/api/')) return route.continue();
    if (url.pathname.endsWith('/auth/session')) return route.fulfill({ json: {
      actor_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01', wiki_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02',
      home_wiki_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02', main_namespace_id: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b03',
      wiki_name: '测试 Wiki', actor_type: 'human', display_name: '编辑者', method: 'session', wikis: [],
    } });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`${baseURL}/dev/editor`);
  const editor = page.locator('[data-block-editor]').first();
  const root = editor.locator('.tiptap');
  await root.waitFor();
  await editor.locator('.ast-table-cell').first().click({ button: 'right' });
  await page.getByRole('menuitem', { name: '在上方插入行', exact: true }).waitFor({ timeout: 5000 });
  await page.keyboard.press('Escape');
  const ast = async () => JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const check = async (name, fn) => { await fn(); console.log(`PASS ${name}`); };
  const wait = async fn => { let last; for (let n = 0; n < 80; n++) { try { await fn(); return; } catch (error) { last = error; } await page.waitForTimeout(50); } throw last; };
  const id = n => `0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12, '0')}`;
  const table = (base = 100, rows = 2, columns = 3) => {
    let n = base;
    const attrs = () => ({ id: id(n++) });
    return { type: 'astTable', attrs: { ...attrs(), caption: '测试表格' }, content: Array.from({ length: rows }, (_, row) => ({
      type: 'astTableRow', attrs: attrs(), content: Array.from({ length: columns }, (_, column) => ({
        type: 'astTableCell', attrs: attrs(), content: [{ type: 'paragraph', attrs: attrs(), content: [{ type: 'text', text: `${String.fromCharCode(65 + row)}${column + 1}`, marks: [{ type: 'bold' }] }] }],
      })),
    })) };
  };
  const set = async content => {
    await page.getByRole('button', { name: '空白文档' }).click(); await root.waitFor();
    await root.evaluate((el, content) => el.editor.commands.setContent({ type: 'doc', content }), content);
  };
  const tableAst = async () => (await ast()).children.find(node => node.type === 'table');
  const matrix = value => value.children.map(row => row.children.map(cell => cell.children[0]?.content?.map(part => part.text ?? '').join('') ?? ''));
  const cells = () => editor.locator('.ast-table-cell');
  const menu = () => page.getByRole('menu');
  const contextAction = async (index, name) => { await cells().nth(index).click({ button: 'right' }); await menu().getByRole('menuitem', { name, exact: true }).click(); await wait(async () => assert.equal(await root.evaluate(el => document.activeElement === el), true)); };
  const caret = async locator => locator.evaluate(node => {
    node.closest('[contenteditable="true"]').focus();
    const range = document.createRange(); range.selectNodeContents(node); range.collapse(false);
    const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
    document.dispatchEvent(new Event('selectionchange'));
  });
  const ids = doc => { const values = []; const visit = item => { if (item.id) values.push(item.id); (item.children ?? []).forEach(visit); }; visit(doc); return values; };
  const undo = () => root.evaluate(el => el.editor.commands.undo());
  const redo = () => root.evaluate(el => el.editor.commands.redo());
  await check('four insertion directions target the clicked cell and preserve content, IDs and undo', async () => {
    for (const [name, expected] of [
      ['在上方插入行', [['A1','A2','A3'],['','',''],['B1','B2','B3']]],
      ['在下方插入行', [['A1','A2','A3'],['B1','B2','B3'],['','','']]],
      ['在左侧插入列', [['A1','','A2','A3'],['B1','','B2','B3']]],
      ['在右侧插入列', [['A1','A2','','A3'],['B1','B2','','B3']]],
    ]) {
      await set([table()]); const before = await ast();
      await caret(cells().first().locator('p'));
      await contextAction(4, name);
      await wait(async () => assert.deepEqual(matrix(await tableAst()), expected));
      const after = await ast(); const allIds = ids(after);
      assert.equal(new Set(allIds).size, allIds.length);
      for (const value of ids(before)) assert(allIds.includes(value));
      const oldCells = before.children[0].children.flatMap(row => row.children);
      const newCells = after.children[0].children.flatMap(row => row.children);
      for (const cell of oldCells) assert.deepEqual(newCells.find(item => item.id === cell.id), cell);
      await wait(async () => assert.equal(await root.evaluate(el => document.activeElement === el), true));
      await undo(); assert.deepEqual(await ast(), before);
      await redo(); assert.deepEqual(await ast(), after);
    }
  });
  await check('insertion focuses its new cell and reloading preserves typed content', async () => {
    await set([table()]); await contextAction(4, '在上方插入行');
    await page.keyboard.insertText('新增内容');
    await wait(async () => assert.equal(matrix(await tableAst())[1][1], '新增内容'));
    const saved = await ast(); await page.getByRole('button', { name: '重新载入当前内容' }).click(); await root.waitFor();
    assert.deepEqual(await ast(), saved);
    assert.equal(await cells().nth(4).innerText(), '新增内容');
  });
  await check('row and column deletion preserve all other cells and can be undone', async () => {
    for (const [name, expected] of [
      ['删除当前行', [['A1','A2','A3']]],
      ['删除当前列', [['A1','A3'],['B1','B3']]],
    ]) {
      await set([table()]); const before = await ast(); await contextAction(4, name);
      await wait(async () => assert.deepEqual(matrix(await tableAst()), expected));
      await undo(); assert.deepEqual(await ast(), before);
    }
  });
  await check('clearing a cell restores its full block subtree on undo', async () => {
    const value = table(); value.content[1].content[1].content.push({ type: 'bulletList', content: [{ type: 'listItem', content: [{ type: 'paragraph', content: [{ type: 'text', text: '嵌套内容' }] }] }] });
    await set([value]); const before = await ast(); await contextAction(4, '清空单元格');
    await wait(async () => assert.equal(matrix(await tableAst())[1][1], ''));
    assert.equal((await tableAst()).children[1].children[1].children.length, 1);
    await undo(); assert.deepEqual(await ast(), before);
  });
  await check('last row and column stay protected; explicit table deletion is undoable', async () => {
    await set([table(100, 1, 1)]); const before = await ast();
    await cells().first().click({ button: 'right' });
    assert.equal(await menu().getByRole('menuitem', { name: '删除当前行', exact: true }).getAttribute('aria-disabled'), 'true');
    assert.equal(await menu().getByRole('menuitem', { name: '删除当前列', exact: true }).getAttribute('aria-disabled'), 'true');
    await menu().getByRole('menuitem', { name: '删除表格', exact: true }).click();
    await wait(async () => assert.equal((await ast()).children.some(node => node.type === 'table'), false));
    await undo(); assert.deepEqual(await ast(), before);
  });
  await check('toolbar, keyboard menu, caption and header switches operate on the current cell', async () => {
    await set([table()]); await caret(cells().nth(4).locator('p'));
    await page.keyboard.press('Shift+F10'); await menu().waitFor();
    assert.match(await menu().innerText(), /第 2 行，第 2 列/);
    const before = await ast(); await page.keyboard.press('Escape');
    assert.deepEqual(await ast(), before);
    await wait(async () => assert.equal(await root.evaluate(el => document.activeElement === el), true));
    await editor.getByRole('button', { name: '表格操作', exact: true }).click();
    await menu().getByRole('menuitem', { name: '在右侧插入列', exact: true }).click();
    assert.deepEqual(matrix(await tableAst()), [['A1','A2','','A3'],['B1','B2','','B3']]);
    await editor.getByLabel('表格标题').fill('新标题'); await editor.getByLabel('表格标题').blur();
    assert.equal((await tableAst()).caption, '新标题');
    for (const [label, key] of [['首行作为表头','has_column_header'],['首列作为表头','has_row_header']]) {
      await editor.getByRole('button', { name: '表格操作', exact: true }).click();
      await menu().getByRole('menuitemcheckbox', { name: label, exact: true }).click();
      assert.equal((await tableAst())[key], true);
      await undo(); assert.equal(Boolean((await tableAst())[key]), false);
    }
  });
  await check('nested and adjacent tables keep their operations separate', async () => {
    const outer = table(100, 1, 1); outer.content[0].content[0].content.push(table(200, 2, 2));
    await set([outer, table(300)]);
    const before = await ast();
    const tables = editor.locator('[data-editor-block-type="astTable"]');
    await tables.nth(1).locator('.ast-table-cell').nth(3).click({ button: 'right' });
    assert.equal(await menu().count(), 1);
    await menu().getByRole('menuitem', { name: '在下方插入行', exact: true }).click();
    const after = await ast();
    assert.equal(after.children[0].children.length, 1);
    assert.equal(after.children[0].children[0].children[0].children[1].children.length, 3);
    assert.deepEqual(after.children[1], before.children[1]);
    await tables.nth(2).locator('.ast-table-cell').first().click({ button: 'right' });
    await menu().getByRole('menuitem', { name: '删除当前列', exact: true }).click();
    assert.deepEqual((await ast()).children[0], after.children[0]);
    assert.equal((await ast()).children[1].children[0].children.length, 2);
  });
  await check('ragged legacy tables never lose unrelated cells or leave empty rows', async () => {
    const value = table(); value.content[0].content = value.content[0].content.slice(0, 1);
    await set([value]); await cells().nth(1).click({ button: 'right' });
    assert.equal(await menu().getByRole('menuitem', { name: '删除当前列', exact: true }).getAttribute('aria-disabled'), 'true');
    await page.keyboard.press('Escape'); await contextAction(3, '在右侧插入列');
    assert.deepEqual(matrix(await tableAst()), [['A1',''],['B1','B2','B3','']]);
  });
  await check('an open menu never retargets when its cell is removed remotely', async () => {
    const value = table(); await set([value]);
    await cells().nth(4).click({ button: 'right' });
    await root.evaluate((el, value) => {
      value.content.splice(1, 1);
      const next = el.editor.schema.nodeFromJSON({ type: 'doc', content: [value] });
      el.editor.view.dispatch(el.editor.state.tr.replaceWith(0, el.editor.state.doc.content.size, next.content).setMeta('addToHistory', false));
    }, value);
    await wait(async () => assert.equal(await menu().getByRole('menuitem', { name: '删除当前列', exact: true }).getAttribute('aria-disabled'), 'true'));
    assert.deepEqual(matrix(await tableAst()), [['A1','A2','A3']]);
    await page.keyboard.press('Escape');
  });
  await check('touch toolbar fits a narrow viewport and read-only mode exposes no editing menu', async () => {
    await set([table()]); await page.setViewportSize({ width: 390, height: 700 });
    await cells().nth(4).locator('p').tap();
    await editor.getByRole('button', { name: '表格操作', exact: true }).tap();
    const box = await menu().boundingBox(); assert(box.x >= 0 && box.x + box.width <= 390);
    await menu().getByRole('menuitem', { name: '在下方插入行', exact: true }).tap();
    assert.equal((await tableAst()).children.length, 3);
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth));
    const before = await ast(); await root.evaluate(el => el.editor.setEditable(false));
    await wait(async () => assert.equal(await editor.getByRole('button', { name: '表格操作', exact: true }).count(), 0));
    await cells().first().click({ button: 'right' });
    assert.equal(await menu().count(), 0); assert.deepEqual(await ast(), before);
  });
  assert.deepEqual(errors, []);
  console.log('PASS no browser runtime errors');
} finally { await browser.close(); }
