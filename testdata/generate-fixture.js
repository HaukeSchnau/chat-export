import { encode } from 'turbo-stream';
import fs from 'fs';

const SHARE_ID = '00000000-0000-4000-8000-000000000001';
const T0 = 1700000000;
const E200 = '\uE200', E201 = '\uE201', E202 = '\uE202';
const marker = (...refs) => E200 + 'cite' + refs.map(r => E202 + r).join('') + E201;

// Messages in linear order. Each entry: [role, content, extra message fields].
const specs = [
  ['system', { content_type: 'text', parts: [''] }, { metadata: { is_visually_hidden_from_conversation: true } }],
  ['user', { content_type: 'text', parts: ['Original custom instructions no longer available'] }, { metadata: { is_visually_hidden_from_conversation: true } }],
  ['user', { content_type: 'multimodal_text', parts: [
      { content_type: 'image_asset_pointer', asset_pointer: 'sediment://file_00000000000000000000000001', size_bytes: 4321, width: 640, height: 480, fovea: null, metadata: { dalle: null, sanitized: true } },
      'Plan a picnic for this park 🌳. What should I bring?',
    ] }, { metadata: { attachments: [{ id: 'file_00000000000000000000000001', name: 'park.jpg', mimeType: 'image/jpeg', width: 640, height: 480, size: 4321 }] } }],
  ['assistant', { content_type: 'thoughts', thoughts: [
      { summary: 'Looking at the park', content: 'The photo shows a lawn with shade trees, so a blanket and cold drinks matter most.', chunks: [], finished: true },
      { summary: 'Looked at the park', content: '', chunks: [], finished: true },
    ] }, { metadata: { reasoning_status: 'is_reasoning' } }],
  ['assistant', { content_type: 'text', parts: ['I will check the local weather and a couple of packing lists first.'] }, { metadata: { is_thinking_preamble_message: true } }],
  ['assistant', { content_type: 'text', parts: [''] }, { recipient: 'web.run', metadata: {} }],
  ['tool', { content_type: 'text', parts: ['The output of this plugin was redacted.'] }, { author: { role: 'tool', name: 'web.run' }, metadata: { is_visually_hidden_from_conversation: true } }],
  ['assistant', { content_type: 'reasoning_recap', content: 'Worked for 4s' }, { metadata: { reasoning_status: 'reasoning_ended', finished_duration_sec: 4 } }],
  ['assistant', { content_type: 'text', parts: [
      '# Picnic plan\n\nBring a blanket, water, and shade.' + marker('turn0search1') + ' Saturday looks dry.' + marker('turn0search2', 'turn0search3') +
      '\n\n## Packing list\n\n- Blanket 🧺\n- Fruit\n\n```md\n# not a heading\n```\n\nOne stale marker follows.' + marker('turn0view9') + E200 + 'memcite' + E201 + ' '
    ] }, { end_turn: true, metadata: (text => ({
      content_references: [
        { matched_text: marker('turn0search1'), start_idx: 49, end_idx: 49 + marker('turn0search1').length, alt: '([weather.example](https://weather.example/forecast?utm_source=chatgpt.com))', type: 'grouped_webpages', items: [ { title: 'Weekend forecast', url: 'https://weather.example/forecast?utm_source=chatgpt.com', attribution: 'weather.example', snippet: '' } ] },
        { matched_text: marker('turn0search2', 'turn0search3'), start_idx: 49 + marker('turn0search1').length + 20, end_idx: 49 + marker('turn0search1').length + 20 + marker('turn0search2', 'turn0search3').length, alt: '', type: 'grouped_webpages', items: [ { title: 'Packing list', url: 'https://picnics.example/list', attribution: 'picnics.example', snippet: '' }, { title: 'Blanket guide', url: 'https://picnics.example/blankets?utm_source=chatgpt.com', attribution: 'picnics.example', snippet: '' } ] },
        { matched_text: E200 + 'memcite' + E201, start_idx: 9999, end_idx: 10008, alt: null, type: 'hidden', invalid: false },
        { matched_text: ' ', start_idx: 10009, end_idx: 10010, alt: '', type: 'sources_footnote', sources: [ { title: 'Weekend forecast', url: 'https://weather.example/forecast?utm_source=chatgpt.com', attribution: 'weather.example' }, { title: 'Packing list', url: 'https://picnics.example/list', attribution: 'picnics.example' } ] },
      ],
    }))() }],
  ['user', { content_type: 'text', parts: ['Draw the picnic and show me the shopping list as code.'] }, {}],
  ['assistant', { content_type: 'multimodal_text', parts: [
      { content_type: 'image_asset_pointer', asset_pointer: 'sediment://file_00000000000000000000000002', size_bytes: 8765, width: 1024, height: 1024, fovea: null, metadata: { dalle: { gen_id: 'gen1', prompt: 'A picnic blanket under a tree', seed: 1, serialization_title: 'DALL-E generation metadata' }, sanitized: true } },
    ] }, { metadata: {} }],
  ['assistant', { content_type: 'code', language: 'json', text: '{"list": ["blanket", "fruit"]}' }, { recipient: 'all', metadata: {} }],
  ['assistant', { content_type: 'text', parts: ['Here is the drawing and the list. Enjoy! 😀'] }, { end_turn: true, metadata: {} }],
];

