const $ = selector => document.querySelector(selector);
let language = 'zh-CN';
let loadSequence = 0;
let overviewReady = false;
let key = '';
const accountViews = new Map();
let nextViewID = 0;
let bridgeState = 'unknown';
const bridgePending = new Map();
const base = '/v0/management/cpa-quota-estimator';
const copy = {
  'zh-CN': {title:'Codex 额度统计',usage:'用量统计',models:'实际模型用量',allAccounts:'所有账号总使用',totalUnavailable:'部分账号读取失败，暂无法汇总总使用量',
    account:'账号',pool:'额度池',used:'上游已用 %',at:'观测时间',reset:'上游重置时间',state:'观测状态',old:'已过重置时间，等待新观测',recorded:'已记录',
    model:'模型',tier:'服务档位',requests:'请求数',failed:'失败数',input:'输入 Tokens',cache:'缓存读取 Tokens',output:'输出 Tokens',
    tokens:'实际 Tokens',usd:'美元参考值',credits:'Credits 参考值',none:'暂无记录',refresh:'刷新',logout:'退出认证',
    login:'CPA 管理认证',connect:'连接',auth:'请输入有效的 CPA Management Key',loading:'读取中',http:'请通过 HTTPS 管理入口连接',
    expand:'展开账号额度',collapse:'收起账号额度',noAccounts:'暂无账号',main:'主额度',weekly:'周额度',range:'统计范围',unavailable:'尚无观测',
    days:['近 24 小时','近 7 天','近 30 天']},
  en: {title:'Codex Quota Statistics',usage:'Usage',models:'Actual model usage',allAccounts:'Total usage across all accounts',totalUnavailable:'Some accounts failed to load; total usage is unavailable',
    account:'Account',pool:'Quota pool',used:'Upstream used %',at:'Observed at',reset:'Upstream reset time',state:'Observation status',old:'Reset time passed; awaiting observation',recorded:'Recorded',
    model:'Model',tier:'Service tier',requests:'Requests',failed:'Failed',input:'Input Tokens',cache:'Cache-read Tokens',output:'Output Tokens',
    tokens:'Actual Tokens',usd:'USD reference',credits:'Credits reference',none:'No records',refresh:'Refresh',logout:'Sign out',
    login:'CPA management authentication',connect:'Connect',auth:'Enter a valid CPA Management Key',loading:'Loading',http:'Connect through your HTTPS management entry',
    expand:'Expand account usage',collapse:'Collapse account usage',noAccounts:'No accounts',main:'Primary',weekly:'Weekly',range:'Date range',unavailable:'No observation',
    days:['Last 24 hours','Last 7 days','Last 30 days']}
};
const tr = name => copy[language][name];

/*
 * Lucide 0.468.0, ISC License.
 * Copyright (c) for portions of Lucide are held by Cole Bemis 2013-2022 as part
 * of Feather (MIT). All other copyright (c) for Lucide are held by Lucide
 * Contributors 2022.
 * Permission to use, copy, modify, and/or distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */
const lucideIcons = {
  'chart-no-axes-combined':'<path d="M12 16v5"/><path d="M16 14v7"/><path d="M20 10v11"/><path d="m22 3-8.646 8.646a.5.5 0 0 1-.708 0L9.354 8.354a.5.5 0 0 0-.707 0L2 15"/><path d="M4 18v3"/><path d="M8 14v7"/>',
  'user-round':'<circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/>',
  'refresh-cw':'<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
  'log-out':'<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" x2="9" y1="12" y2="12"/>',
  layers:'<path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z"/><path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12"/><path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17"/>',
  activity:'<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2"/>',
  coins:'<circle cx="8" cy="8" r="6"/><path d="M18.09 10.37A6 6 0 1 1 10.34 18"/><path d="M7 6h1v4"/><path d="m16.71 13.88 .7 .71-2.82 2.82"/>',
  'credit-card':'<rect width="20" height="14" x="2" y="5" rx="2"/><line x1="2" x2="22" y1="10" y2="10"/>',
  'chevron-down':'<path d="m6 9 6 6 6-6"/>',
  'chevron-left':'<path d="m15 18-6-6 6-6"/>',
  'chevron-right':'<path d="m9 18 6-6-6-6"/>',
  plus:'<path d="M5 12h14"/><path d="M12 5v14"/>',
  check:'<path d="M20 6 9 17l-5-5"/>',
  x:'<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  pencil:'<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"/><path d="m15 5 4 4"/>',
  'trash-2':'<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/>',
  'calendar-days':'<path d="M8 2v4"/><path d="M16 2v4"/><rect width="18" height="18" x="3" y="4" rx="2"/><path d="M3 10h18"/><path d="M8 14h.01"/><path d="M12 14h.01"/><path d="M16 14h.01"/><path d="M8 18h.01"/><path d="M12 18h.01"/><path d="M16 18h.01"/>',
  'clock-3':'<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16.5 12"/>'
};
const uiIcon = name => '<svg class="ui-icon" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">' + (lucideIcons[name] || '') + '</svg>';
function paintIcons(root) {
  for (const target of root.querySelectorAll('[data-icon]')) target.innerHTML=uiIcon(target.dataset.icon);
}
function labeledIcon(target, label, icon, iconOnly = false) {
  target.innerHTML=uiIcon(icon);
  if (iconOnly) {
    target.title=label;
    target.setAttribute('aria-label',label);
  } else target.append(document.createTextNode(label));
}

