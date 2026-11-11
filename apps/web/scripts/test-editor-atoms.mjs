// Run /dev/editor with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id = n => `0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addCookies([{ name: 'anby_session', value: 'editor-fixture', url: baseURL }]);
  const page = await context.newPage(), errors = [], outside = [];
  page.on('pageerror', error => errors.push(error.message));
  let mime = 'image/png', delay = 0;
  await page.route('**/*', async route => {
    const u = new URL(route.request().url()), p = u.pathname;
    if (u.origin !== baseURL) { outside.push(u.href); return route.fulfill({ body: '' }); }
    if (!p.startsWith('/api/')) return route.continue();
    if (p.endsWith('/auth/session')) return route.fulfill({ json: {actor_id:id(1),wiki_id:id(2),home_wiki_id:id(2),main_namespace_id:id(3),wiki_name:'测试 Wiki',actor_type:'human',display_name:'编辑者',method:'session',wikis:[]} });
    const revision = {id:id(61),asset_id:id(90),mime_type:mime,size_bytes:2048,created_at:'2026-01-01',content_hash:'hash'};
    if (p === '/api/v1/assets') return route.fulfill({json:{items:[{id:id(90),name:'新的文件',status:'active',current_revision:revision,created_at:'2026-01-01',updated_at:'2026-01-01'}],next_cursor:''}});
    if (/assets\/revisions\/[^/]+\/content$/.test(p)) return route.fulfill({contentType:'image/png',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=','base64')});
    if (/assets\/revisions\/[^/]+$/.test(p)) { if(delay)await new Promise(resolve=>setTimeout(resolve,delay)); return route.fulfill({json:revision}); }
    return route.fulfill({json:{items:[]}});
  });
  await page.goto(`${baseURL}/dev/editor`);
  const root = page.locator('[data-block-editor]').first().locator('.tiptap').first(); await root.waitFor();
  const ast = async () => JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const dialog = () => page.getByRole('dialog').last();
  const set = async content => { await page.getByRole('button',{name:'空白文档'}).click(); await root.waitFor(); await root.evaluate((el,content)=>el.editor.chain().setContent({type:'doc',content}).command(({tr})=>{tr.setMeta('addToHistory',false);return true;}).run(),content); };
  const check = async (name,fn) => { await fn(); console.log(`PASS ${name}`); };
  const wait = async fn => { let error;for(let n=0;n<80;n++){try{await fn();return;}catch(e){error=e;}await page.waitForTimeout(50);}throw error; };
  const undo = async () => root.evaluate(el=>el.editor.commands.undo());
  const redo = async () => root.evaluate(el=>el.editor.commands.redo());
  const p = {id:id(85),type:'paragraph',content:[{type:'text',text:'保留加粗文字',marks:['bold']}]};
  const button = (actions=[{type:'notification',message:'原通知',tone:'success'}]) => ({type:'pageButton',attrs:{id:id(100),label:'原按钮',confirmation:'原确认',actionsJson:JSON.stringify(actions)}});
  const html = () => ({type:'htmlBlock',attrs:{id:id(101),html:'<p>原内容</p>',height:240,caption:'原标题'}});
  const media = (type) => ({type:`${type}Asset`,attrs:{id:id(102),assetRevisionId:id(62),caption:'原说明',...(type==='image'?{altText:'自定义替代文字'}:type==='file'?{name:'原文件名'}:{title:'原标题'}),...(type==='audio'?{artist:'原艺术家',album:'原专辑',durationSeconds:1.5}:type==='pdf'?{startPage:3,height:640}:{})}});
  const open = async label => root.getByRole('button',{name:label,exact:true}).click();
  const cancel = async () => dialog().getByRole('button',{name:'取消',exact:true}).click();
  const choose = async label => {await dialog().getByRole('button',{name:`选择${label}`,exact:true}).click();await dialog().getByRole('option').filter({hasText:'新的文件'}).click();};
  await check('every media, HTML and button block exposes an editing button without folded properties',async()=>{
    await set([button(),html(),...['image','video','audio','file','pdf'].map(media)]);
    for(const label of ['编辑按钮','编辑 HTML','编辑图片','编辑视频','编辑音频','编辑文件','编辑 PDF'])assert(await root.getByRole('button',{name:label,exact:true}).isVisible());
    assert.equal(await root.locator('details').count(),0);
  });
  await check('button editing is atomic, cancelable, reversible and preserves nested and legacy actions',async()=>{
    const actions=[{type:'insert_blocks',target_page_id:id(71),blocks:[p]},{type:'notification',message:'原通知'},{type:'webhook',url:'https://example.com/hook',payload:{old:true}},{type:'update_dataset_record',record_id:id(91),values:{old:1}}];
    await set([button(actions)]);const before=await ast();await open('编辑按钮');
    await dialog().getByLabel('按钮名称',{exact:true}).fill('新按钮');await dialog().getByLabel('确认提示（可选）').fill('新确认');await dialog().getByLabel('通知内容').fill('新通知');
    assert.deepEqual(await ast(),before);await cancel();assert.deepEqual(await ast(),before);
    await open('编辑按钮');assert.equal(await dialog().getByLabel('按钮名称',{exact:true}).inputValue(),'原按钮');
    await dialog().getByLabel('按钮名称',{exact:true}).fill('新按钮');await dialog().getByLabel('通知内容').fill('新通知');await dialog().getByRole('button',{name:'保存按钮',exact:true}).click();
    const after=await ast();assert.equal(after.children[0].label,'新按钮');assert.deepEqual(after.children[0].actions[0],actions[0]);assert.deepEqual(after.children[0].actions[3],actions[3]);assert.equal(after.children[0].id,id(100));
    await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
  });
  await check('button action JSON validation keeps the draft and does not silently save old payload',async()=>{
    await set([button([{type:'webhook',url:'https://example.com/hook',payload:{old:true}}])]);const before=await ast();await open('编辑按钮');
    const payload=dialog().getByLabel('请求内容（JSON）');await payload.fill('{broken');await dialog().getByRole('button',{name:'保存按钮',exact:true}).click();
    assert.deepEqual(await ast(),before);assert.equal(await payload.inputValue(),'{broken');
    await payload.fill('[]');await dialog().getByRole('button',{name:'保存按钮',exact:true}).click();assert.deepEqual(await ast(),before);
    await payload.fill('{"updated":true}');await dialog().getByRole('button',{name:'保存按钮',exact:true}).click();assert.deepEqual((await ast()).children[0].actions[0].payload,{updated:true});
  });
  await check('button actions can be added and removed without mixing remaining row drafts',async()=>{
    await set([button()]);await open('编辑按钮');await dialog().getByRole('button',{name:'添加 Webhook'}).click();
    await dialog().getByLabel('公网 HTTPS URL').fill('https://example.com/hook');await dialog().getByLabel('请求内容（JSON）').fill('{"kept":1}');
    await dialog().getByRole('region',{name:'动作 1',exact:true}).getByRole('button',{name:'移除动作'}).click();
    assert.equal(await dialog().getByLabel('请求内容（JSON）').inputValue(),'{"kept":1}');await dialog().getByRole('button',{name:'保存按钮',exact:true}).click();assert.deepEqual((await ast()).children[0].actions[0].payload,{kept:1});
  });
  await check('HTML preview updates in its sandbox and save preserves identity with single undo',async()=>{
    await set([html()]);const before=await ast();await open('编辑 HTML');
    const code='<h1>预览标题</h1><script>window.executed=true</script><img src="https://example.com/blocked.png">';
    await dialog().getByLabel('HTML 代码').fill(code);const frame=dialog().locator('iframe');assert.equal(await frame.getAttribute('sandbox'),'');
    await frame.contentFrame().getByRole('heading',{name:'预览标题'}).waitFor();assert.equal(await frame.contentFrame().locator('body').evaluate(()=>window.executed),undefined);
    assert(!outside.some(url=>url.includes("example.com/blocked.png")));assert.deepEqual(await ast(),before);
    await dialog().getByLabel('高度（px）').fill('50');await dialog().getByRole('button',{name:'保存 HTML'}).click();assert.deepEqual(await ast(),before);
    await dialog().getByLabel('高度（px）').fill('400');await dialog().getByLabel('标题（可选）').fill('新的标题');await dialog().screenshot({path:'/tmp/anby-html-editor.png',animations:'disabled'});
    await dialog().getByRole('button',{name:'保存 HTML'}).click();const after=await ast();assert.equal(after.children[0].html,code);assert.equal(after.children[0].height,400);assert.equal(after.children[0].id,id(101));await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
    await page.getByRole('button',{name:'重新载入当前内容'}).click();await root.waitFor();assert.deepEqual(await ast(),after);
  });
  await check('HTML import previews locally and cancellation restores the saved source',async()=>{
    await set([html()]);const before=await ast();await open('编辑 HTML');await dialog().getByLabel('HTML 文件').setInputFiles({name:'demo.html',mimeType:'text/html',buffer:Buffer.from('<b>导入内容</b>')});
    await wait(async()=>assert.equal(await dialog().getByLabel('HTML 代码').inputValue(),'<b>导入内容</b>'));await cancel();assert.deepEqual(await ast(),before);await open('编辑 HTML');assert.equal(await dialog().getByLabel('HTML 代码').inputValue(),'<p>原内容</p>');await cancel();
  });
  await check('all five media kinds edit metadata atomically while retaining file versions and IDs',async()=>{
    for(const [type,label,m] of [['image','图片','image/png'],['video','视频','video/mp4'],['audio','音频','audio/mpeg'],['file','文件','application/octet-stream'],['pdf',' PDF','application/pdf']]){
      mime=m;await set([media(type)]);const before=await ast();await open(`编辑${label}`);await dialog().getByLabel('说明（可选）').fill('新说明');assert.deepEqual(await ast(),before);await cancel();assert.deepEqual(await ast(),before);
      await open(`编辑${label}`);await dialog().getByLabel('说明（可选）').fill('新说明');await dialog().getByRole('button',{name:'保存媒体'}).click();await wait(async()=>assert.equal((await ast()).children[0].caption,'新说明'));const after=await ast();assert.deepEqual(after.children[0],{...before.children[0],caption:'新说明'});
      await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
    }
  });
  await check('PDFs without an explicit start page open on page one and can save',async()=>{
    mime='application/pdf';const block=media('pdf');delete block.attrs.startPage;await set([block]);await open('编辑 PDF');assert.equal(await dialog().getByLabel('起始页').inputValue(),'1');await dialog().getByRole('button',{name:'保存媒体'}).click();await dialog().waitFor({state:'detached'});assert.equal((await ast()).children[0].type,'pdf');
  });
  await check('replacing media preserves custom text and rejects mismatched MIME types',async()=>{
    mime='image/png';await set([media('image')]);const before=await ast();await open('编辑图片');await choose('文件');mime='audio/mpeg';await dialog().getByRole('button',{name:'保存媒体'}).click();await page.getByText('所选文件版本不是图片',{exact:true}).waitFor();assert.deepEqual(await ast(),before);
    mime='image/png';await dialog().getByRole('button',{name:'保存媒体'}).click();await wait(async()=>assert.equal((await ast()).children[0].asset_revision_id,id(61)));assert.equal((await ast()).children[0].alt_text,'自定义替代文字');
  });
  await check('canceling a pending media save prevents late writes',async()=>{
    mime='image/png';await set([media('image')]);const before=await ast();await open('编辑图片');await dialog().getByLabel('说明（可选）').fill('不得保存');delay=700;await dialog().getByRole('button',{name:'保存媒体'}).click();await cancel();await page.waitForTimeout(850);delay=0;assert.deepEqual(await ast(),before);
  });
  await check('remote updates are preserved and readonly mode hides editing controls',async()=>{
    await set([html()]);await open('编辑 HTML');await dialog().getByLabel('HTML 代码').fill('<p>旧草稿</p>');
    await root.evaluate(el=>el.editor.commands.command(({tr,dispatch})=>{if(dispatch)tr.setNodeMarkup(0,undefined,{...tr.doc.firstChild.attrs,html:'<p>远端更新</p>'});return true;}));
    await dialog().getByRole('button',{name:'保存 HTML'}).click();await page.getByText('此块已更新，请重新打开编辑，避免覆盖新内容。',{exact:true}).waitFor();assert.equal((await ast()).children[0].html,'<p>远端更新</p>');await cancel();
    await root.evaluate(el=>el.editor.setEditable(false));assert.equal(await root.getByRole('button',{name:'编辑 HTML',exact:true}).count(),0);await root.evaluate(el=>el.editor.setEditable(true));
  });
  await check('new button and HTML insertion reuse the same forms',async()=>{
    for(const [query,menu,title,fill,save] of [['button','页面按钮','插入页面按钮','通知内容','插入按钮'],['html','HTML','插入 HTML','HTML 代码','插入 HTML']]){
      await set([{type:'paragraph'}]);await root.click();await page.keyboard.insertText('/'+query);await page.getByRole('button',{name:new RegExp(menu)}).click();await dialog().getByRole('heading',{name:title,exact:true}).waitFor();await dialog().getByLabel(fill,{exact:true}).fill('新内容');await dialog().getByRole('button',{name:save,exact:true}).click();assert.equal((await ast()).children[0].type,query==='button'?'button':'html');
    }
  });
  await check('editing panels fit a 320px viewport and support keyboard cancel',async()=>{
    await page.setViewportSize({width:320,height:844});await page.evaluate(()=>document.documentElement.classList.add('dark'));
    for(const [block,label] of [[button(),'编辑按钮'],[html(),'编辑 HTML'],[media('image'),'编辑图片']]){
      await set([block]);await open(label);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));const box=await dialog().boundingBox();assert(box.width<=320 && box.x>=0);await dialog().screenshot({path:`/tmp/anby-atom-mobile-${block.type}.png`,animations:'disabled'});await page.keyboard.press('Escape');await dialog().waitFor({state:'detached'});
    }
  });
  assert.deepEqual(errors,[]);console.log('PASS no browser runtime errors');
} finally {await browser.close();}
