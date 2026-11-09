// Run with Node >= 22.18: node scripts/test-editor-adapter.mjs
import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { test } from 'node:test';
const web = fileURLToPath(new URL('../', import.meta.url));
registerHooks({ resolve(specifier, context, next) {
  if (specifier.startsWith('@/')) return next(pathToFileURL(`${web}${specifier.slice(2)}.ts`).href, context);
  if (specifier.startsWith('.') && context.parentURL?.startsWith(pathToFileURL(web).href) && !/\.[cm]?[jt]sx?$/.test(specifier)) return next(`${specifier}.ts`, context);
  return next(specifier, context);
} });
const { toAst } = await import('../lib/editor/toAst.ts');
const { fromAst } = await import('../lib/editor/fromAst.ts');
const { createAdapterState, newBlockId } = await import('../lib/editor/ids.ts');
const document = (children) => ({ type: 'document', schema_version: 1, children });
const paragraph = (text) => ({ type: 'paragraph', id: newBlockId(), content: [{ type: 'text', text }] });
const pm = (content) => ({ type: 'doc', content: [{ type: 'paragraph', attrs: { id: newBlockId() }, content }] });

test('nested and toggleable headings retain structure and IDs after saving', () => {
  for (const toggleable of [true, false, undefined]) {
    const heading = { type: 'heading', id: newBlockId(), level: 2, content: [{ type: 'text', text: 'Title' }], children: [paragraph('Body')] };
    if (toggleable !== true) delete heading.children;
    if (toggleable !== undefined) heading.is_toggleable = toggleable;
    const ast = document([heading]);
    assert.deepEqual(toAst(fromAst(ast)), ast);
  }
});
test('toggle summaries retain rich text and body on load/save', () => {
  const ast = document([{ type: 'toggle', id: newBlockId(), content: [{ type: 'text', text: 'Summary', marks: ['bold'] }], children: [paragraph('Body')] }]);
  assert.deepEqual(toAst(fromAst(ast)), ast);
});
test('Shift+Enter is saved as a visible newline', () => {
  assert.deepEqual(toAst(pm([{ type: 'text', text: 'a' }, { type: 'hardBreak' }, { type: 'text', text: 'b' }])).children[0].content,
    [{ type: 'text', text: 'a' }, { type: 'text', text: '\n' }, { type: 'text', text: 'b' }]);
});
test('inline code retains additional formatting instead of silently dropping it', () => {
  const ast = toAst(pm([{ type: 'text', text: 'Code', marks: [{ type: 'inlineCode' }, { type: 'bold' }, { type: 'textColor', attrs: { color: 'red' } }] }]));
  assert.deepEqual(ast.children[0].content, [{ type: 'text', text: 'Code', marks: ['code', 'bold'], text_color: 'red' }]);
  assert.deepEqual(toAst(fromAst(ast)), ast);
});
test('unsupported link formatting and unknown blocks cannot be silently lost', () => {
  assert.throws(() => toAst(pm([{ type: 'text', text: 'Link', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }, { type: 'bold' }] }])), /链接/);
  assert.throws(() => toAst({ type: 'doc', content: [{ type: 'unknown' }] }), /未知/);
});
test('failed validation does not change future block IDs', () => {
  const state = createAdapterState();
  assert.throws(() => toAst({ type: 'doc', content: [{ type: 'equation', attrs: { expression: '' } }] }, state));
  assert.deepEqual(state.generatedIds, new Map());
});

const { Schema } = await import('@tiptap/pm/model');
const { EditorState } = await import('@tiptap/pm/state');
const { history, undo } = await import('@tiptap/pm/history');
const { applyRemoteDocument } = await import('../lib/editor/remote-document.ts');
const pmSchema = new Schema({ nodes: {
  doc: { content: 'paragraph*' }, paragraph: { content: 'text*' }, text: {},
} });
const pmDocument = (...values) => pmSchema.node('doc', null, values.map((value) => pmSchema.node('paragraph', null, value ? pmSchema.text(value) : undefined)));

test('local undo preserves a remote edit in an unchanged neighboring paragraph', () => {
  let state = EditorState.create({ schema: pmSchema, doc: pmDocument('a', 'b'), plugins: [history()] });
  state = state.apply(state.tr.insertText('local', 2));
  const transaction = state.tr;
  applyRemoteDocument(transaction, pmDocument('alocal', 'b remote'));
  state = state.apply(transaction);
  assert.equal(undo(state, (tr) => { state = state.apply(tr); }), true);
  assert.deepEqual(state.doc.toJSON(), pmDocument('a', 'b remote').toJSON());
});
test('remote insertion, deletion and empty document replacement produce the exact target', () => {
  for (const [before, after] of [[['abc'], ['abXc']], [['abc'], ['ac']], [['a'], ['a', 'b']], [['a'], []]]) {
    const state = EditorState.create({ schema: pmSchema, doc: pmDocument(...before) });
    const tr = state.tr;
    applyRemoteDocument(tr, pmDocument(...after));
    assert.deepEqual(tr.doc.toJSON(), pmDocument(...after).toJSON());
  }
});

test('malformed component configuration is not silently replaced with an empty object', () => {
  assert.throws(() => toAst({ type: 'doc', content: [{ type: 'componentBlock', attrs: { id: newBlockId(), componentId: newBlockId(), entityId: newBlockId(), componentVersion: 1, displayConfig: '{broken' } }] }), /JSON/);
});
