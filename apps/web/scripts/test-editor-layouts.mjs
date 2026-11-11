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
  const layout = (count=2) => ({type:'columnList',attrs:{id:id(100)},content:Array.from({length:count},(_,i)=>({type:'column',attrs:{id:id(110+i),widthRatio:i+1},content:[p(120+i,`正文${i+1}`)]}))});
  const cols = () => root.locator('.ast-column');
  const group = () => root.locator('.ast-columns').first();
  const layoutAst = async () => (await ast()).children[0];
  const undo = async () => root.evaluate(el=>el.editor.commands.undo());
  const redo = async () => root.evaluate(el=>el.editor.commands.redo());
  const act = async (index,name) => {await cols().nth(index).getByRole('button',{name:/栏操作/}).click();await page.getByRole('menuitem',{name,exact:true}).click();};
  const handle = () => group().getByRole('separator').first();
  const startDrag = async () => {const b=await handle().boundingBox(); const x=b.x+b.width/2,y=b.y+b.height/2;await page.mouse.move(x,y);await page.mouse.down();return{x,y};};
  const widths = async () => cols().evaluateAll(nodes=>nodes.map(n=>n.getBoundingClientRect().width));
  await check('new columns start empty and focus the first column',async()=>{
    await set([{type:'paragraph'}]);await root.evaluate(el=>{el.editor.commands.setTextSelection(1);el.editor.view.focus();});
    await page.keyboard.insertText('/columns');await page.getByRole('button',{name:/分栏/}).click();
    assert.deepEqual((await layoutAst()).children.map(c=>c.children[0].content),[[],[]]);
    await page.keyboard.insertText('第一栏输入');assert.equal((await layoutAst()).children[0].children[0].content[0].text,'第一栏输入');
  });
  await check('columns expose controls directly and preserve weighted layout',async()=>{
    await set([layout()]);assert.equal(await group().locator('details,input[type=number]').count(),0);
    const w=await widths();assert(Math.abs(w[1]/w[0]-2)<.05);
    await root.screenshot({path:'/tmp/anby-layout-columns.png'});
  });
  await check('column insertion and movement preserve content, IDs and independent undo',async()=>{
    for(const [action,index,order] of [['在左侧插入栏',1,['正文1','','正文2']],['在右侧插入栏',0,['正文1','','正文2']],['向左移动',1,['正文2','正文1']],['向右移动',0,['正文2','正文1']]]) {
      await set([layout()]);const before=await ast();await act(index,action);
      const after=await ast();assert.deepEqual(after.children[0].children.map(c=>c.children[0].content[0]?.text||''),order);
      for(const old of before.children[0].children) assert.deepEqual(after.children[0].children.find(c=>c.id===old.id),old);
      await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
    }
  });
  await check('column count stops at five and insertion focuses the new body',async()=>{
    await set([layout()]);for(let i=0;i<3;i++)await group().getByRole('button',{name:'增加一栏'}).click();
    assert.equal((await layoutAst()).children.length,5);assert(await group().getByRole('button',{name:'增加一栏'}).isDisabled());
    await page.keyboard.insertText('新增正文');assert.equal((await layoutAst()).children[4].children[0].content[0].text,'新增正文');
    await cols().first().getByRole('button',{name:/栏操作/}).click();assert.equal(await page.getByRole('menuitem',{name:'在左侧插入栏'}).getAttribute('data-disabled'),'');await page.keyboard.press('Escape');
  });
  await check('merging either direction retains subtree order and undo restores containers',async()=>{
    for(const action of ['合并到左栏','合并到右栏']){
      await set([layout(3)]);const before=await ast();await act(1,action);const merged=await layoutAst();
      assert.equal(merged.children.length,2);assert.deepEqual(merged.children.flatMap(c=>c.children),before.children[0].children.flatMap(c=>c.children));
      await undo();assert.deepEqual(await ast(),before);
    }
    await set([layout()]);const before=await ast();await act(0,'合并到右栏');
    assert.deepEqual((await ast()).children,[...before.children[0].children.flatMap(c=>c.children),...before.children.slice(1)]);
    await undo();assert.deepEqual(await ast(),before);
    await group().getByRole('button',{name:'取消分栏'}).click();assert.deepEqual((await ast()).children,[...before.children[0].children.flatMap(c=>c.children),...before.children.slice(1)]);
  });
  await check('drag resizes visibly, saves once and undoes without losing content',async()=>{
    await set([layout()]);const before=await ast(),w=await widths();const at=await startDrag();await page.mouse.move(at.x+45,at.y,{steps:5});
    assert.deepEqual(await ast(),before);assert((await widths())[0]>w[0]+30);
    await page.mouse.up();const after=await ast();assert(after.children[0].children[0].width_ratio>1);
    assert(Math.abs(after.children[0].children.reduce((n,c)=>n+c.width_ratio,0)-3)<.0001);
    assert.deepEqual(after.children[0].children.flatMap(c=>c.children),before.children[0].children.flatMap(c=>c.children));
    await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
    await page.getByRole('button',{name:'重新载入当前内容'}).click();await root.waitFor();assert.deepEqual(await ast(),after);
    await handle().dblclick();assert.equal((await layoutAst()).children[0].width_ratio,1.5);
  });
  await check('Escape cancels a drag and keyboard resizing respects bounds',async()=>{
    await set([layout()]);const before=await ast(),w=await widths();const at=await startDrag();await page.mouse.move(at.x+40,at.y);await page.keyboard.press('Escape');await page.mouse.up();
    assert.deepEqual(await ast(),before);assert.deepEqual(await widths(),w);
    await handle().focus();await page.keyboard.press('Home');assert.equal((await layoutAst()).children[0].width_ratio,.1);
    await page.keyboard.press('End');assert(Math.abs((await layoutAst()).children[1].width_ratio-.1)<.0001);
    await page.keyboard.press('ArrowLeft');assert((await layoutAst()).children[1].width_ratio>.1);
    await group().getByRole('button',{name:'平均分配'}).click();assert.deepEqual((await layoutAst()).children.map(c=>c.width_ratio),[1,1]);
  });
  await check('remote changes cancel drag previews without overwriting current widths',async()=>{
    await set([layout()]);const at=await startDrag();await page.mouse.move(at.x+40,at.y);
    await root.evaluate(el=>{const tr=el.editor.state.tr;const node=tr.doc.nodeAt(1);tr.setNodeMarkup(1,undefined,{...node.attrs,widthRatio:3}).setMeta('addToHistory',false);el.editor.view.dispatch(tr);});
    await page.mouse.up();assert.equal((await layoutAst()).children[0].width_ratio,3);
    assert.equal(await cols().first().evaluate(el=>Number(el.style.flexGrow)),3);
  });
  await check('nested columns and empty unwrapping preserve surrounding content',async()=>{
    const nested=layout();nested.attrs.id=id(300);nested.content.forEach((c,i)=>{c.attrs.id=id(310+i);c.content=[p(320+i,`内层${i+1}`)];});
    const outer=layout();outer.content[0].content.push(nested);
    await set([outer]);const before=await ast();
    const inner=root.locator('.ast-columns').nth(1);await inner.getByRole('button',{name:'取消分栏',exact:true}).click();
    const after=await ast();assert.deepEqual(after.children[0].children[1],before.children[0].children[1]);
    assert.deepEqual(after.children[0].children[0].children,[before.children[0].children[0].children[0],...before.children[0].children[0].children[1].children.flatMap(c=>c.children)]);
    await undo();assert.deepEqual(await ast(),before);
    const empty=layout();empty.content.forEach(c=>{c.content=[];});await set([empty]);await group().getByRole('button',{name:'取消分栏'}).click();
    assert.equal((await ast()).children[0].type,'paragraph');await page.keyboard.insertText('继续输入');assert(JSON.stringify(await ast()).includes('继续输入'));
  });
  const breadcrumb = items => ({type:'breadcrumb',attrs:{id:id(200),itemsJson:JSON.stringify(items)}});
  const pathItems=[{display_text:'首页',target_page_id:id(71)},{display_text:'当前页面'}];
  const path = () => root.locator('[data-editor-block-type="breadcrumb"]');
  const dialog = () => page.getByRole('dialog',{name:'编辑页面路径'});
  await check('path items open editing directly, reorder atomically and cancel cleanly',async()=>{
    await set([breadcrumb(pathItems)]);assert.equal(await path().locator('details').count(),0);const before=await ast();
    await path().getByRole('button',{name:'编辑路径项：当前页面'}).click();assert.equal(await dialog().getByRole('textbox',{name:'第 2 项名称'}).evaluate(el=>el===document.activeElement),true);
    await dialog().getByRole('button',{name:'向前移动第 2 项'}).click();await dialog().getByRole('textbox',{name:'第 1 项名称'}).fill(' 新名称 ');
    assert.deepEqual(await ast(),before);await dialog().getByRole('button',{name:'取消',exact:true}).click();assert.deepEqual(await ast(),before);
    await path().getByRole('button',{name:'编辑路径项：当前页面'}).click();await dialog().getByRole('button',{name:'向前移动第 2 项'}).click();await dialog().getByRole('button',{name:'保存页面路径'}).click();
    assert.deepEqual((await ast()).children[0].items,[pathItems[1],pathItems[0]]);await undo();assert.deepEqual(await ast(),before);
  });
  await check('path addition searches pages, supports plain labels and prevents empty names',async()=>{
    await set([breadcrumb([{display_text:'起点'}])]);await path().getByRole('button',{name:'添加路径项'}).click();assert(await dialog().getByRole('button',{name:'保存页面路径'}).isDisabled());
    await dialog().locator('.ast-path-item').last().getByRole('button',{name:'选择页面',exact:true}).click();
    const picker=page.getByRole('dialog',{name:'选择页面'});await picker.getByRole('combobox').fill('人物');await picker.getByRole('option').filter({hasText:'人物介绍'}).click();
    await dialog().getByRole('button',{name:'保存页面路径'}).click();assert.equal((await ast()).children[0].items[1].target_page_id,id(71));
    await path().getByRole('button',{name:'编辑路径项：人物介绍'}).click();await dialog().getByRole('button',{name:'清除页面'}).click();await dialog().getByRole('textbox',{name:'第 2 项名称'}).fill('  纯文字  ');await dialog().getByRole('button',{name:'保存页面路径'}).click();
    assert.deepEqual((await ast()).children[0].items[1],{display_text:'纯文字'});
    await page.getByRole('button',{name:'重新载入当前内容'}).click();await root.waitFor();assert.equal(await path().getByRole('button',{name:'编辑路径项：纯文字'}).count(),1);
  });
  await check('path removal and limits protect the last item and maximum length',async()=>{
    await set([breadcrumb([{display_text:'唯一项'}])]);await path().getByRole('button',{name:'编辑路径项：唯一项'}).click();
    assert(await dialog().getByRole('button',{name:'移除第 1 项'}).isDisabled());await dialog().getByRole('button',{name:'取消',exact:true}).click();
    await set([breadcrumb(Array.from({length:32},(_,i)=>({display_text:`路径${i+1}`})))]);assert(await path().getByRole('button',{name:'添加路径项'}).isDisabled());
    await path().getByRole('button',{name:'编辑路径项：路径32',exact:true}).click();assert(await dialog().getByRole('button',{name:'添加路径项'}).isDisabled());
    await dialog().getByRole('button',{name:'移除第 32 项'}).click();await dialog().getByRole('button',{name:'保存页面路径'}).click();assert.equal((await ast()).children[0].items.length,31);
  });
  await check('stale path drafts never overwrite remote updates',async()=>{
    await set([breadcrumb(pathItems)]);await path().getByRole('button',{name:'编辑路径项：当前页面'}).click();await dialog().getByRole('textbox',{name:'第 2 项名称'}).fill('本地修改');
    await root.evaluate(el=>{const tr=el.editor.state.tr;tr.setNodeMarkup(0,undefined,{...tr.doc.firstChild.attrs,itemsJson:JSON.stringify([{display_text:'远端更新'}])}).setMeta('addToHistory',false);el.editor.view.dispatch(tr);});
    await dialog().getByRole('button',{name:'保存页面路径'}).click();assert.deepEqual((await ast()).children[0].items,[{display_text:'远端更新'}]);
  });
  await check('mobile controls stay usable without overflow and readonly hides editing controls',async()=>{
    await set([breadcrumb(pathItems),layout()]);await page.setViewportSize({width:320,height:900});
    const bounds=await cols().evaluateAll(nodes=>nodes.map(n=>n.getBoundingClientRect().toJSON()));assert(bounds[1].y>bounds[0].y);
    assert.equal(await handle().isVisible(),false);await cols().first().getByRole('button',{name:/栏操作/}).tap();await page.getByRole('menuitem',{name:'向右移动'}).tap();
    await path().getByRole('button',{name:'编辑路径项：当前页面'}).tap();assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));
    await page.screenshot({path:'/tmp/anby-layout-mobile.png',animations:'disabled'});await dialog().getByRole('button',{name:'取消',exact:true}).tap();
    await root.evaluate(el=>el.editor.setEditable(false));assert.equal(await group().locator('button,[role=separator]').count(),0);assert.equal(await path().locator('button').count(),0);
  });
  await check('no browser runtime errors',async()=>assert.deepEqual(errors,[]));
} finally { await browser.close(); }
