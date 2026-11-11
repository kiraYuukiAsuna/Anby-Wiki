// Run /dev/editor with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id = n => `0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
const browser = await chromium.launch({channel:'chrome',headless:true});
try {
  const context = await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addCookies([{name:'anby_session',value:'editor-fixture',url:baseURL}]);
  const page = await context.newPage(), errors = [], previews = [], resolves = [];
  let failResolve=false, resolveDelay=0, failPreview=false, previewDelay=0, missingVersion=false;
  page.on('pageerror',error=>errors.push(error.message));
  const component = {id:id(10),name:'人物资料卡',component_key:'people-card',created_by:id(1),created_at:'2026-01-01',updated_at:'2026-01-01'};
  const entity = {id:id(20),display_label:'安比',canonical_key:'anby',status:'active',entity_type:{id:id(21),name:'人物',type_key:'person'},labels:[{label:'安比',is_primary:true}],aliases:[]};
  await page.route('**/*',async route=>{
    const u=new URL(route.request().url()),p=u.pathname;
    if(u.origin!==baseURL)return route.fulfill({body:''});
    if(!p.startsWith('/api/'))return route.continue();
    const json=value=>route.fulfill({json:value});
    if(p.endsWith('/auth/session'))return json({actor_id:id(1),wiki_id:id(2),home_wiki_id:id(2),main_namespace_id:id(3),wiki_name:'测试 Wiki',actor_type:'human',display_name:'编辑者',method:'session',wikis:[]});
    if(p==='/api/v1/link-previews/resolve'){
      const body=route.request().postDataJSON();resolves.push(body);if(resolveDelay)await new Promise(r=>setTimeout(r,resolveDelay));
      return failResolve?route.fulfill({status:503,json:{error:{code:'unavailable',message:'unavailable'}}}):json({url:body.url,title:'网页标题',description:'网页摘要',site_name:'网页站点'});
    }
    if(p==='/api/v1/components')return json({items:[component],next_cursor:''});
    if(/components\/[^/]+\/versions\/\d+\/preview$/.test(p)){
      const body=route.request().postDataJSON();previews.push({path:p,...body});if(previewDelay)await new Promise(r=>setTimeout(r,previewDelay));
      if(failPreview || body.props.title==='invalid')return route.fulfill({status:400,json:{error:{code:'validation_error',message:'invalid configuration'}}});
      const escaped=String(body.props.title??'默认标题').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
      return json({html:`<p>资料卡预览：${escaped}</p>`});
    }
    if(/components\/[^/]+\/versions$/.test(p))return json({items:(missingVersion?[2]:[1,2]).map(version=>({component_id:id(10),version,props_schema:{type:'object',properties:{title:{type:'string',default:'默认标题'}}},renderer_ref:'builtin.key_value',status:'published',created_by:id(1),created_at:'2026-01-01'}))});
    if(/components\/[^/]+$/.test(p))return json(component);
    if(p==='/api/v1/entities')return json({items:[entity],next_cursor:''});
    if(/entities\/[^/]+$/.test(p))return json(entity);
    return json({items:[]});
  });
  await page.goto(baseURL+'/dev/editor');
  const root=page.locator('[data-block-editor]').first().locator('.tiptap').first();await root.waitFor();
  const ast=async()=>JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const dialog=()=>page.getByRole('dialog').last();
  const set=async content=>{await page.getByRole('button',{name:'空白文档'}).click();await root.waitFor();await root.evaluate((el,content)=>el.editor.chain().setContent({type:'doc',content}).command(({tr})=>{tr.setMeta('addToHistory',false);return true;}).run(),content);};
  const check=async(name,fn)=>{await fn();console.log(`PASS ${name}`);};
  const wait=async fn=>{let error;for(let i=0;i<80;i++){try{await fn();return;}catch(e){error=e;}await page.waitForTimeout(50);}throw error;};
  const open=async name=>root.getByRole('button',{name,exact:true}).click();
  const cancel=async()=>dialog().getByRole('button',{name:'取消',exact:true}).click();
  const undo=async()=>root.evaluate(el=>el.editor.commands.undo());
  const redo=async()=>root.evaluate(el=>el.editor.commands.redo());
  const link=(type='linkPreview')=>({type,attrs:{id:id(100),url:'https://example.com/original',title:'原标题',description:'原摘要',siteName:'原站点',...(type==='bookmark'?{caption:'原说明'}:{})}});
  const card=(config={title:'原配置'})=>({type:'componentBlock',attrs:{id:id(101),componentId:id(10),componentVersion:1,entityId:id(20),displayConfig:JSON.stringify(config)}});
  await check('links and components expose direct controls and component body renders its actual preview',async()=>{
    await set([link(),link('bookmark'),{type:'embedBlock',attrs:{id:id(102),url:'https://www.youtube.com/watch?v=dQw4w9WgXcQ',provider:'youtube'}},card()]);
    for(const name of ['编辑链接预览','编辑书签','编辑嵌入内容','编辑知识组件'])assert(await root.getByRole('button',{name,exact:true}).isVisible());
    assert.equal(await root.locator('details').count(),0);await root.getByText('资料卡预览：原配置',{exact:true}).waitFor();assert(!await root.innerText().then(text=>text.includes(id(20))));
  });
  await check('manual link edits preview immediately, cancel cleanly and preserve identity through undo and reload',async()=>{
    for(const type of ['linkPreview','bookmark']){
      await set([link(type)]);const before=await ast();await open(type==='bookmark'?'编辑书签':'编辑链接预览');
      await dialog().getByLabel('标题',{exact:true}).fill('新标题');await dialog().getByLabel('摘要',{exact:true}).fill('新摘要');await dialog().getByText('新标题',{exact:true}).waitFor();assert.deepEqual(await ast(),before);await cancel();assert.deepEqual(await ast(),before);
      await open(type==='bookmark'?'编辑书签':'编辑链接预览');assert.equal(await dialog().getByLabel('标题',{exact:true}).inputValue(),'原标题');await dialog().getByLabel('标题',{exact:true}).fill('新标题');await dialog().getByLabel('链接地址').fill('https://example.com/new');await dialog().getByRole('button',{name:'保存链接'}).click();
      const after=await ast();assert.deepEqual(after.children[0],{...before.children[0],title:'新标题',url:'https://example.com/new'});await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);await page.getByRole('button',{name:'重新载入当前内容'}).click();await root.waitFor();assert.deepEqual(await ast(),after);
    }
  });
  await check('metadata refresh validates URLs, updates only the draft and keeps manual editing available after failure',async()=>{
    await set([link()]);const before=await ast();await open('编辑链接预览');const count=resolves.length;await dialog().getByLabel('链接地址').fill('javascript:alert(1)');await dialog().getByRole('button',{name:'获取网页信息'}).click();assert.equal(resolves.length,count);await dialog().getByRole('button',{name:'保存链接'}).click();assert.deepEqual(await ast(),before);
    await dialog().getByLabel('链接地址').fill('https://example.com/new');await dialog().getByRole('button',{name:'获取网页信息'}).click();await wait(async()=>assert.equal(await dialog().getByLabel('标题',{exact:true}).inputValue(),'网页标题'));assert.deepEqual(await ast(),before);assert.equal(await dialog().getByLabel('摘要',{exact:true}).inputValue(),'网页摘要');
    failResolve=true;await dialog().getByRole('button',{name:'获取网页信息'}).click();await page.getByText('无法获取网页信息',{exact:true}).waitFor();assert.equal(await dialog().getByLabel('标题',{exact:true}).inputValue(),'网页标题');failResolve=false;await dialog().getByLabel('标题',{exact:true}).fill('手动标题');await dialog().getByRole('button',{name:'保存链接'}).click();assert.equal((await ast()).children[0].title,'手动标题');
  });
  await check('canceling automatic metadata resolution prevents late saves',async()=>{
    await set([link()]);const before=await ast();await open('编辑链接预览');await dialog().getByLabel('标题',{exact:true}).fill('');resolveDelay=700;await dialog().getByRole('button',{name:'保存链接'}).click();await cancel();await page.waitForTimeout(850);resolveDelay=0;assert.deepEqual(await ast(),before);
  });
  await check('link preview clicks stay in the editor and remote changes cannot be overwritten',async()=>{
    await set([link()]);await open('编辑链接预览');const tabs=context.pages().length;await dialog().getByRole('link').click();assert.equal(context.pages().length,tabs);
    await root.evaluate(el=>el.editor.commands.command(({tr,dispatch})=>{if(dispatch)tr.setNodeMarkup(0,undefined,{...tr.doc.firstChild.attrs,title:'远端标题'});return true;}));
    await dialog().getByLabel('标题',{exact:true}).fill('旧草稿');await dialog().getByRole('button',{name:'保存链接'}).click();await page.getByText('此块已更新，请重新打开编辑，避免覆盖新内容。',{exact:true}).waitFor();assert.equal((await ast()).children[0].title,'远端标题');await cancel();
  });
  await check('component edits save in one step with server validation, fixed version and atomic undo',async()=>{
    await set([card()]);const before=await ast();await open('编辑知识组件');await dialog().getByLabel('title',{exact:true}).fill('修改后的配置');assert.equal(await dialog().getByLabel('已发布版本').inputValue(),'1');assert.deepEqual(await ast(),before);await dialog().getByRole('button',{name:'保存组件',exact:true}).click();
    await wait(async()=>assert.equal((await ast()).children[0].display_config.title,'修改后的配置'));const after=await ast();assert.equal(after.children[0].component_version,1);assert.equal(after.children[0].id,id(101));assert(previews.some(p=>p.path.includes('/versions/1/')&&p.props.title==='修改后的配置'));
    await root.getByText('资料卡预览：修改后的配置',{exact:true}).waitFor();await undo();assert.deepEqual(await ast(),before);await redo();assert.deepEqual(await ast(),after);
    await page.getByRole('button',{name:'重新载入当前内容'}).click();await root.waitFor();await root.getByText('资料卡预览：修改后的配置',{exact:true}).waitFor();assert.deepEqual(await ast(),after);
  });
  await check('component validation failures retain the draft and permit retry',async()=>{
    await set([card()]);const before=await ast();await open('编辑知识组件');await dialog().getByLabel('title',{exact:true}).fill('invalid');await dialog().getByRole('button',{name:'保存组件',exact:true}).click();await page.getByText('组件预览失败',{exact:true}).waitFor();assert.deepEqual(await ast(),before);assert.equal(await dialog().getByLabel('title',{exact:true}).inputValue(),'invalid');
    await dialog().getByLabel('title',{exact:true}).fill('有效配置');await dialog().getByRole('button',{name:'保存组件',exact:true}).click();await wait(async()=>assert.equal((await ast()).children[0].display_config.title,'有效配置'));
  });
  await check('complex component configuration is preserved and malformed JSON cannot save',async()=>{
    const config={title:'复杂配置',nested:{keep:[1,2]},extra:true};await set([card(config)]);const before=await ast();await open('编辑知识组件');const field=dialog().getByLabel('显示配置 JSON');assert.deepEqual(JSON.parse(await field.inputValue()),config);await field.fill('{broken');await dialog().getByRole('button',{name:'保存组件',exact:true}).click();assert.deepEqual(await ast(),before);await field.fill(JSON.stringify({...config,title:'更新复杂配置'}));await dialog().getByRole('button',{name:'保存组件',exact:true}).click();await wait(async()=>assert.equal((await ast()).children[0].display_config.title,'更新复杂配置'));assert.deepEqual((await ast()).children[0].display_config.nested,config.nested);
  });
  await check('component preview outages leave content intact and allow retry',async()=>{
    failPreview=true;await set([card({title:'失败后重试'})]);const before=await ast();await root.getByRole('button',{name:'重试预览'}).waitFor();assert.deepEqual(await ast(),before);failPreview=false;await root.getByRole('button',{name:'重试预览'}).click();await root.getByText('资料卡预览：失败后重试',{exact:true}).waitFor();assert.deepEqual(await ast(),before);
  });
  await check('canceling pending component validation prevents late writes',async()=>{
    await set([card()]);const before=await ast();await open('编辑知识组件');await dialog().getByLabel('title',{exact:true}).fill('不得写入');previewDelay=700;await dialog().getByRole('button',{name:'保存组件',exact:true}).click();await cancel();await page.waitForTimeout(850);previewDelay=0;assert.deepEqual(await ast(),before);
  });
  await check('remote component updates survive an in-flight validation',async()=>{
    await set([card()]);await open('编辑知识组件');await dialog().getByLabel('title',{exact:true}).fill('过期配置');previewDelay=700;await dialog().getByRole('button',{name:'保存组件',exact:true}).click();
    await root.evaluate(el=>el.editor.commands.command(({tr,dispatch})=>{if(dispatch)tr.setNodeMarkup(0,undefined,{...tr.doc.firstChild.attrs,displayConfig:JSON.stringify({title:'远端配置'})});return true;}));
    await page.getByText('此块已更新，请重新打开编辑，避免覆盖新内容。',{exact:true}).waitFor();assert.equal((await ast()).children[0].display_config.title,'远端配置');previewDelay=0;await cancel();
  });
  await check('unavailable pinned component versions do not silently change',async()=>{
    missingVersion=true;await page.reload();await root.waitFor();await set([card()]);const before=await ast();await open('编辑知识组件');await dialog().getByText('v1 暂不可用，请选择其他版本',{exact:true}).waitFor({state:'attached'});assert.equal(await dialog().getByRole('button',{name:'保存组件',exact:true}).count(),0);assert.deepEqual(await ast(),before);
    await dialog().getByLabel('已发布版本').selectOption('2');await dialog().getByLabel('title',{exact:true}).waitFor();assert.equal(await dialog().getByLabel('title',{exact:true}).inputValue(),'原配置');await dialog().getByRole('button',{name:'保存组件',exact:true}).click();await wait(async()=>assert.equal((await ast()).children[0].component_version,2));missingVersion=false;await page.reload();await root.waitFor();
  });
  await check('external insertion shares the form and reopening discards canceled drafts',async()=>{
    await set([{type:'paragraph'}]);const insert=async()=>{await root.click();await page.keyboard.insertText('/preview');await page.getByRole('button',{name:/外部内容/}).click();};
    await insert();await dialog().getByLabel('标题',{exact:true}).fill('未保存');await cancel();await insert();assert.equal(await dialog().getByLabel('标题',{exact:true}).inputValue(),'');await dialog().getByLabel('类型').selectOption('link_preview');await dialog().getByLabel('链接地址').fill('https://example.com/insert');await dialog().getByRole('button',{name:'插入',exact:true}).click();await wait(async()=>assert.equal((await ast()).children[0].title,'网页标题'));assert.equal((await ast()).children[0].type,'link_preview');
  });
  await check('mobile dialogs fit 320px, keep controls accessible and readonly hides editing',async()=>{
    await page.setViewportSize({width:320,height:844});await page.evaluate(()=>document.documentElement.classList.add('dark'));
    for(const [block,label] of [[link(),'编辑链接预览'],[card(),'编辑知识组件']]){
      await set([block]);await open(label);if(label==='编辑知识组件')await dialog().getByLabel('title',{exact:true}).waitFor();const box=await dialog().boundingBox();assert(box.width<=320&&box.x>=0&&box.y>=0&&box.y+box.height<=844);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));await dialog().screenshot({path:`/tmp/anby-links-mobile-${block.type}.png`,animations:'disabled'});await page.keyboard.press('Escape');await dialog().waitFor({state:'detached'});await root.evaluate(el=>el.editor.setEditable(false));assert.equal(await root.getByRole('button',{name:label,exact:true}).count(),0);await root.evaluate(el=>el.editor.setEditable(true));
    }
  });
  assert.deepEqual(errors,[]);console.log('PASS no browser runtime errors');
} finally {await browser.close();}
