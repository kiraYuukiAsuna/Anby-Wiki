// Run the development server with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL=process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id=n=>`0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12,'0')}`;
const browser=await chromium.launch({channel:'chrome',headless:true});
try {
  const context=await browser.newContext({viewport:{width:1280,height:900},hasTouch:true});
  await context.addCookies([{name:'anby_session',value:'collaboration-fixture',url:baseURL}]);
  const page=await context.newPage(),errors=[],outgoing=[],sockets=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.clock.install();
  await page.route('**/*',route=>{
    const u=new URL(route.request().url()),p=u.pathname;if(u.origin!==baseURL)return route.fulfill({body:''});if(!p.startsWith('/api/'))return route.continue();
    if(p.endsWith('/auth/session'))return route.fulfill({json:{actor_id:id(1),wiki_id:id(2),home_wiki_id:id(2),main_namespace_id:id(3),wiki_name:'测试 Wiki',actor_type:'human',display_name:'当前编辑者',method:'session',wikis:[]}});
    if(/pages\/[^/]+$/.test(p))return route.fulfill({json:{page:{id:id(10),display_title:'协作成员显示验证'},content:null,can_edit:true,via_alias:false}});
    return route.fulfill({json:{items:[]}});
  });
  await page.routeWebSocket(/\/collaboration\?/,socket=>{
    sockets.push(socket);socket.onMessage(message=>{if(typeof message==='string')outgoing.push(JSON.parse(message));});
    socket.send(JSON.stringify({type:'hello',document_id:id(20)}));socket.send(JSON.stringify({type:'ready',latest_sequence:0}));
  });
  const check=async(name,fn)=>{await fn();console.log(`PASS ${name}`);};
  const send=(actor,client=actor,extra={})=>sockets.at(-1).send(JSON.stringify({type:'presence',actor_id:id(actor),client_id:id(client),actor_display_name:`成员${actor}`,actor_username:`member${actor}`,actor_email:`member${actor}@example.com`,cursor:{},...extra}));
  const avatars=()=>page.locator('[data-collaborator-id]');
  const person=actor=>page.locator(`[data-collaborator-id="${id(actor)}"]`);
  await page.goto(baseURL+'/dev/collaboration');
  const root=page.locator('[data-block-editor] .tiptap');await root.waitFor();await page.locator('[data-collaboration-status=online]').waitFor();
  await page.addStyleTag({content:'nextjs-portal{display:none}'});
  await check('status removes redundant version wording and announces online without a cursor',async()=>{
    assert.equal(await page.getByText('基于当前发布版本编辑',{exact:true}).count(),0);assert.equal(await avatars().count(),0);
    assert(outgoing.some(message=>message.type==='presence'&&JSON.stringify(message.cursor)==='{}'));
    await root.click();await page.keyboard.insertText('新改动');await page.getByText('有未发布改动',{exact:true}).waitFor();
  });
  await check('avatars follow online status and hover shows authenticated name and email',async()=>{
    send(31,31,{actor_display_name:'爱丽丝'});await person(31).waitFor();assert.equal(await person(31).innerText(),'爱');
    const status=await page.locator('[data-collaboration-status]').boundingBox(),avatar=await person(31).boundingBox();assert(avatar.x>status.x+status.width&&Math.abs(avatar.y-status.y)<10);
    await person(31).hover();const tooltip=page.getByRole('tooltip');await tooltip.waitFor();assert((await tooltip.innerText()).includes('member31@example.com'));assert((await tooltip.innerText()).includes('爱丽丝'));
    await page.keyboard.press('Escape');await page.mouse.move(0,0);
  });
  await check('click opens selectable profile details and Escape closes them',async()=>{
    await person(31).click();const details=page.getByRole('dialog',{name:'爱丽丝的资料'});await details.waitFor();assert((await details.innerText()).includes('member31@example.com'));await page.keyboard.press('Escape');await details.waitFor({state:'detached'});
  });
  await check('multiple tabs count once per person and own tabs are excluded',async()=>{
    send(31,131,{actor_display_name:'爱丽丝'});send(1,101);await page.waitForTimeout(100);assert.equal(await avatars().count(),1);assert.equal(await page.getByRole('group',{name:'1 位协作者在线'}).count(),1);
  });
  await check('overflow shows four avatars and an expandable remaining-user list',async()=>{
    for(let actor=32;actor<=36;actor++)send(actor);
    const more=page.getByRole('button',{name:'查看其余 2 位协作者'});await more.waitFor();assert.equal(await avatars().count(),4);await more.click();const list=page.getByRole('dialog',{name:'更多在线协作者'});await list.waitFor();assert((await list.innerText()).includes('member35@example.com'));assert((await list.innerText()).includes('member36@example.com'));await page.keyboard.press('Escape');await list.waitFor({state:'detached'});
  });
  await check('keyboard focus exposes details without a mouse',async()=>{
    await person(31).focus();await page.getByRole('tooltip').waitFor();assert((await page.getByRole('tooltip').innerText()).includes('member31@example.com'));await page.keyboard.press('Escape');
    await page.keyboard.press('Enter');await page.getByRole('dialog',{name:'爱丽丝的资料'}).waitFor();await page.keyboard.press('Escape');await page.locator('[role=dialog]').waitFor({state:'detached'});
  });
  await check('mobile touch and dark mode keep profiles and header within the viewport',async()=>{
    await page.setViewportSize({width:320,height:844});await page.evaluate(()=>document.documentElement.classList.add('dark'));
    await person(32).tap();const details=page.getByRole('dialog',{name:'成员32的资料'});await details.waitFor();const bounds=await details.boundingBox();assert(bounds.x>=0&&bounds.x+bounds.width<=320);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    await page.screenshot({path:'/tmp/anby-collaborators-mobile.png',animations:'disabled'});await page.keyboard.press('Escape');await details.waitFor({state:'detached'});await page.setViewportSize({width:1280,height:900});await page.evaluate(()=>document.documentElement.classList.remove('dark'));
  });
  await check('missing email and long names remain usable without inventing identity fields',async()=>{
    send(32,32,{actor_display_name:'一位名字很长的协作者用于检查自动换行'.repeat(4),actor_email:undefined});await person(32).click();const details=page.getByRole('dialog');await details.waitFor();assert(!(await details.innerText()).includes('undefined'));assert(!(await details.innerText()).includes('member32@example.com'));assert((await details.boundingBox()).width<=350);await page.keyboard.press('Escape');await details.waitFor({state:'detached'});
    send(32);await page.mouse.move(0,0);await page.screenshot({path:'/tmp/anby-collaborators-desktop.png',animations:'disabled'});
  });
  await check('expired presence disappears while the editor heartbeat continues',async()=>{
    const heartbeats=outgoing.filter(message=>message.type==='presence').length;await page.clock.fastForward(46_000);await page.locator('[data-collaboration-presence]').waitFor({state:'detached'});assert(outgoing.filter(message=>message.type==='presence').length>heartbeats);assert((await root.innerText()).includes('新改动'));
  });
  await check('disconnect clears stale users and reconnect retains local edits',async()=>{
    send(31);await person(31).waitFor();const count=sockets.length;sockets.at(-1).close({code:1011,reason:'fixture disconnect'});await page.locator('[data-collaboration-presence]').waitFor({state:'detached'});await page.locator('[data-collaboration-status=online]').waitFor();assert(sockets.length>count);assert((await root.innerText()).includes('新改动'));send(33);await person(33).waitFor();assert.equal(await avatars().count(),1);
  });
  await check('revoked editing access removes collaborator details and the editor',async()=>{
    sockets.at(-1).close({code:1008,reason:'fixture permission revoked'});await page.getByText('当前页面仅可阅读',{exact:true}).waitFor();assert.equal(await avatars().count(),0);assert.equal(await root.count(),0);
  });
  assert.deepEqual(errors,[]);console.log('PASS no browser runtime errors');
}finally{await browser.close();}
