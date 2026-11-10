// Run against /dev/editor with EDITOR_TEST_URL and an installed PLAYWRIGHT_MODULE.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.EDITOR_TEST_URL || 'http://127.0.0.1:3107';
const id = n => `0198a1b2-c3d4-7e5f-8a9b-${String(n).padStart(12, '0')}`;
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addCookies([{ name: 'anby_session', value: 'editor-fixture', url: baseURL }]);
  const page = await context.newPage();
  const errors = [], requests = [];
  page.on('pageerror', error => errors.push(error.message));
  let failMembers = false, badMedia = false;
  const component = { id: id(10), name: '人物资料卡', component_key: 'people-card', created_by: id(1), created_at: '2026-01-01', updated_at: '2026-01-01' };
  const entity = { id: id(20), display_label: '安比', canonical_key: 'anby', status: 'active', entity_type: { id: id(21), name: '人物', type_key: 'person' }, labels: [{ label: '安比', is_primary: true }], aliases: [] };
  const revision = (n, mime = 'image/png') => ({ id: id(n), asset_id: id(90), mime_type: mime, size_bytes: 2048, created_at: '2026-01-01', content_hash: 'hash' });
  await page.route('**/*', route => {
    const u = new URL(route.request().url()), p = u.pathname;
    if (u.origin !== baseURL) return route.fulfill({ body: '' });
    if (!p.startsWith('/api/')) return route.continue();
    requests.push(u.pathname + u.search);
    const respond = json => route.fulfill({ json });
    if (p.endsWith('/auth/session')) return respond({ actor_id: id(1), wiki_id: id(2), home_wiki_id: id(2), main_namespace_id: id(3), wiki_name: '测试 Wiki', actor_type: 'human', display_name: '编辑者', method: 'session', wikis: [] });
    if (p === '/api/v1/components') return respond(u.searchParams.has('cursor') ? { items: [component], next_cursor: '' } : { items: [{ ...component, id: id(11), name: '其他组件' }], next_cursor: 'second' });
    if (/components\/[^/]+\/versions\/\d+\/preview$/.test(p)) return respond({ html: '<p>资料卡预览</p>' });
    if (/components\/[^/]+\/versions$/.test(p)) return respond({ items: [1, 2].map(version => ({ component_id: id(10), version, props_schema: { type: 'object', properties: { title: { type: 'string', default: '默认标题' } } }, renderer_ref: 'builtin.key_value', status: 'published', created_by: id(1), created_at: '2026-01-01' })) });
    if (/components\/[^/]+$/.test(p)) return respond(component);
    if (p === '/api/v1/entities') return respond({ items: [entity], next_cursor: '' });
    if (/entities\/[^/]+$/.test(p)) return respond(entity);
    if (p === '/api/v1/claims/options') return respond({ items: [{ id: id(31), label: '安比 · 身高', description: '156 cm' }], next_cursor: '' });
    if (p === '/api/v1/citations/options') return respond({ items: [{ id: id(41), label: '人物图鉴', description: '角色简介' }], next_cursor: '' });
    if (p === '/api/v1/wiki/member-options') {
      if (failMembers) return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: '暂不可用' } } });
      return respond({ items: [{ id: id(51), label: '小明', description: 'xiaoming' }], next_cursor: '' });
    }
    if (p === '/api/v1/assets') return respond({ items: [{ id: id(90), name: '安比头像.png', status: 'active', current_revision: revision(61), created_at: '2026-01-01', updated_at: '2026-01-01' }], next_cursor: '' });
    if (/assets\/revisions\/[^/]+\/content$/.test(p)) return route.fulfill({ contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64') });
    if (/assets\/revisions\/[^/]+$/.test(p)) return respond(revision(61, badMedia ? 'audio/mpeg' : 'image/png'));
    if (p === '/api/v1/pages' || p === '/api/v1/pages/search') return respond({ items: [{ id: id(71), display_title: '人物介绍', namespace: 'main', language: 'zh', matched_on: 'title', highlight: '', score: 1 }], next_cursor: '' });
    if (/pages\/[^/]+\/outline$/.test(p)) return respond({ items: [{ heading_block_id: id(81), title: '人物经历', level: 2, parent_heading_block_id: '', slug: 'history', position_key: '1' }] });
    if (/pages\/[^/]+$/.test(p)) return respond({ page: { id: id(71), display_title: '人物介绍' }, content: {}, via_alias: false, can_edit: true });
    return respond({ items: [] });
  });
  const editor = page.locator('[data-block-editor]').first();
  const root = editor.locator('.tiptap').first();
  const ast = async () => JSON.parse(await page.locator('[data-editor-ast]').innerText());
  const wait = async fn => { let err; for (let n=0;n<80;n++) { try { await fn(); return; } catch(e) {err=e;} await page.waitForTimeout(50); } throw err; };
  const check = async (name, fn) => { await fn(); console.log(`PASS ${name}`); };
  const set = async content => { await page.getByRole('button',{name:'空白文档'}).click(); await root.waitFor(); await root.evaluate((el, content) => el.editor.commands.setContent({ type:'doc', content }), content); };
  const insert = async (query, title) => { await page.getByRole('button',{name:'空白文档'}).click(); await root.waitFor(); await root.click(); await page.keyboard.insertText('/'+query); await page.getByRole('button',{name:new RegExp(title)}).click(); };
  const choose = async (label, text, query = text) => {
    await page.getByRole('button',{name:`选择${label}`,exact:true}).last().click();
    const dialog = page.getByRole('dialog').last();
    await dialog.getByRole('combobox',{name:`搜索${label}`}).fill(query);
    await dialog.getByRole('option').filter({hasText:text}).click();
  };
  await page.goto(baseURL+'/dev/editor'); await root.waitFor();
  await check('component insertion searches all catalog pages and resolves entity names', async()=>{
    await insert('component','组件');
    await choose('组件','人物资料卡');
    await choose('实体','安比');
    await page.getByRole('button',{name:'运行预览'}).click();
    await page.getByRole('button',{name:'插入组件',exact:true}).click();
    await wait(async()=>assert.equal((await ast()).children[0].component_id,id(10)));
    assert.equal((await ast()).children[0].component_version,2);
    assert.equal((await ast()).children[0].entity_id,id(20));
    assert(requests.some(request=>request.includes('components?')&&request.includes('cursor=second')));
  });
  await check('editing preserves pinned version and config; cancellation and atomic undo retain original',async()=>{
    await set([{type:'componentBlock',attrs:{componentId:id(10),componentVersion:1,entityId:id(22),displayConfig:'{"title":"原来的标题"}'}}]);
    const before=await ast();
    await editor.locator('[data-editor-block-type="componentBlock"] summary').click();
    await page.getByRole('button',{name:'选择组件与实体'}).click();
    assert.equal(await page.locator('#insert-component-version').inputValue(),'1');
    assert.equal(await page.getByLabel('title',{exact:true}).inputValue(),'原来的标题');
    await page.getByRole('button',{name:'关闭',exact:true}).first().click();
    assert.deepEqual(await ast(),before);
    await page.getByRole('button',{name:'选择组件与实体'}).click();
    await choose('实体','安比');
    await page.getByRole('button',{name:'运行预览'}).click();
    await page.getByRole('button',{name:'保存组件',exact:true}).click();
    await wait(async()=>assert.equal((await ast()).children[0].entity_id,id(20)));
    assert.equal((await ast()).children[0].display_config.title,'原来的标题');
    await root.evaluate(el=>el.editor.commands.undo());
    assert.deepEqual(await ast(),before);
  });
  await check('media insertion uses names, auto fills text and retains MIME validation during edits', async()=>{
    await insert('media','媒体');
    await choose('文件','安比头像.png');
    await page.getByRole('button',{name:'插入',exact:true}).click();
    await wait(async()=>assert.equal((await ast()).children[0].asset_revision_id,id(61)));
    assert.equal((await ast()).children[0].alt_text,'安比头像.png');
    await set([{type:'imageAsset',attrs:{assetRevisionId:id(62),altText:'原图'}}]);
    await editor.locator('[data-editor-block-type="imageAsset"] summary').click();
    badMedia=true;
    await choose('文件','安比头像.png');
    await page.getByText('文件类型不匹配，请重新选择。',{exact:true}).waitFor();
    assert.equal((await ast()).children[0].asset_revision_id,id(62));
    badMedia=false;
    await page.getByRole('dialog').last().getByRole('option').filter({hasText:'安比头像.png'}).click();
    await wait(async()=>assert.equal((await ast()).children[0].asset_revision_id,id(61)));
  });
  await check('person insertion supports failed search retry without raw account IDs',async()=>{
    await insert('mention','提及');
    failMembers=true;
    await page.getByRole('button',{name:'选择人员',exact:true}).click();
    const dialog=page.getByRole('dialog').last();
    await dialog.getByRole('combobox').fill('小明');
    await dialog.getByRole('button',{name:'重试'}).waitFor();
    failMembers=false;
    await dialog.getByRole('button',{name:'重试'}).click();
    await dialog.getByRole('option').filter({hasText:'小明'}).click();
    await page.getByRole('button',{name:'插入',exact:true}).click();
    await wait(async()=>assert.equal((await ast()).children[0].content[0].actor_id,id(51)));
    assert.equal((await ast()).children[0].content[0].display_text,'小明');
  });
  await check('existing page, entity, claim and citation references search by names',async()=>{
    for(const [type,attr,label,target,name] of [['pageReference','targetPageId','页面',71,'人物介绍'],['entityReference','entityId','实体',20,'安比'],['claimReference','claimId','事实',31,'安比 · 身高'],['citationReference','citationId','引文',41,'人物图鉴']]) {
      await set([{type:'paragraph',content:[{type,attrs:{[attr]:id(99),displayText:'原引用'}}]}]);
      await editor.locator('.ast-inline-editor').focus();
      await choose(label,name);
      await wait(async()=>assert.equal((await ast()).children[0].content[0][{targetPageId:'target_page_id',entityId:'entity_id',claimId:'claim_id',citationId:'citation_id'}[attr]],id(target)));
    }
  });
  await check('chapter changes save page and heading atomically',async()=>{
    await set([{type:'paragraph',content:[{type:'pageAnchorReference',attrs:{targetPageId:id(72),targetHeadingBlockId:id(82),displayText:'原章节'}}]}]);
    await editor.locator('.ast-inline-editor').focus();
    await page.getByRole('button',{name:'选择页面与章节'}).click();
    await choose('页面','人物介绍');
    assert.equal(await page.getByRole('button',{name:'保存章节链接'}).isDisabled(),true);
    await choose('章节','人物经历');
    await page.getByRole('button',{name:'保存章节链接'}).click();
    await wait(async()=>assert.equal((await ast()).children[0].content[0].target_heading_block_id,id(81)));
    assert.equal((await ast()).children[0].content[0].target_page_id,id(71));
  });
  await check('breadcrumb and page button references use selectors and preserve nested content',async()=>{
    await set([{type:'breadcrumb',attrs:{itemsJson:'[{"display_text":"当前页面"}]'}}]);
    await editor.locator('[data-editor-block-type="breadcrumb"] summary').click();
    await page.getByRole('button',{name:'编辑页面路径'}).click();
    await choose('页面','人物介绍');
    await page.getByRole('button',{name:'保存页面路径'}).click();
    assert.equal((await ast()).children[0].items[0].target_page_id,id(71));
    await set([{type:'pageButton',attrs:{label:'添加段落',actionsJson:JSON.stringify([{type:'insert_blocks',target_page_id:id(72),blocks:[{id:id(85),type:'paragraph',content:[{type:'text',text:'待插入的文字'}]}]}])}}]);
    await editor.locator('[data-editor-block-type="pageButton"] summary').click();
    await page.getByRole('button',{name:'编辑按钮动作'}).click();
    await choose('页面','人物介绍');
    await page.getByRole('button',{name:'保存按钮动作'}).click();
    assert.equal((await ast()).children[0].actions[0].target_page_id,id(71));
    assert.equal((await ast()).children[0].actions[0].blocks[0].content[0].text,'待插入的文字');
  });
  await check('mobile picker supports keyboard selection and does not overflow',async()=>{
    await page.setViewportSize({width:390,height:844});
    await insert('component','组件');
    await page.getByRole('button',{name:'选择组件',exact:true}).click();
    await page.getByRole('dialog').last().getByRole('combobox').fill('人物资料卡');
    await page.getByRole('option').filter({hasText:'人物资料卡'}).waitFor();
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));
    await page.keyboard.press('ArrowDown');await page.keyboard.press('Enter');
    await wait(async()=>assert.equal(await page.getByRole('dialog').count(),1));
    await page.keyboard.press('Escape');
  });
  await check('all fixture reference fields avoid manual UUID inputs',async()=>{
    await page.reload(); await root.waitFor();
    const ids = await editor.locator('input,textarea').evaluateAll(elements=>elements.map(el=>el.value).filter(value=>/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)));
    assert.deepEqual(ids,[]);
  });
  assert.deepEqual(errors,[]);
  console.log('PASS no browser runtime errors');
} finally {await browser.close();}