const ids = specs.map((_, i) => `msg-${String(i + 1).padStart(4, '0')}`);
const rootId = 'root-0000';
const mapping = { [rootId]: { id: rootId, message: null, parent: null, children: [ids[0]] } };
const linear = [{ id: rootId, message: null, parent: null, children: [ids[0]] }];
specs.forEach(([role, content, extra], i) => {
  const id = ids[i];
  const message = {
    id, author: { role, metadata: {} }, create_time: T0 + i * 10, update_time: null, content,
    status: 'finished_successfully', end_turn: null, weight: 1, recipient: 'all', channel: null,
    ...extra,
    metadata: { shared_conversation_id: SHARE_ID, ...(extra.metadata ?? {}) },
  };
  const node = { id, message, parent: i === 0 ? rootId : ids[i - 1], children: i + 1 < ids.length ? [ids[i + 1]] : [] };
  mapping[id] = node; linear.push(node);
});

const data = {
  title: 'Fixture: Picnic Planning 🧺',
  create_time: T0 + 1000, update_time: T0 + 1001, moderation_results: [],
  conversation_id: SHARE_ID, is_archived: false, safe_urls: [], blocked_urls: [],
  default_model_slug: 'fixture-model', mapping, current_node: ids[ids.length - 1],
  is_public: true, linear_conversation: linear, continue_conversation_url: `https://chatgpt.com/share/${SHARE_ID}/continue`,
};
const root = {
  loaderData: {
    root: { statsigGateEvaluationsPromise: Promise.resolve({}) },
    'routes/share.$shareId.($action)': { sharedConversationId: SHARE_ID, serverResponse: { data }, moderationMode: false },
  },
  actionData: null, errors: null,
};

const chunks = [];
const td = new TextDecoder(); for await (const chunk of encode(root)) chunks.push(td.decode(chunk));
const esc = s => JSON.stringify(s).replace(/</g, '\\u003c').replace(/>/g, '\\u003e');
const script = chunks.map(c => `<script nonce="fixture">window.__reactRouterContext.streamController.enqueue(${esc(c)});</script>`).join('');
const html = `<!DOCTYPE html><html lang="en-US"><head><meta charSet="UTF-8"/><title>ChatGPT - ${data.title}</title></head><body><div id="root"></div>` +
  `<script nonce="fixture">window.__reactRouterContext = {"basename":"/","ssr":true};window.__reactRouterContext.stream = new ReadableStream({start(controller){window.__reactRouterContext.streamController = controller;}}).pipeThrough(new TextEncoderStream());</script>` +
  script + `<script nonce="fixture">window.__reactRouterContext.streamController.close();</script></body></html>\n`;
fs.writeFileSync(process.argv[2], html);
console.log('chunks', chunks.length, 'bytes', html.length, 'messages', linear.length);
