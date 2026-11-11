// Run /dev/editor with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL=process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id=n=>`0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
// Matches the two fixed backend renderer structures, including long wrapping values.
const fixtures=[
  {n:10,renderer:'builtin.entity_claim_infobox',html:`<aside class="entity-infobox" data-component-renderer="entity-claim-infobox" data-entity-type="person"><header class="entity-infobox-header"><p class="entity-infobox-type">人物</p><h2>安比的角色资料</h2></header><dl><div data-property-key="name"><dt>名称</dt><dd><span class="entity-infobox-value">安比·德玛拉</span></dd></div><div data-property-key="faction"><dt>所属阵营</dt><dd><span class="entity-infobox-value"><a href="/entities/${id(20)}">狡兔屋与一个需要换行的很长名称</a></span></dd></div></dl><footer class="entity-infobox-verification">1 条事实待核验</footer></aside>`},
  {n:11,renderer:'builtin.key_value',html:'<dl data-component-renderer="key-value"><dt>名称</dt><dd>安比·德玛拉</dd><dt>所属阵营</dt><dd>狡兔屋与一个需要换行的很长名称</dd></dl>'}
];
const browser=await chromium.launch({channel:'chrome',headless:true});
try {
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addCookies([{name:'anby_session',value:'editor-fixture',url:baseURL}]);
  const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/*',route=>{
    const u=new URL(route.request().url()),p=u.pathname;if(u.origin!==baseURL)return route.fulfill({body:''});if(!p.startsWith('/api/'))return route.continue();
    const json=value=>route.fulfill({json:value});
    if(p.endsWith('/auth/session'))return json({actor_id:id(1),wiki_id:id(2),home_wiki_id:id(2),main_namespace_id:id(3),wiki_name:'测试 Wiki',actor_type:'human',display_name:'编辑者',method:'session',wikis:[]});
    const fixture=fixtures.find(f=>p.includes(id(f.n)))??fixtures[0];
    if(/components\/[^/]+\/versions\/\d+\/preview$/.test(p))return json({html:fixture.html});
    if(/components\/[^/]+\/versions$/.test(p))return json({items:[{component_id:id(fixture.n),version:1,props_schema:{type:'object',properties:{title:{type:'string',default:'安比'}}},renderer_ref:fixture.renderer,status:'published',created_by:id(1),created_at:'2026-01-01'}]});
    if(/components\/[^/]+$/.test(p))return json({id:id(fixture.n),name:'资料卡',component_key:'card',created_by:id(1),created_at:'2026-01-01',updated_at:'2026-01-01'});
    if(/entities\/[^/]+$/.test(p))return json({id:id(20),display_label:'安比',canonical_key:'anby',status:'active',entity_type:{id:id(21),name:'人物',type_key:'person'},labels:[{label:'安比',is_primary:true}],aliases:[]});
    return json({items:[]});
  });
  await page.goto(baseURL+'/dev/editor');const root=page.locator('[data-block-editor]').first().locator('.tiptap').first();await root.waitFor();
  await page.addStyleTag({content:'*,*::before,*::after{transition:none !important;animation:none !important}'});
  const measure=locator=>locator.evaluate(el=>[el,...el.querySelectorAll('*')].map((node,index)=>{
    const style=getComputedStyle(node),rect=node.getBoundingClientRect();
    const properties=['fontFamily','fontSize','lineHeight','fontWeight','letterSpacing','minHeight','padding','display','gridTemplateColumns','gap','color','backgroundColor','borderColor','borderRadius','textDecorationLine'];
    return {tag:node.tagName,text:node.childElementCount?undefined:node.textContent,...Object.fromEntries(properties.map(key=>[key,style[key]])),...(index?{margin:style.margin}:{}),width:Math.round(rect.width*100)/100,height:Math.round(rect.height*100)/100};
  }));
  for(const fixture of fixtures){
    await page.getByRole('button',{name:'空白文档'}).click();await root.waitFor();
    await root.evaluate((el,attrs)=>el.editor.commands.setContent({type:'doc',content:[{type:'componentBlock',attrs}]}),{id:id(100),componentId:id(fixture.n),componentVersion:1,entityId:id(20),displayConfig:'{"title":"安比"}'});
    const editorCard=root.locator('[data-component-renderer]');await editorCard.waitFor();
    // Match ServerRenderedDocument's wrapper to exercise the reader's actual CSS.
    await page.evaluate(html=>{const reader=document.createElement('div');reader.id='component-reader-fixture';reader.className='wiki-rendered';reader.innerHTML=html;document.body.append(reader);},fixture.html);
    const readerCard=page.locator('#component-reader-fixture [data-component-renderer]');
    await root.getByRole('button',{name:'编辑知识组件',exact:true}).click();const dialog=page.getByRole('dialog').last();await dialog.getByRole('button',{name:'运行预览'}).click();const dialogCard=dialog.locator('[data-component-renderer]');await dialogCard.waitFor();
    for(const dark of [false,true])for(const width of [240,320,640]){
      await page.evaluate(dark=>document.documentElement.classList.toggle('dark',dark),dark);
      // Equal available width separates typography and column rules from page chrome.
      for(const card of [editorCard,dialogCard,readerCard])await card.evaluate((el,width)=>{el.parentElement.style.width=width+'px';el.parentElement.style.maxWidth='none';},width);
      const editor=await measure(editorCard),preview=await measure(dialogCard);
      if(fixture.n===10)assert.equal(editor[0].width,Math.min(width,352),'infobox previews share the reader card maximum width');
      // Article floats keep their placement; compare the same card width inside that layout.
      await readerCard.evaluate((el,width)=>{el.style.width=width+'px';},editor[0].width);
      const reader=await measure(readerCard);
      assert.deepEqual(editor,preview,`${fixture.renderer}: editor/preview at ${width}px, dark=${dark}`);
      assert.deepEqual(editor,reader,`${fixture.renderer}: editor/reader at ${width}px, dark=${dark}`);
      console.log(`PASS ${fixture.renderer} matches editor, preview and reader at ${width}px, dark=${dark}`);
    }
    await page.setViewportSize({width:320,height:844});
    for(const card of [editorCard,dialogCard,readerCard])await card.evaluate(el=>{el.parentElement.style.width='240px';});
    await readerCard.evaluate(el=>{el.style.width='240px';});
    assert.deepEqual(await measure(editorCard),await measure(dialogCard));assert.deepEqual(await measure(editorCard),await measure(readerCard));
    await dialogCard.screenshot({path:`/tmp/anby-component-layout-preview-${fixture.n}.png`,animations:'disabled'});
    await dialog.getByRole('button',{name:'取消',exact:true}).click();
    await editorCard.screenshot({path:`/tmp/anby-component-layout-editor-${fixture.n}.png`,animations:'disabled'});
    await page.locator('#component-reader-fixture').evaluate(el=>el.remove());
    await page.setViewportSize({width:1440,height:1000});await page.evaluate(()=>document.documentElement.classList.remove('dark'));
    console.log(`PASS ${fixture.renderer} matches at a 320px viewport`);
  }
  await page.getByRole('button',{name:'空白文档'}).click();await root.waitFor();
  await root.evaluate(el=>el.editor.commands.setContent({"type": "doc", "content": [{"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "正文标题"}]}, {"type": "paragraph", "content": [{"type": "text", "text": "正文链接", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}]}]}));
  assert.deepEqual(await root.locator('h2').evaluate(el=>{const s=getComputedStyle(el);return[s.fontSize,s.fontWeight];}),['24px','700']);
  assert.equal(await root.locator('p').evaluate(el=>getComputedStyle(el).minHeight),'24px');
  assert.equal(await root.locator('a').evaluate(el=>getComputedStyle(el).textDecorationLine),'underline');
  console.log('PASS normal editor headings, paragraphs and links retain their typography');
  assert.deepEqual(errors,[]);console.log('PASS no browser runtime errors');
}finally{await browser.close();}
