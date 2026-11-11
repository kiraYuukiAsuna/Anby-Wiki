// Exercises the real reading routes against an isolated read-only API fixture.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {openSync,closeSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL=process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id=n=>`0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
const names={[id(30)]:'安比',[id(31)]:'狡兔屋'};
let failBindings=true,failDetail=false;const requests=[];
const pageData=(n,extra={})=>({page:{id:id(n),wiki_id:id(2),namespace_id:id(3),normalized_title:'anby',display_title:'安比的页面',language:'zh-CN',content_model:'block-v1',status:'active',current_revision_id:null,created_by:id(1),created_at:'2026-01-01',updated_at:'2026-01-01'},content:null,can_edit:n!==11&&n!==14,via_alias:false,...extra});
const api=createServer((req,res)=>{
  const u=new URL(req.url,'http://fixture'),p=u.pathname;requests.push({method:req.method,path:p,cookie:req.headers.cookie});
  const json=(value,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(value));};
  if(req.method!=='GET')return json({code:'fixture_read_only'},405);
  if(p.endsWith('/auth/session'))return json({actor_id:id(1),wiki_id:id(2),home_wiki_id:id(2),main_namespace_id:id(3),wiki_name:'测试 Wiki',actor_type:'human',display_name:'编辑者',method:'session',wikis:[]});
  if(p.endsWith('/entity-bindings')){
    const n=Number(p.split('/').at(-2).slice(-12));if(n===15&&failBindings)return json({code:'temporary_failure',message:'fixture failure'},503);
    const rows=n===10||n===15?[[31,'mentioned'],[30,'primary']]:n===11?[[31,'mentioned']]:n===12||n===16?[[30,'mentioned'],[31,'mentioned'],[30,'mentioned']]:[];
    return json({items:rows.map(([entity,role])=>({page_id:id(n),entity_id:id(entity),role,language:'zh-CN',created_at:'2026-01-01'}))});
  }
  if(/\/entities\/[^/]+\/graph$/.test(p)){
    const entity=p.split('/').at(-2);return json({root_id:entity,direction:'both',requested_depth:2,reached_depth:0,truncated:false,edges:[],nodes:[{id:entity,canonical_key:entity,status:'active',entity_type_key:'person',entity_type_name:'人物',label:names[entity],language:'zh-CN',description:'',depth:0}]});
  }
  if(/\/entities\/[^/]+$/.test(p)){
    const entity=p.split('/').at(-1);if(entity===id(31)&&failDetail)return json({code:'temporary_failure',message:'fixture failure'},503);
    return json({id:entity,canonical_key:names[entity],status:'active',entity_type:{id:id(40),name:'人物',type_key:'person'},labels:[{label:names[entity],language:'zh-CN',is_primary:true}],aliases:[]});
  }
  if(p.endsWith('/pages/by-title'))return json(pageData(10,{via_alias:true}));
  if(/\/pages\/[0-9a-f-]+$/.test(p)){
    const n=Number(p.split('/').at(-1).slice(-12));return json(n===17?pageData(10,{redirect:{from_page_id:id(17),from_title:'旧页面',hops:1,resolved:true,target:{kind:'page',page_id:id(10)}}}):pageData(n));
  }
  return json({items:[],ready:true});
});
await new Promise(resolve=>api.listen(0,'127.0.0.1',resolve));
const webDir=fileURLToPath(new URL('../',import.meta.url));
const log=openSync('/tmp/anby-page-graph-dev.log','w');
const next=spawn(process.execPath,['node_modules/next/dist/bin/next','dev','--hostname','127.0.0.1','--port',new URL(baseURL).port],{cwd:webDir,env:{...process.env,API_BASE_URL:`http://127.0.0.1:${api.address().port}`},detached:true,stdio:['ignore',log,log]});
let browser;
try {
  let ready=false;for(let i=0;i<100;i++){if(next.exitCode!==null)throw new Error('Next exited; see /tmp/anby-page-graph-dev.log');try{const r=await fetch(baseURL+'/login');if(r.ok){ready=true;break;}}catch{}await new Promise(resolve=>setTimeout(resolve,200));}assert(ready,'development server started');
  browser=await chromium.launch({channel:'chrome',headless:true});const context=await browser.newContext({viewport:{width:1280,height:900},hasTouch:true});await context.addCookies([{name:'anby_session',value:'page-graph-fixture',url:baseURL}]);
  const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  const check=async(name,fn)=>{await fn();console.log(`PASS ${name}`);};
  const visit=async n=>{await page.goto(`${baseURL}/pages/${id(n)}`);await page.getByRole('heading',{name:'安比的页面',exact:true}).waitFor();};
  const button=()=>page.getByRole('button',{name:'知识图谱',exact:true});
  const link=()=>page.getByRole('link',{name:'知识图谱',exact:true});
  await check('reading header opens the primary entity graph even when mentioned entities come first',async()=>{
    await visit(10);await link().waitFor();assert.equal(await link().getAttribute('href'),`/explore/graph?entity_id=${id(30)}`);await link().click();await page.waitForURL(`**/explore/graph?entity_id=${id(30)}`);await page.getByRole('img',{name:/实体关系图/}).waitFor();assert(requests.some(r=>r.path===`/api/v1/entities/${id(30)}/graph`));
  });
  await check('one associated entity opens directly for read-only readers',async()=>{
    await visit(11);await link().waitFor();assert.equal(await link().getAttribute('href'),`/explore/graph?entity_id=${id(31)}`);assert.equal(await page.getByRole('link',{name:'编辑',exact:true}).count(),0);
  });
  await check('multiple associations offer deduplicated named choices and open the selected graph',async()=>{
    await visit(12);await button().click();const dialog=page.getByRole('dialog');await dialog.getByRole('link',{name:/安比/}).waitFor();assert.equal(await dialog.getByRole('link',{name:/安比/}).count(),1);assert(!(await dialog.innerText()).includes(id(30)));await dialog.getByRole('link',{name:/狡兔屋/}).click();await page.waitForURL(`**/explore/graph?entity_id=${id(31)}`);await page.getByRole('img',{name:/实体关系图/}).waitFor();
  });
  await check('unbound pages explain the empty state and link editors to the existing binding section',async()=>{
    await visit(13);await button().click();const dialog=page.getByRole('dialog');await dialog.getByText('这个页面还没有关联实体。',{exact:true}).waitFor();assert.equal(await dialog.getByRole('link',{name:'关联实体',exact:true}).getAttribute('href'),`/pages/${id(13)}/tools#page-entities`);assert.equal(await dialog.getByRole('link',{name:'浏览知识图谱'}).getAttribute('href'),'/explore/graph');
  });
  await check('read-only empty pages retain browsing without offering binding edits',async()=>{
    await visit(14);await button().click();const dialog=page.getByRole('dialog');await dialog.getByText('这个页面还没有关联实体。',{exact:true}).waitFor();assert.equal(await dialog.getByRole('link',{name:'关联实体',exact:true}).count(),0);
  });
  await check('binding failures show retry rather than a false empty result',async()=>{
    await visit(15);await button().click();const dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'重试',exact:true}).waitFor();assert((await dialog.innerText()).includes('页面关联实体加载失败'));assert.equal(await dialog.getByText('这个页面还没有关联实体。',{exact:true}).count(),0);failBindings=false;await dialog.getByRole('button',{name:'重试',exact:true}).click();await link().waitFor();assert.equal(await link().getAttribute('href'),`/explore/graph?entity_id=${id(30)}`);
  });
  await check('one unavailable entity does not block other choices and can be retried',async()=>{
    failDetail=true;await visit(16);await button().click();const dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'重试',exact:true}).waitFor();assert((await dialog.innerText()).includes('实体暂时无法读取'));await dialog.getByRole('link',{name:/安比/}).waitFor();failDetail=false;await dialog.getByRole('button',{name:'重试',exact:true}).click();await dialog.getByRole('link',{name:/狡兔屋/}).waitFor();
  });
  await check('redirects use the displayed destination page and title routes share the graph entry',async()=>{
    await visit(17);await link().waitFor();assert.equal(await link().getAttribute('href'),`/explore/graph?entity_id=${id(30)}`);await page.goto(baseURL+'/wiki/'+encodeURIComponent('安比'));await link().waitFor();assert.equal(await link().getAttribute('href'),`/explore/graph?entity_id=${id(30)}`);
  });
  await check('mobile and keyboard users can open and dismiss the entity chooser without page overflow',async()=>{
    await page.setViewportSize({width:320,height:844});await visit(12);await button().tap();const dialog=page.getByRole('dialog');await dialog.getByRole('link',{name:/安比/}).waitFor();const box=await dialog.boundingBox();assert(box.x>=0&&box.x+box.width<=320);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:'/tmp/anby-page-graph-mobile.png',animations:'disabled'});await page.keyboard.press('Escape');await dialog.waitFor({state:'detached'});await button().focus();await page.keyboard.press('Enter');await dialog.waitFor();await page.keyboard.press('Escape');await page.setViewportSize({width:1280,height:900});await visit(10);await link().waitFor();await page.screenshot({path:'/tmp/anby-page-graph-desktop.png',animations:'disabled'});
  });
  assert(requests.every(r=>r.method==='GET'),'graph entry never writes data');assert(requests.filter(r=>/\/pages\/[0-9a-f-]+$/.test(r.path)).every(r=>r.cookie?.includes('anby_session=page-graph-fixture')),'SSR forwards the session');assert.deepEqual(errors,[]);console.log('PASS read-only API flow, authenticated SSR and no runtime errors');
}finally{
  await browser?.close();try{process.kill(-next.pid,'SIGTERM');}catch{}await new Promise(resolve=>api.close(resolve));closeSync(log);
}