function rememberedKey() {
  try {
    let raw = localStorage.getItem('cli-proxy-auth');
    if (raw && raw.startsWith('enc::v1::')) {
      const bytes = Uint8Array.from(atob(raw.slice(9)), c => c.charCodeAt(0));
      const mask = new TextEncoder().encode('cli-proxy-api-webui::secure-storage|' + location.host + '|' + navigator.userAgent);
      raw = new TextDecoder().decode(bytes.map((b,i) => b ^ mask[i % mask.length]));
    }
    const saved = JSON.parse(raw || 'null'), auth = saved && (saved.state || saved);
    if (auth && auth.managementKey && auth.rememberPassword !== false &&
      (!auth.apiBase || new URL(auth.apiBase,location.origin).origin === location.origin)) return String(auth.managementKey).trim();
    return sessionStorage.getItem('cqe-stats-key') || sessionStorage.getItem('cqe-management-key') || '';
  } catch (_) { return ''; }
}

window.addEventListener('message', event => {
  const data = event.data;
  if (event.origin !== location.origin || event.source !== window.parent || !data ||
    data.type !== 'cpamp:plugin-api-response' || !bridgePending.has(data.id)) return;
  const pending = bridgePending.get(data.id);
  clearTimeout(pending.timer);
  bridgePending.delete(data.id);
  if (data.status >= 200 && data.status < 300) pending.resolve(data.data);
  else pending.reject(Object.assign(new Error(typeof data.data === 'string' ? data.data : tr('auth')),
    {auth:data.status === 401 || data.status === 403}));
});

async function api(path, options = {}) {
  if (window.parent !== window && bridgeState !== 'unavailable') {
    try {
      const data = await new Promise((resolve,reject) => {
        const id = crypto.randomUUID();
        const timer = setTimeout(() => {
          bridgePending.delete(id);
          reject(Object.assign(new Error('Management bridge unavailable'),{bridgeUnavailable:true}));
        },bridgeState === 'unknown' ? 1500 : 15000);
        bridgePending.set(id,{resolve,reject,timer});
        window.parent.postMessage({type:'cpamp:plugin-api-request',id,path:base+path,method:options.method || 'GET',body:options.body},location.origin);
      });
      bridgeState = 'available';
      return data;
    } catch (error) {
      if (!error.bridgeUnavailable) throw error;
      bridgeState = 'unavailable';
    }
  }
  if (key && location.protocol !== 'https:' && !['127.0.0.1','localhost','[::1]'].includes(location.hostname)) {
    throw new Error(tr('http'));
  }
  const response = await fetch(base+path,{...options,headers:{'Content-Type':'application/json',...(key ? {Authorization:'Bearer '+key} : {})}});
  if (response.status === 401 || response.status === 403) throw Object.assign(new Error(tr('auth')),{auth:true});
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}

function heads(target,names) {
  const row = document.createElement('tr');
  for (const name of names) {
    const th=document.createElement('th'); th.textContent=tr(name);
    if (['requests','failed','input','cache','output','tokens','usd','credits'].includes(name)) th.className='metric-cell';
    row.append(th);
  }
  target.replaceChildren(row);
}
function tableRows(target,rows,columns) {
  target.replaceChildren();
  for (const values of rows) { const row=document.createElement('tr'); ledgerCells(row,values); target.append(row); }
  if (!rows.length) { const row=document.createElement('tr'),cell=document.createElement('td'); cell.colSpan=columns; cell.textContent=tr('none'); row.append(cell); target.append(row); }
}
function renderStats() {
  document.documentElement.lang=language;
  document.title=tr('title');
  for (const [selector,name] of [['#title','title'],['#loginTitle','login'],['#connect','connect']]) $(selector).textContent=tr(name);
  for (const id of ['refresh','logout']) { $('#'+id).title=tr(id); $('#'+id).setAttribute('aria-label',tr(id)); }
  $('#accountsEmpty').textContent=tr('noAccounts');
  $('#accountsTitle').textContent=tr('account');
  $('#accountsCount').textContent=ledgerNumber(accountViews.size);
  renderAllAccountsUsage();
  for (const view of accountViews.values()) {
    renderAccounting(view);
    renderAccountStats(view);
  }
}

