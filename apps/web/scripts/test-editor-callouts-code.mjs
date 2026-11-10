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
  const ast = async () => JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const set = async content => {
    await page.getByRole('button', { name: '空白文档' }).click(); await root.waitFor();
    await root.evaluate((el, content) => el.editor.chain().setContent({ type: 'doc', content }).command(({ tr }) => { tr.setMeta('addToHistory', false); return true; }).run(), content);
  };
  const check = async (name, run) => { await run(); console.log(`PASS ${name}`); };
  const caret = async pos => root.evaluate((el, pos) => { el.editor.commands.setTextSelection(pos); el.editor.view.focus(); }, pos);
  const slashCode = async () => { await page.keyboard.insertText('/cod'); await page.getByRole('button', { name: /代码块/ }).click(); };
  const paragraph = text => ({ type: 'paragraph', ...(text ? { content: [{ type: 'text', text }] } : {}) });
  const softLines = { type: 'paragraph', content: [{ type: 'hardBreak' }, { type: 'text', text: '你好' }, { type: 'hardBreak' }, { type: 'text', text: '测试' }] };
  for (const [name, content] of [
    ['soft breaks', [softLines]],
    ['separate paragraphs', [paragraph(), paragraph('你好'), paragraph('测试')]],
    ['reloaded newline text', [paragraph('\n你好\n测试')]],
  ]) await check(`slash code preserves following ${name} and focuses code`, async () => {
    await set(content); await caret(1); await slashCode();
    const saved = await ast();
    assert.equal(saved.children[0].type, 'code'); assert.equal(saved.children[0].content, '');
    assert.equal(saved.children[1].content[0].text.startsWith('你好'), true);
    assert(saved.children.slice(1).some(block => JSON.stringify(block).includes('你好')));
    assert(saved.children.slice(1).some(block => JSON.stringify(block).includes('测试')));
    await page.keyboard.insertText('const x = 1;');
    assert.equal((await ast()).children[0].content, 'const x = 1;');
    const typed = await ast();
    await page.getByRole('button', { name: '重新载入当前内容' }).click(); await root.waitFor();
    const reloaded = await ast();
    assert.deepEqual(reloaded.children[0], typed.children[0]);
    assert.deepEqual(reloaded.children.slice(1).map(block => [block.id, block.content.map(inline => inline.text).join('')]), typed.children.slice(1).map(block => [block.id, block.content.map(inline => inline.text).join('')]));
  });
  await check('code insertion preserves rich content on both sides and undo redo', async () => {
    await set([{ type: 'paragraph', content: [
      { type: 'text', text: '前文', marks: [{ type: 'bold' }] }, { type: 'hardBreak' },
      { type: 'hardBreak' }, { type: 'text', text: '后文', marks: [{ type: 'italic' }] },
      { type: 'pageReference', attrs: { targetPageId: '0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01', displayText: '引用' } },
    ] }]);
    await caret(4); await slashCode();
    const saved = await ast();
    assert.deepEqual(saved.children[0].content, [{ type: 'text', text: '前文', marks: ['bold'] }]);
    assert.equal(saved.children[1].type, 'code');
    assert.deepEqual(saved.children[2].content[0], { type: 'text', text: '后文', marks: ['italic'] });
    assert.equal(saved.children[2].content[1].display_text, '引用');
    await root.evaluate(el => el.editor.commands.undo());
    assert(!(await ast()).children.some(block => block.type === 'code'));
    assert(JSON.stringify(await ast()).includes('后文'));
    await root.evaluate(el => el.editor.commands.redo()); assert.deepEqual(await ast(), saved);
  });
  for (const [name, wrap] of [
    ['callout', p => ({ type: 'callout', content: [p] })],
    ['list item', p => ({ type: 'bulletList', content: [{ type: 'listItem', content: [p] }] })],
    ['toggle summary', p => ({ type: 'toggleBlock', content: [p] })],
    ['table cell', p => ({ type: 'astTable', content: [{ type: 'astTableRow', content: [{ type: 'astTableCell', content: [p] }] }] })],
  ]) for (const [contentName, p] of [['soft lines', softLines], ['empty paragraph', paragraph()]]) await check(`code insertion in ${name} with ${contentName} preserves the container`, async () => {
    await set([wrap(p)]);
    await root.evaluate(el => { let first; el.editor.state.doc.descendants((node, pos) => { if (first === undefined && node.type.name === 'paragraph') first = pos + 1; }); el.editor.commands.setTextSelection(first); el.editor.view.focus(); });
    await slashCode(); await page.keyboard.insertText('code');
    const doc = await root.evaluate(el => el.editor.getJSON());
    assert.equal(doc.content[0].type, wrap(softLines).type);
    if (contentName === 'soft lines') { assert(JSON.stringify(doc).includes('你好')); assert(JSON.stringify(doc).includes('测试')); }
    assert.equal(await root.evaluate(el => el.editor.state.selection.$from.parent.type.name), 'codeBlock');
    assert.equal(await root.evaluate(el => el.editor.state.selection.$from.parent.textContent), 'code');
  });
  await check('slash remains literal inside code', async () => {
    await set([{ type: 'codeBlock' }]); await caret(1); await page.keyboard.insertText('/cod');
    assert.equal(await page.locator('.anby-slash-menu').count(), 0);
    assert.equal((await ast()).children[0].content, '/cod');
  });
  const callout = () => root.locator('[data-editor-block-type="callout"]').first();
  const style = () => callout().getByRole('button', { name: '提示框样式' });
  const panel = () => page.getByRole('dialog', { name: '提示框样式' });
  await check('slash callout focuses its body for immediate typing', async () => {
    await set([paragraph()]); await caret(1); await page.keyboard.insertText('/callout');
    await page.getByRole('button', { name: /提示框/ }).click(); await page.keyboard.insertText('提示正文');
    assert.equal((await ast()).children[0].type, 'callout');
    assert.equal((await ast()).children[0].children[0].content[0].text, '提示正文');
  });
  await check('callout body edits directly and style controls preserve content', async () => {
    await set([{ type: 'callout', attrs: { kind: 'warning', color: 'orange', icon: '!' }, content: [paragraph('注意')] }]);
    assert.equal(await callout().locator('details,input,select').count(), 0);
    await callout().locator('p').fill('直接编辑提示内容');
    const body = (await ast()).children[0].children;
    await style().click();
    await panel().getByRole('button', { name: '危险', exact: true }).click();
    await panel().getByRole('button', { name: '紫色', exact: true }).click();
    await panel().getByRole('button', { name: '图标 💡', exact: true }).click();
    assert.equal((await ast()).children[0].kind, 'danger');
    assert.equal((await ast()).children[0].color, 'purple');
    assert.equal((await ast()).children[0].icon, '💡');
    assert.deepEqual((await ast()).children[0].children, body);
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.activeElement?.getAttribute('aria-label') === '提示框样式');
    const saved = await ast();
    await page.getByRole('button', { name: '重新载入当前内容' }).click(); await root.waitFor();
    assert.deepEqual(await ast(), saved);
    assert.equal((await page.locator('[data-ast-document] aside[role="note"]').innerText()).replace(/\s+/g, ' '), '💡 直接编辑提示内容');
  });
  await check('callout custom icon applies explicitly and cancel preserves the original', async () => {
    await style().click(); await panel().getByRole('textbox', { name: '自定义图标' }).fill('未提交');
    await page.keyboard.press('Escape'); assert.equal((await ast()).children[0].icon, '💡');
    await style().click(); await panel().getByRole('textbox', { name: '自定义图标' }).fill('⭐');
    await panel().getByRole('button', { name: '应用' }).click(); assert.equal((await ast()).children[0].icon, '⭐');
    await style().click(); await panel().getByRole('button', { name: '默认', exact: true }).last().click();
    assert.equal((await ast()).children[0].icon, undefined);
    await panel().getByRole('button', { name: '默认', exact: true }).first().click();
    assert.equal((await ast()).children[0].color, undefined);
    await page.keyboard.press('Escape');
  });
  await check('callout Enter continues content and an empty last line exits', async () => {
    await set([{ type: 'callout', content: [paragraph('正文')] }]); await caret(4);
    await page.keyboard.press('Enter'); await page.keyboard.insertText('第二段');
    assert.equal((await ast()).children[0].children.length, 2);
    await page.keyboard.press('Enter'); await page.keyboard.press('Enter'); await page.keyboard.insertText('外部段落');
    const doc = await ast(); assert.equal(doc.children[0].children.length, 2);
    assert.equal(doc.children[1].content[0].text, '外部段落');
    await root.evaluate(el => el.editor.commands.undo()); assert(JSON.stringify(await ast()).includes('正文'));
  });
  await check('empty callout exits and Backspace unwraps all children', async () => {
    await set([{ type: 'callout', content: [paragraph()] }]); await caret(2); await page.keyboard.press('Enter');
    assert.equal((await ast()).children[0].type, 'paragraph');
    await set([{ type: 'callout', content: [paragraph('正文'), paragraph('保留后文')] }]); await caret(2); await page.keyboard.press('Backspace');
    assert.equal((await ast()).children[0].type, 'paragraph');
    assert.equal((await ast()).children[1].content[0].text, '保留后文');
  });
  await check('callout style controls fit narrow touch viewports and respect readonly', async () => {
    await set([{ type: 'callout', content: [paragraph('手机提示框')] }]);
    await page.setViewportSize({ width: 320, height: 800 }); await style().tap();
    const box = await panel().boundingBox(); assert(box.x >= 0 && box.x + box.width <= 320);
    await panel().getByRole('button', { name: '绿色' }).tap();
    await page.screenshot({ path: '/tmp/anby-callout-mobile.png' });
    await page.keyboard.press('Escape');
    const before = await ast(); await root.evaluate(el => el.editor.setEditable(false));
    assert.equal(await style().count(), 0); assert.deepEqual(await ast(), before);
    await root.evaluate(el => el.editor.setEditable(true)); await page.setViewportSize({ width: 1440, height: 1000 });
    await callout().screenshot({ path: '/tmp/anby-callout-editor.png' });
  });
  await check('no browser runtime errors', async () => assert.deepEqual(errors, []));
} finally { await browser.close(); }
