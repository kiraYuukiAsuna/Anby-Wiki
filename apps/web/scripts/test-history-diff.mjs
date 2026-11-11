// Real history routes with an isolated, read-only API fixture.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {openSync,closeSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL=process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id=n=>`0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
const p=(n,text)=>({id:id(n),type:'paragraph',content:[{type:'text',text}]});
const table=value=>({id:id(20),type:'table',has_column_header:true,children:[{id:id(21),type:'table_row',children:[{id:id(22),type:'table_cell',children:[p(23,value)]}]}]});
const toggle=value=>({id:id(30),type:'toggle',content:[{type:'text',text:'折叠摘要'}],children:[p(31,value)]});
const component={id:id(40),type:'component',component_id:id(41),component_version:1,entity_id:id(42),display_config:{}};
const columns={id:id(60),type:'column_list',children:[{id:id(61),type:'column',width_ratio:1,children:[p(62,'左栏正文')]},{id:id(63),type:'column',width_ratio:1,children:[p(64,'右栏正文')]}]};
const heading={id:id(10),type:'heading',level:2,content:[{type:'text',text:'安比 · 资料'}]};
const long=Array.from({length:18},(_,i)=>p(100+i,`第 ${i+1} 段未修改的上下文，保留页面阅读排版。`.repeat(4)));
const before={type:'document',schema_version:1,children:[heading,component,p(11,'负责记录作战资料'),table('待确认'),toggle('原来的折叠内容'),p(50,'将被删除的内容'),columns,...long]};
const after={...before,children:[heading,component,p(11,'负责整理与校对作战资料'),toggle('新的折叠内容'),p(51,'新增的资料来源'),table('已校对'),columns,...long]};
const change=(n,type,extra={})=>({block_id:id(n),parent_id:'',type,...extra});
const diff={changes:[change(11,'changed',{fields:[{field:'content',before:'记录',after:'整理与校对'}]}),change(23,'changed',{fields:[{field:'content',before:'待确认',after:'已校对'}]}),change(31,'changed',{fields:[{field:'content',before:'原来',after:'新'}]}),change(50,'removed',{path:[5]}),change(51,'added',{path:[4]}),...before.children.map((b,i)=>({block_id:b.id,parent_id:'',type:'moved',before_path:[i],after_path:[i+1]}))]};
const render=b=>{
 const attr=`data-history-block-id="${b.id}"`,children=()=>b.children?.map(render).join('')||'',text=()=>b.content?.map(n=>n.text||'').join('')||'';
 switch(b.type){
  case'paragraph':return `<p ${attr}><strong>${text()}</strong></p>`;
  case'heading':return `<h2 ${attr} id="${b.id}">${text()}</h2>`;
  case'table':return `<figure ${attr} class="wiki-table"><table><thead>${children()}</thead></table></figure>`;
  case'table_row':return `<tr ${attr}>${children()}</tr>`;
  case'table_cell':return `<th ${attr} scope="col">${children()}</th>`;
  case'toggle':return `<details ${attr} class="toggle"><summary>${text()}</summary>${children()}</details>`;
  case'component':return `<aside ${attr} class="entity-infobox"><div class="entity-infobox-header"><h2>安比</h2></div><dl><div><dt>所属组织</dt><dd>狡兔屋</dd></div></dl></aside>`;
  case'column_list':return `<div ${attr} class="column-list">${children()}</div>`;
  case'column':return `<section ${attr} class="column" style="--column-ratio:1">${children()}</section>`;
 }
};
const revision=n=>({id:id(n),page_id:id(1),content_snapshot_id:id(n+10),actor_id:id(2),actor_display_name:n===3?'小林':'小陈',summary:'资料校对',created_at:n===3?'2026-11-10T00:00:00Z':'2026-11-11T00:00:00Z',is_minor:false});
let failPreview=false;const requests=[];
const api=createServer((req,res)=>{
 const u=new URL(req.url,'http://fixture'),path=u.pathname;requests.push({method:req.method,path,query:u.search,cookie:req.headers.cookie});
 const json=(value,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(value));};
 if(req.method!=='GET')return json({code:'read_only'},405);
 if(path.endsWith('/auth/session'))return json({actor_id:id(2),wiki_id:id(7),home_wiki_id:id(7),main_namespace_id:id(8),wiki_name:'测试 Wiki',actor_type:'human',display_name:'编辑者',method:'session',wikis:[]});
 if(/\/pages\/[0-9a-f-]+$/.test(path))return json({page:{id:id(1),wiki_id:id(7),namespace_id:id(8),normalized_title:'anby',display_title:'安比资料',language:'zh-CN',content_model:'block-v1',status:'active',current_revision_id:id(4),created_by:id(2),created_at:'2026-01-01',updated_at:'2026-01-01'},content:null,can_edit:true,via_alias:false});
 if(path.endsWith('/diff'))return json({changes:u.searchParams.get('from')===u.searchParams.get('to')?[]:diff.changes});
 if(/\/revisions\/[0-9a-f-]+$/.test(path)){
  const rid=path.split('/').at(-1);if(![id(3),id(4)].includes(rid))return json({code:'not_found'},404);
  const n=rid===id(3)?3:4,doc=n===3?before:after;
  return json({revision:revision(n),ast_json:doc,...(u.searchParams.get('include_render')==='true'?failPreview&&n===3?{render_error:true}:{html:doc.children.map(render).join(''),renderer_version:'v13'}:{})});
 }
 return json({items:[]});
});
await new Promise(resolve=>api.listen(0,'127.0.0.1',resolve));
const log=openSync('/tmp/anby-history-dev.log','w');
const next=spawn(process.execPath,['node_modules/next/dist/bin/next','dev','--hostname','127.0.0.1','--port',new URL(baseURL).port],{cwd:fileURLToPath(new URL('../',import.meta.url)),env:{...process.env,API_BASE_URL:`http://127.0.0.1:${api.address().port}`},detached:true,stdio:['ignore',log,log]});
let browser;
try{
 let ready=false;for(let i=0;i<120;i++){if(next.exitCode!==null)throw new Error('Next failed; see /tmp/anby-history-dev.log');try{if((await fetch(baseURL+'/login')).ok){ready=true;break;}}catch{}await new Promise(r=>setTimeout(r,200));}assert(ready);
 browser=await chromium.launch({channel:'chrome',headless:true});const context=await browser.newContext({viewport:{width:1440,height:1000},hasTouch:true});await context.addCookies([{name:'anby_session',value:'history-fixture',url:baseURL}]);
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const visit=async(from=3,to=4)=>{await page.goto(`${baseURL}/pages/${id(1)}/diff?from=${id(from)}&to=${id(to)}`);};
 const section=()=>page.getByRole('region',{name:'版本对比'}),nav=()=>page.getByRole('navigation',{name:'变更位置'});
 const side=n=>page.locator(`[data-history-side="${n}"]`),block=(sideName,n)=>side(sideName).locator(`[data-history-block-id="${id(n)}"]`);
 const check=async(name,fn)=>{await fn();console.log(`PASS ${name}`);};
 await check('default visual mode removes passive moves',async()=>{
  await visit();await block('before',11).waitFor();assert.equal(await section().getByRole('button',{name:'页面对比',exact:true}).getAttribute('aria-pressed'),'true');
  assert.equal(await page.locator('[data-change-count="moved"]').innerText(),'移动 1');assert.equal(await page.locator('[data-change-count="changed"]').innerText(),'修改 3');assert.equal(await nav().getByRole('button').count(),6);
  await page.getByText('引用数据按当前状态展示；对比的是各版本保存的文字、布局和组件配置。',{exact:true}).waitFor();
 });
 await check('Chinese highlights preserve markup, table cells and expanded toggles',async()=>{
  assert.equal(await block('before',11).locator('strong mark').innerText(),'记录');assert.equal(await block('after',11).locator('strong mark').innerText(),'整理与校对');
  assert.equal(await block('before',22).locator('mark').innerText(),'待确认');assert.equal(await block('after',30).getAttribute('open'),'');
  const ids=await page.locator('[data-history-document] [id]').evaluateAll(els=>els.map(el=>el.id));assert.equal(new Set(ids).size,ids.length);
 });
 await check('navigation selects both versions and explains missing counterparts',async()=>{
  await nav().getByRole('button',{name:/新增的资料来源/}).click();await page.getByText('此处内容在修改后新增',{exact:true}).waitFor();assert.equal(await block('after',51).getAttribute('data-history-selected'),'');
  await nav().getByRole('button',{name:/将被删除的内容/}).click();await page.getByText('此处内容已在修改后删除',{exact:true}).waitFor();
  await nav().getByRole('button',{name:/表格 · 单元格/}).click();assert.equal(await block('before',23).getAttribute('data-history-selected'),'');assert.equal(await block('after',23).getAttribute('data-history-selected'),'');
 });
 await check('JSON retains raw changes; switching restores selection and text diff',async()=>{
  await section().getByRole('button',{name:'结构详情 · JSON'}).click();assert.equal(await section().locator('[data-change-type="moved"]').count(),before.children.length);
  await section().getByRole('button',{name:'页面对比',exact:true}).click();await block('after',11).locator('mark').waitFor();assert.equal(await block('after',23).getAttribute('data-history-selected'),'');
 });
 await check('filter keeps table geometry and heading context',async()=>{
  await page.getByLabel('仅看变更',{exact:true}).check();assert.equal(await block('before',100).isVisible(),false);assert.equal(await block('before',22).isVisible(),true);assert.equal(await block('before',10).isVisible(),true);await page.getByLabel('仅看变更',{exact:true}).uncheck();assert.equal(await block('before',100).isVisible(),true);
 });
 await check('knowledge card float and column layout match reading styles',async()=>{
  const card=await block('after',40).boundingBox(),text=await block('after',11).boundingBox();assert(card.x>text.x);
  const left=await block('after',61).boundingBox(),right=await block('after',63).boundingBox();assert(right.x>left.x);
  await page.screenshot({path:'/tmp/anby-history-desktop.png',animations:'disabled'});
 });
 await check('linked scrolling follows common blocks and can be disabled',async()=>{
  const beforePane=side('before').getByLabel('修改前页面'),afterPane=side('after').getByLabel('修改后页面');
  await beforePane.evaluate(el=>{el.scrollTop=el.scrollHeight/2;});await page.waitForFunction(()=>document.querySelector('[aria-label="修改后页面"]').scrollTop>100);
  await page.getByLabel('联动滚动',{exact:true}).uncheck();const position=await afterPane.evaluate(el=>el.scrollTop);await beforePane.evaluate(el=>{el.scrollTop=0;});await page.waitForTimeout(80);assert.equal(await afterPane.evaluate(el=>el.scrollTop),position);
 });
 await check('mobile toggle preserves selected block without 320px overflow',async()=>{
  for(const width of [390,320]){
   await page.setViewportSize({width,height:900});await nav().getByRole('button',{name:/表格 · 单元格/}).click();await page.getByRole('button',{name:'修改前',exact:true}).click();assert(await side('before').isVisible());assert.equal(await side('after').isVisible(),false);assert.equal(await block('before',23).getAttribute('data-history-selected'),'');
   await page.getByRole('button',{name:'修改后',exact:true}).click();assert(await side('after').isVisible());assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   await page.evaluate(()=>document.documentElement.classList.add('dark'));await page.screenshot({path:`/tmp/anby-history-mobile-${width}.png`,animations:'disabled'});
  }
 });
 await check('same revision verifies existence before showing an empty diff',async()=>{
  await visit(3,3);await page.getByText('两版相同，没有内容或块顺序变化。',{exact:true}).waitFor();assert.equal(await nav().count(),0);
  await page.goto(`${baseURL}/pages/${id(1)}/diff?from=${id(99)}&to=${id(99)}`);await page.getByRole('heading',{name:'页面不存在',exact:true}).waitFor();assert.equal(await section().count(),0);
 });
 await check('preview failure retries independently',async()=>{
  await page.setViewportSize({width:1440,height:1000});failPreview=true;await visit();await page.getByRole('button',{name:'重试预览',exact:true}).waitFor();assert(await block('after',11).isVisible());failPreview=false;await page.getByRole('button',{name:'重试预览',exact:true}).click();await block('before',11).waitFor();assert.equal(await block('before',11).locator('mark').innerText(),'记录');
 });
 assert(requests.every(r=>r.method==='GET'));assert(requests.filter(r=>r.path.includes('/revisions/')).every(r=>r.cookie?.includes('anby_session=history-fixture')));assert(requests.some(r=>r.query.includes('include_render=true')));assert.deepEqual(errors,[]);console.log('PASS authenticated read-only previews and no browser runtime errors');
}finally{await browser?.close();try{process.kill(-next.pid,'SIGTERM');}catch{}await new Promise(resolve=>api.close(resolve));closeSync(log);}