function renderAllAccountsUsage() {
  labeledIcon($('#allAccountsTitle'),tr('allAccounts'),'layers');
  const views=[...accountViews.values()];
  const failed=overviewReady && views.some(view => view.accountError);
  const ready=overviewReady && !failed && views.every(view => view.ledgerState && !view.ledgerLoading);
  const totals={};
  if (ready) {
    for (const view of views) {
      for (const name of ['total_tokens','requests','usd_reference','credits_reference','usd_unpriced_requests','credits_unpriced_requests']) {
        totals[name]=(totals[name] || 0)+Number(view.ledgerState.lifetime[name] || 0);
      }
    }
  }
  renderLedgerMetrics($('#allAccountsMetrics'),totals,ready);
  $('#allAccountsUsage').setAttribute('aria-busy',String(!ready && !failed));
  $('#allAccountsStatus').textContent=failed ? tr('totalUnavailable') : '';
  $('#allAccountsStatus').hidden=!failed;
}

function renderAccountSummary(view) {
  const summary=view.root.querySelector('summary');
  summary.title=tr(view.root.open ? 'collapse' : 'expand');
  const target=view.root.querySelector('.account-overview');
  target.replaceChildren();
  if (!view.ledgerState) {
    target.textContent=view.accountError || tr('loading');
    return;
  }
  const tokens=document.createElement('span');
  tokens.className='account-total';
  const amount=ledgerAmount(view.ledgerState.lifetime.total_tokens,'tokens');
  tokens.title=amount.title;
  tokens.append(document.createTextNode(amount.value));
  const unit=document.createElement('span');
  unit.className='accounting-unit';
  unit.textContent=amount.unit;
  tokens.append(unit);
  target.append(tokens);
  const observations=view.ledgerState.last_quota_observations || [];
  for (const scope of ['main','weekly']) {
    const reading=observations.find(q => q.scope===scope);
    const span=document.createElement('span');
    span.className='account-quota';
    span.dataset.scope=scope;
    const stale=reading && reading.reset_at <= Date.now()/1000;
    span.dataset.stale=String(Boolean(stale));
    const label=document.createElement('span'),value=document.createElement('span'),meter=document.createElement('meter');
    label.className='quota-label'; label.textContent=tr(scope);
    value.className='quota-value';
    value.textContent=reading ? ledgerAmount(reading.used_percent,'percent').text : tr('unavailable');
    meter.min=0; meter.max=100;
    const percent=Number(reading && reading.used_percent);
    meter.value=Number.isFinite(percent) ? Math.max(0,Math.min(100,percent)) : 0;
    meter.setAttribute('aria-label',tr(scope));
    span.title=reading ? (stale ? tr('old')+'; ' : '')+tr('at')+': '+ledgerDate(reading.observed_at)+'; '+tr('reset')+': '+ledgerDate(reading.reset_at) : tr('unavailable');
    if (!reading) meter.setAttribute('aria-hidden','true');
    span.append(label,value,meter);
    target.append(span);
  }
}

function renderAccountStats(view) {
  const find=view.find;
  find('usageTitle').textContent=tr('usage');
  labeledIcon(find('modelsTitle'),tr('models'),'activity');
  find('days').setAttribute('aria-label',tr('range'));
  [...find('days').options].forEach((option,i) => option.textContent=copy[language].days[i]);
  find('accountError').textContent=view.accountError;
  find('modelError').textContent=view.modelError;
  heads(find('modelHeads'),['model','tier','requests','failed','input','cache','output','tokens','usd','credits']);
  tableRows(find('modelRows'),view.modelRows.map(r => {
    const t=r.totals;
    return [r.model,r.service_tier || 'standard',ledgerAmount(t.requests, 'requests'),ledgerAmount(t.failed, 'requests'),
      ledgerAmount(t.input_tokens, 'tokens'),ledgerAmount(t.cache_read_tokens, 'tokens'),ledgerAmount(t.output_tokens, 'tokens'),
      ledgerAmount(t.total_tokens, 'tokens'),t.usd_unpriced_requests ? '-' : ledgerAmount(t.usd_reference, 'usd'),t.credits_unpriced_requests ? '-' : ledgerAmount(t.credits_reference, 'credits')];
  }),10);
  renderAccountSummary(view);
}

function viewIsCurrent(view) {
  return accountViews.get(view.account)===view;
}

