'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');

class Element {
  constructor() {
    this.children = [];
    this.dataset = {};
    this.attributes = {};
    this.textContent = '';
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; this.textContent = ''; }
  setAttribute(name, value) { this.attributes[name] = value; }
}

function renderSummary(totals, language = 'zh-CN') {
  const summary = new Element(), target = new Element(), button = new Element();
  const view = {root: {open: false, querySelector: selector => selector === 'summary' ? summary : target},
    find: () => button, ledgerState: {lifetime: totals, last_quota_observations: []}};
  const context = vm.createContext({
    document: {createElement: () => new Element(), createTextNode: text => ({textContent: text})},
    window: {addEventListener() {}}, view, summaryLanguage: language, console,
  });
  for (const file of ['stats.js', 'accounting.js']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../web', file), 'utf8'), context);
  }
  vm.runInContext('language=summaryLanguage; renderAccountSummary(view);', context);
  return target.children;
}

test('account summary pairs recorded Tokens and USD with explicit units', () => {
  const rows = renderSummary({total_tokens: 9450000, usd_reference: 12.3456, usd_unpriced_requests: 0});
  assert.equal(rows.length, 4);
  assert.deepEqual(rows[0].children.map(child => child.textContent), ['实际 Tokens', '9.45 M', 'Tokens']);
  assert.deepEqual(rows[1].children.map(child => child.textContent), ['美元参考值', '12.3456', 'USD']);
  assert.match(rows[1].title, /不是实际扣费/);
});

test('unknown rates are not presented as a complete dollar total', () => {
  const rows = renderSummary({total_tokens: 42, usd_reference: 2, usd_unpriced_requests: 3});
  assert.equal(rows[1].children[1].textContent, '-');
  assert.equal(rows[1].children[2].textContent, 'USD');
  assert.equal(rows[1].dataset.incomplete, 'true');
  assert.match(rows[1].title, /3 次/);
});

test('zero-dollar and English summaries keep the reference label', () => {
  const rows = renderSummary({total_tokens: 0, usd_reference: 0, usd_unpriced_requests: 0}, 'en');
  assert.equal(rows[1].children[0].textContent, 'USD reference');
  assert.equal(rows[1].children[1].textContent, '0');
  assert.equal(rows[1].children[2].textContent, 'USD');
});
