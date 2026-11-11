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
  const context=await browser.newContext({viewport:{width:1440,height:1000},hasTouch:true});
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
  await page.addStyleTag({content:'*,*::before,*::after{transition:none !important;animation:none !important}nextjs-portal{display:none}'});
  // Give the article its own track; the fixture's AST/debug panels are not article content.
  await root.evaluate(el=>{const grid=el.closest('.grid');grid.style.gridTemplateColumns='minmax(0,1fr)';for(const section of [...grid.querySelectorAll(':scope > section')].slice(1))section.style.display='none';});
  const measure=locator=>locator.evaluate(el=>[el,...el.querySelectorAll('*')].map((node,index)=>{
    const style=getComputedStyle(node),rect=node.getBoundingClientRect();
    const properties=['fontFamily','fontSize','lineHeight','fontWeight','letterSpacing','minHeight','padding','display','gridTemplateColumns','gap','color','backgroundColor','borderColor','borderRadius','textDecorationLine'];
    return {tag:node.tagName,text:node.childElementCount?undefined:node.textContent,...Object.fromEntries(properties.map(key=>[key,style[key]])),...(index?{margin:style.margin}:{}),width:Math.round(rect.width*100)/100,height:Math.round(rect.height*100)/100};
  }));
  // Compare whole documents without overriding either card's dimensions or placement.
  const prose='安比是一位性格沉稳的少女。她喜欢电影，也习惯用电影里的故事解释生活。';
  const paragraph=n=>({type:'paragraph',attrs:{id:id(n)},content:[{type:'text',text:prose.repeat(8)}]});
  const component={type:'componentBlock',attrs:{id:id(100),componentId:id(10),componentVersion:1,entityId:id(20),displayConfig:'{"title":"安比"}'}};
  const articleLayout=locator=>locator.evaluate(el=>{
    const card=el.querySelector('.entity-infobox').getBoundingClientRect(),article=el.getBoundingClientRect();
    const paragraph=el.querySelector(':scope > p[data-after-card]')??el.querySelector(':scope > .node-componentBlock + p');
    const range=document.createRange();range.selectNodeContents(paragraph);const lines=[...range.getClientRects()];
    const round=n=>Math.round(n*100)/100;
    return {rightGap:round(article.right-card.right),cardTop:round(card.top-article.top),cardWidth:round(card.width),cardHeight:round(card.height),
      lines:lines.map(r=>[round(r.left-article.left),round(r.top-article.top),round(r.width)]),
      textBesideCard:lines.some(r=>r.top<card.bottom&&r.bottom>card.top&&r.right<=card.left),
      textBelowCard:lines.some(r=>r.top>card.bottom&&r.right>card.left)};
  });
  for(const [viewport,width,intro] of [[1440,640,false],[1440,960,true],[1440,320,true],[320,240,false],[720,640,true],[721,640,false]]){
    await page.setViewportSize({width:viewport,height:1000});
    await page.getByRole('button',{name:'空白文档'}).click();
    await root.evaluate((el,{width,content})=>{el.style.width=width+'px';el.editor.chain().setContent({type:'doc',content}).command(({tr})=>{tr.setMeta('addToHistory',false);return true;}).run();},{width,content:[...(intro?[paragraph(99)]:[]),component,paragraph(101),paragraph(102)]});
    await root.locator('.entity-infobox').waitFor();
    await page.evaluate(({html,text,width,intro})=>{const reader=document.createElement('div');reader.id='component-article-fixture';reader.className='wiki-rendered';reader.style.width=width+'px';reader.innerHTML=(intro?`<p>${text}</p>`:'')+html+`<p data-after-card>${text}</p><p>${text}</p>`;document.body.append(reader);},{html:fixtures[0].html,text:prose.repeat(8),width,intro});
    const reader=page.locator('#component-article-fixture');
    const expected=await articleLayout(reader),actual=await articleLayout(root);
    assert.deepEqual(actual,expected,`whole article at ${width}px, viewport ${viewport}, preceding prose ${intro}`);
    assert.equal(actual.textBesideCard,viewport>720,'prose wraps beside desktop infoboxes only');
    if(viewport>720&&width<960)assert(actual.textBelowCard,'long prose resumes at full width below the card');
    const card=await root.locator('.entity-infobox').boundingBox(),surface=await root.boundingBox();assert(card.y+card.height<=surface.y+surface.height,'editor contains its floating card');
    if(viewport<=720)await root.locator('.entity-infobox').tap({position:{x:10,y:5}});else await root.locator('.entity-infobox').click({position:{x:10,y:5}});await page.mouse.move(0,0);assert.equal(await root.locator('.node-componentBlock.ProseMirror-selectednode').count(),1);assert.equal(await root.locator('.ast-editable-block__actions').evaluate(el=>getComputedStyle(el).opacity),'1','selecting the card exposes its edit action, including on touch');await root.getByRole('button',{name:'编辑知识组件',exact:true}).click();await page.getByRole('dialog').last().getByRole('button',{name:'取消',exact:true}).click();
    await page.locator('[role=dialog]').waitFor({state:'detached'});
    await root.locator(':scope > .node-componentBlock + p').click();
    const before=await root.evaluate(el=>el.editor.getJSON());
    await root.evaluate(el=>{let pos;el.editor.state.doc.descendants((node,offset)=>{if(node.attrs.id=== '0198a1b2-c3d4-7e5f-8a9b-000000000101')pos=offset+1;});el.editor.commands.setTextSelection(pos);el.editor.view.focus();});
    await page.keyboard.insertText('继续编辑');await page.keyboard.press('Enter');
    assert((await root.innerText()).includes('继续编辑'));await root.evaluate(el=>el.editor.commands.undo());assert.deepEqual(await root.evaluate(el=>el.editor.getJSON()),before,'typing/Enter beside the card undo without changing component content');
    await page.mouse.move(0,0);await root.screenshot({path:`/tmp/anby-component-flow-editor-${viewport}-${width}.png`,animations:'disabled'});await reader.screenshot({path:`/tmp/anby-component-flow-reader-${viewport}-${width}.png`,animations:'disabled'});
    await reader.evaluate(el=>el.remove());await root.evaluate(el=>el.style.removeProperty('width'));
    console.log(`PASS whole article flow, editing and undo at ${width}px, viewport ${viewport}, preceding prose ${intro}`);
  }
  await page.setViewportSize({width:1440,height:1000});
  await page.getByRole('button',{name:'空白文档'}).click();
  await root.evaluate((el,{component,paragraph})=>el.editor.commands.setContent({type:'doc',content:[{type:'columnList',content:[{type:'column',attrs:{widthRatio:1},content:[component,paragraph]},{type:'column',attrs:{widthRatio:1},content:[{...component,attrs:{...component.attrs,id:'0198a1b2-c3d4-7e5f-8a9b-000000000200'}},paragraph]}]}]}),{component,paragraph:paragraph(201)});
  await root.locator('.entity-infobox').first().waitFor();
  for(const column of await root.locator('.ast-column-content > [data-node-view-content-react]').all()){
    const layout=await articleLayout(column);assert.equal(layout.rightGap,0);assert(layout.textBesideCard);
    const bounds=await column.boundingBox(),card=await column.locator('.entity-infobox').boundingBox();assert(card.y+card.height<=bounds.y+bounds.height,'floats stay within their column');
  }
  console.log('PASS multiple infoboxes stay right aligned and contained within their own columns');
  await page.getByRole('button',{name:'空白文档'}).click();
  await root.evaluate((el,component)=>el.editor.commands.setContent({type:'doc',content:[{...component,attrs:{...component.attrs,componentId:'0198a1b2-c3d4-7e5f-8a9b-000000000011'}}]}),component);
  await root.locator('[data-component-renderer="key-value"]').waitFor();
  assert.equal(await root.locator('.node-componentBlock').evaluate(el=>getComputedStyle(el).float),'none');
  assert.equal((await root.locator('[data-component-renderer="key-value"]').boundingBox()).width,(await root.boundingBox()).width);
  await root.getByRole('button',{name:'编辑知识组件',exact:true}).focus();assert.equal(await root.locator('.ast-editable-block__actions').evaluate(el=>getComputedStyle(el).opacity),'1');
  console.log('PASS key-value cards keep full width and keyboard focus exposes the edit action');
  for(const fixture of fixtures){
    await page.getByRole('button',{name:'空白文档'}).click();await root.waitFor();
    await root.evaluate((el,attrs)=>el.editor.commands.setContent({type:'doc',content:[{type:'componentBlock',attrs}]}),{id:id(100),componentId:id(fixture.n),componentVersion:1,entityId:id(20),displayConfig:'{"title":"安比"}'});
    const editorCard=root.locator('[data-component-renderer]');await editorCard.waitFor();
    // Match ServerRenderedDocument's wrapper to exercise the reader's actual CSS.
    await page.evaluate(html=>{const reader=document.createElement('div');reader.id='component-reader-fixture';reader.className='wiki-rendered';reader.innerHTML=html;document.body.append(reader);},fixture.html);
    const readerCard=page.locator('#component-reader-fixture [data-component-renderer]');
    await root.locator('.ast-component').hover();await root.getByRole('button',{name:'编辑知识组件',exact:true}).click();const dialog=page.getByRole('dialog').last();await dialog.getByRole('button',{name:'运行预览'}).click();const dialogCard=dialog.locator('[data-component-renderer]');await dialogCard.waitFor();
    for(const dark of [false,true])for(const width of [240,320,640]){
      await page.evaluate(dark=>document.documentElement.classList.toggle('dark',dark),dark);
      // Equal available width separates typography and column rules from page chrome.
      for(const card of [editorCard,dialogCard,readerCard])await card.evaluate((el,width)=>{el.parentElement.style.width=width+'px';el.parentElement.style.maxWidth='none';if(el.classList.contains('entity-infobox'))el.style.width=Math.min(width,352)+'px';},width);
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
    for(const card of [editorCard,dialogCard,readerCard])await card.evaluate(el=>{el.parentElement.style.width='240px';if(el.classList.contains('entity-infobox'))el.style.width='240px';});
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