function createAccountView(account) {
  const root=$('#accountTemplate').content.firstElementChild.cloneNode(true);
  paintIcons(root);
  root.dataset.account=account;
  root.querySelector('.account-name').textContent=account;
  const view={root,account,find:name => root.querySelector('[data-role="'+name+'"]'),
    ledgerState:null,ledgerLoading:true,ledgerOffset:0,ledgerEditID:0,ledgerGeneration:0,modelGeneration:0,
    modelRows:[],accountError:'',modelError:'',paymentBusy:false};
  view.select=selector => root.querySelector(selector.replace(/#([\w-]+)/g,'[data-role="$1"]'));
  const id='account-'+(++nextViewID);
  view.find('usageTitle').id=id+'-usageTitle';
  view.find('wbPanelUsage').setAttribute('aria-labelledby',view.find('usageTitle').id);
  root.open=false;
  try {
    const saved=JSON.parse(localStorage.getItem('cqe-account-visibility') || '{}');
    if (typeof saved[account]==='boolean') root.open=saved[account];
  } catch (_) {}
  root.addEventListener('toggle',() => {
    if (!viewIsCurrent(view)) return;
    renderAccountSummary(view);
    try {
      const saved=JSON.parse(localStorage.getItem('cqe-account-visibility') || '{}');
      const merged={...saved,[account]:root.open};
      localStorage.setItem('cqe-account-visibility',JSON.stringify(merged));
    } catch (_) {}
  });
  accountViews.set(account,view);
  setupAccounting(view);
  view.find('days').onchange=() => loadModels(view);
  renderAccountStats(view);
  return view;
}

function handleAuth(error) {
  if (error.auth && !$('#login').open) $('#login').showModal();
}

async function loadModels(view) {
  const generation=++view.modelGeneration;
  view.find('days').disabled=true;
  view.modelError='';
  try {
    const rows=await api('/usage?'+new URLSearchParams({account:view.account,days:view.find('days').value}));
    if (!viewIsCurrent(view) || generation!==view.modelGeneration) return;
    view.modelRows=rows;
  } catch (error) {
    if (!viewIsCurrent(view) || generation!==view.modelGeneration) return;
    view.modelRows=[];
    view.modelError=error.message;
    handleAuth(error);
  } finally {
    if (viewIsCurrent(view) && generation===view.modelGeneration) {
      view.find('days').disabled=false;
      renderAccountStats(view);
    }
  }
}

async function load() {
  const sequence=++loadSequence;
  overviewReady=false;
  renderAllAccountsUsage();
  $('#refresh').disabled=true;
  $('#status').textContent=tr('loading');
  try {
    const overview=await api('/overview');
    if (sequence !== loadSequence) return;
    const accounts=[...new Set(overview.accounts)];
    for (const [account,view] of accountViews) {
      if (!accounts.includes(account)) { accountViews.delete(account); view.root.remove(); }
    }
    for (const account of accounts) {
      const view=accountViews.get(account) || createAccountView(account);
      view.ledgerLoading=true;
      view.accountError='';
      $('#accounts').append(view.root);
    }
    overviewReady=true;
    $('#accountsCount').textContent=ledgerNumber(accounts.length);
    renderAllAccountsUsage();
    $('#accountsEmpty').hidden=accounts.length>0;
    $('#version').textContent='v'+overview.plugin_version;
    // Keep all accounts visible while bounding concurrent per-account API calls.
    const queue=[...accountViews.values()];
    await Promise.all(Array.from({length:Math.min(4,queue.length)},async () => {
      while (queue.length && sequence===loadSequence) {
        const view=queue.shift();
        await loadAccounting(view);
        if (sequence!==loadSequence || !viewIsCurrent(view)) return;
        await loadModels(view);
      }
    }));
    if (sequence !== loadSequence) return;
    renderStats();
    $('#status').textContent='';
    if (![...accountViews.values()].some(view => view.accountError || view.modelError) && $('#login').open) $('#login').close();
  } catch (error) {
    if (sequence !== loadSequence) return;
    $('#status').textContent=error.message;
    handleAuth(error);
  } finally { if (sequence === loadSequence) $('#refresh').disabled=false; }
}

function initialize() {
  key=rememberedKey();
  paintIcons(document);
  renderStats();
  $('#refresh').onclick=load;
  $('#language').onchange=() => { language=$('#language').value; renderStats(); };
  $('#logout').onclick=() => {
    ++loadSequence; key='';
    overviewReady=false;
    accountViews.clear(); $('#accounts').replaceChildren();
    $('#refresh').disabled=false;
    try { sessionStorage.removeItem('cqe-stats-key'); sessionStorage.removeItem('cqe-management-key'); } catch (_) {}
    $('#managementKey').value=''; renderStats();
    if (!$('#login').open) $('#login').showModal();
  };
  $('#loginForm').onsubmit=async event => {
    event.preventDefault(); key=$('#managementKey').value.trim(); $('#managementKey').value='';
    try { sessionStorage.setItem('cqe-stats-key',key); } catch (_) {}
    await load();
  };
  load();
}
window.addEventListener('DOMContentLoaded',initialize);
