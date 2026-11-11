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
    if (url.pathname === '/api/v1/pages' || url.pathname === '/api/v1/pages/search') return route.fulfill({ json: { items: [{id:'0198a1b2-c3d4-7e5f-8a9b-000000000071',display_title:'人物介绍',namespace:'main',language:'zh',matched_on:'title',highlight:'',score:1}],next_cursor:'' } });
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
  const id = n => `0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
  const p = (n,text) => ({type:'paragraph',attrs:{id:id(n)},content:[{type:'text',text,marks:[{type:'bold'}]}]});
  const marker = () => editor.locator('.anby-block-gutter');
  const active = n => editor.locator(`[data-block-indicator="active"][data-block-id="${id(n)}"]`);
  const focus = async n => {
    await root.evaluate((el,id)=>{let pos;el.editor.state.doc.descendants((node,offset)=>{if(node.attrs.id===id)pos=offset+1;});el.editor.commands.setTextSelection(pos);el.editor.view.focus();},id(n));
    await active(n).waitFor();
  };
  const sameBounds = async n => {
    const actual=await active(n).boundingBox(),expected=await root.locator(`[data-id="${id(n)}"]`).boundingBox();
    for(const key of ['x','y','width','height'])assert(Math.abs(actual[key]-expected[key])<1,`indicator ${key} follows block ${n}`);
  };
  await check('focus and block selection do not change content, IDs or text layout',async()=>{
    await set([p(1,'第一个正文块'),p(2,'第二个正文块')]);
    const before=await ast(),box=await root.locator('p').first().boundingBox();
    await focus(1);await sameBounds(1);assert.deepEqual(await ast(),before);assert.deepEqual(await root.locator('p').first().boundingBox(),box);
    assert.equal(await marker().getAttribute('data-block-id'),id(1));
    await marker().click();
    assert.equal(await root.evaluate(el=>el.editor.state.selection.node?.attrs.id),id(1));assert.deepEqual(await ast(),before);
    await page.keyboard.press('ArrowRight');assert.equal(await root.evaluate(el=>el.editor.state.selection.constructor.name),'TextSelection');
  });
  await check('hover previews another block while the current editing block stays marked',async()=>{
    await focus(1);const before=await ast();
    await root.locator(`[data-id="${id(2)}"]`).hover();await editor.locator(`[data-block-indicator="hover"][data-block-id="${id(2)}"]`).waitFor();
    await active(1).waitFor();assert.equal(await marker().getAttribute('data-block-id'),id(2));assert.deepEqual(await ast(),before);
    await marker().hover();await page.getByRole('tooltip').waitFor();assert((await page.getByRole('tooltip').innerText()).includes('点击选中整块'));
    await marker().click();await active(2).waitFor();assert.equal(await root.evaluate(el=>el.editor.state.selection.node.attrs.id),id(2));
  });
  await check('keyboard navigation and typing move the persistent marker without mouse movement',async()=>{
    await focus(1);await page.mouse.move(0,0);
    await root.evaluate(el=>{const n=el.editor.state.doc.firstChild;el.editor.commands.setTextSelection(n.nodeSize-1);});
    await page.keyboard.press('ArrowDown');await active(2).waitFor();assert.equal(await marker().getAttribute('data-block-id'),id(2));
    await page.keyboard.insertText('继续输入');await active(2).waitFor();assert((await ast()).children[1].content.some(x=>x.text?.includes('继续输入')));
  });
  await check('toggle summary identifies its real container and body identifies its nested block',async()=>{
    await set([{type:'toggleBlock',attrs:{id:id(10)},content:[{type:'paragraph',content:[{type:'text',text:'折叠摘要'}]},p(11,'折叠正文')]}]);
    await root.locator('.ast-outline p').first().click();await active(10).waitFor();assert.equal(await marker().getAttribute('data-block-path'),'折叠块');
    await focus(11);assert.equal(await marker().getAttribute('data-block-path'),'折叠块 › 正文');
    await root.getByRole('button',{name:'折叠内容',exact:true}).click();await active(10).waitFor();assert.equal(await root.locator(`[data-id="${id(11)}"]`).isVisible(),false);
  });
  await check('lists, tables and columns locate the deepest real block and expose container context',async()=>{
    const fixtures=[
      [{type:'bulletList',attrs:{id:id(20)},content:[{type:'listItem',attrs:{id:id(21)},content:[p(22,'列表正文')]}]},22,'无序列表 › 列表项 › 正文'],
      [{type:'astTable',attrs:{id:id(30)},content:[{type:'astTableRow',attrs:{id:id(31)},content:[{type:'astTableCell',attrs:{id:id(32)},content:[p(33,'单元格正文')]}]}]},33,'表格 › 表格行 › 单元格 › 正文'],
      [{type:'columnList',attrs:{id:id(40)},content:[{type:'column',attrs:{id:id(41),widthRatio:1},content:[p(42,'左栏')]},{type:'column',attrs:{id:id(43),widthRatio:1},content:[p(44,'右栏')]}]},44,'分栏 › 栏 › 正文'],
    ];
    for(const [block,n,path]of fixtures){await set([block]);await focus(n);await sameBounds(n);assert.equal(await marker().getAttribute('data-block-path'),path);await marker().click();assert.equal(await root.evaluate(el=>el.editor.state.selection.node.attrs.id),id(n));}
  });
  await check('atomic blocks can be selected from the gutter',async()=>{
    await set([p(50,'分割线之前'),{type:'horizontalRule',attrs:{id:id(51)}},p(52,'分割线之后')]);
    const before=await ast();await root.locator('hr').hover();await marker().click();await active(51).waitFor();assert.equal(await marker().getAttribute('aria-label'),'选中分割线块');assert.deepEqual(await ast(),before);
  });
  await check('node view form focus identifies its owning block',async()=>{
    await set([{type:'preformatted',attrs:{id:id(60),text:'等宽内容',kind:'plain',caption:'说明'}}]);
    await root.locator('summary').click();await root.locator('textarea').focus();await active(60).waitFor();assert.equal(await marker().getAttribute('data-block-path'),'等宽文本');
  });
  await check('position changes and removal never select a stale block offset',async()=>{
    await set([p(70,'前文'),p(71,'目标块')]);await focus(71);await root.locator(`[data-id="${id(71)}"]`).hover();
    await root.evaluate(el=>el.editor.commands.insertContentAt(0,{type:'paragraph',attrs:{id:'0198a1b2-c3d4-7e5f-8a9b-000000000072'},content:[{type:'text',text:'新增前文'}]}));
    await page.mouse.move(0,0);await focus(71);await editor.locator(`.anby-block-gutter[data-block-id="${id(71)}"]`).waitFor();await marker().click();assert.equal(await root.evaluate(el=>el.editor.state.selection.node.attrs.id),id(71));
    await page.keyboard.press('Backspace');await page.waitForFunction(id=>!document.querySelector(`.anby-block-gutter[data-block-id="${id}"]`),id(71));
    assert(!(await ast()).children.some(n=>n.id===id(71)));
  });
  await check('scroll and resize keep the rail aligned and out of text, including dark touch layouts',async()=>{
    await set(Array.from({length:12},(_,i)=>p(100+i,`第 ${i+1} 个正文块，包含用于换行的文字。`.repeat(3))));
    for(const width of [1440,720,320]){
      await page.setViewportSize({width,height:900});await root.locator(`[data-id="${id(107)}"]`).scrollIntoViewIfNeeded();
      await focus(107);if(width===320)await root.locator(`[data-id="${id(107)}"]`).tap({position:{x:20,y:14}});else await root.locator(`[data-id="${id(107)}"]`).click({position:{x:20,y:14}});await active(107).waitFor();await sameBounds(107);
      const handle=await marker().boundingBox(),text=await root.locator(`[data-id="${id(107)}"]`).boundingBox();assert(handle.x>=0&&handle.x+handle.width<=text.x,'gutter never covers text');
      await page.mouse.move(0,0);await editor.locator(`.anby-block-gutter[data-block-id="${id(107)}"]`).waitFor();await marker().focus();assert(await marker().evaluate(el=>el===document.activeElement));await page.keyboard.press('Enter');assert.equal(await root.evaluate(el=>el.editor.state.selection.node.attrs.id),id(107));
      await page.evaluate(()=>document.documentElement.classList.toggle('dark'));
      await page.screenshot({path:`/tmp/anby-block-gutter-${width}.png`,animations:'disabled'});
    }
  });
  await check('read-only editor hides editing controls',async()=>{
    await root.evaluate(el=>el.editor.setEditable(false));await marker().waitFor({state:'detached'});assert.equal(await editor.locator('[data-block-indicator]').count(),0);
    await root.evaluate(el=>el.editor.setEditable(true));await focus(107);await marker().waitFor();
  });
  assert.deepEqual(errors,[]);console.log('PASS no browser runtime errors');
}finally{await browser.close();}
