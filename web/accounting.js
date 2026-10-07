const ledgerCopy = {
  'zh-CN': {
    lifetime: '累计使用', subscriptions: '月账本', weeklyLedger: '周账本', fiveHourLedger: '五小时账本',
    tokens: '实际 Tokens', requests: '请求数', usd: '美元参考值', credits: 'Credits 参考值',
    dropped: '已知入库失败请求：', missing: '无法定价请求（美元 / Credits）：',
    paid: '支付时间（UTC+8）', amount: '支付金额（USD）', next: '下一次支付', ongoing: '进行中',
    status: '状态', finished: '已结束', notStarted: '未开始', awaitingReset: '待确认重置',
    add: '添加支付记录', save: '保存修改', cancel: '取消', edit: '编辑', remove: '删除',
    confirm: '删除这条实际支付记录？相邻订阅区间将重新划分。',
    noPayments: '尚未录入实际支付日期', noResets: '尚无已确认的重置周期',
    period: '周期', scope: '额度池', peak: '已用峰值 %', quota: '额度使用增长', start: '开始时间', end: '结束 / 重置时间',
    previous: '上一页', following: '下一页', page: '页', loading: '正在读取账本', none: '尚无记录',
    incomplete: '部分记录',
    monthlyBasis: '按实际支付时间划分订阅区间，不按自然月推算。',
    shared: '主额度与周额度可能对应同一批请求，不能跨表重复相加。',
    reference: '按记录时价格折算的参考值，不是实际扣费或套餐额度。'
  },
  en: {
    lifetime: 'Cumulative usage', subscriptions: 'Monthly ledger', weeklyLedger: 'Weekly ledger', fiveHourLedger: 'Five-hour ledger',
    tokens: 'Actual Tokens', requests: 'Requests', usd: 'USD reference', credits: 'Credits reference',
    dropped: 'Known failed-to-store requests: ', missing: 'Unpriced requests (USD / Credits): ',
    paid: 'Payment time (UTC+8)', amount: 'Payment amount (USD)', next: 'Next payment', ongoing: 'In progress',
    status: 'Status', finished: 'Ended', notStarted: 'Not started', awaitingReset: 'Awaiting reset observation',
    add: 'Add payment', save: 'Save changes', cancel: 'Cancel', edit: 'Edit', remove: 'Delete',
    confirm: 'Delete this payment? Adjacent subscription intervals will be recomputed.',
    noPayments: 'No actual payment dates recorded', noResets: 'No confirmed reset cycles',
    period: 'Period', scope: 'Quota pool', peak: 'Peak used %', quota: 'Quota usage growth', start: 'Start time', end: 'End / reset time',
    previous: 'Previous', following: 'Next', page: 'Page', loading: 'Loading accounting', none: 'No records',
    incomplete: 'Partial records',
    monthlyBasis: 'Subscription intervals use actual payment dates, not inferred calendar months.',
    shared: 'Primary and weekly pools may refer to the same requests. Do not sum usage across tables.',
    reference: 'Reference values use recorded rates, not actual charges or plan allowances.'
  }
};
const lc = key => (ledgerCopy[language] || ledgerCopy['zh-CN'])[key];
const ledgerNumber = value => Number(value || 0).toLocaleString(language === 'en' ? 'en-US' : 'zh-CN', {maximumFractionDigits: 4});
function ledgerAmount(value, kind) {
  const unit = kind === 'tokens' ? 'Tokens' : kind === 'requests' ? (language === 'en' ? 'requests' : '次') :
    kind === 'usd' ? 'USD' : kind === 'percent' ? '%' : 'Credits';
  const numeric = Number(value ?? 0);
  if (!Number.isFinite(numeric)) return {text: '- ' + unit, value: '-', unit, title: '- ' + unit};
  const locale = language === 'en' ? 'en-US' : 'zh-CN';
  const exact = numeric.toLocaleString(locale, {maximumFractionDigits: 20}) + ' ' + unit;
  let scale = 1, suffix = '';
  if (kind === 'tokens' || kind === 'requests') {
    // Select after rounding so the display never ends up at 1,000 K or 1,000 M.
    for (const [threshold, label] of [[1e3,'K'],[1e6,'M'],[1e9,'B']]) {
      if (Math.abs(numeric) >= threshold * 0.999995) { scale = threshold; suffix = ' ' + label; }
    }
  }
  const number = (numeric / scale).toLocaleString(locale, {maximumFractionDigits: scale > 1 ? 2 : 4}) + suffix;
  return {text: number + ' ' + unit, value: number, unit, title: exact};
}
function ledgerValue(target, amount) {
  target.textContent = amount.text;
  target.title = amount.title;
}
const ledgerDate = value => value ? new Intl.DateTimeFormat(language === 'en' ? 'en-GB' : 'zh-CN',
  {timeZone: 'Asia/Shanghai', dateStyle: 'short', timeStyle: 'short', hour12: false}).format(new Date(value * 1000)) : lc('none');
const ledgerQuotaText = items => (items || []).map(q => q.scope + ': ' + ledgerNumber(q.percent) + '%' +
  (q.partial ? ' (' + lc('incomplete') + ')' : '')).join(' / ') || lc('none');

function ledgerPeriodStatus(period, subscription, now) {
  const start=subscription ? period.paid_at : period.started_at;
  if (start > now) return 'notStarted';
  if (subscription) return !period.next_paid_at || period.next_paid_at > now ? 'ongoing' : 'finished';
  if (period.ended_at) return 'finished';
  if (period.current !== false && period.reset_at > now) return 'ongoing';
  return 'awaitingReset';
}

function ledgerStatusCell(row, index, status) {
  const badge=document.createElement('span');
  badge.className='period-status status-'+status;
  badge.textContent=lc(status);
  row.children[index].replaceChildren(badge);
  if (status==='ongoing') row.classList.add('period-current');
}

function setupAccounting(view) {
  const $=view.select;
  const roles=html => html.replace(/id="([\w-]+)"/g,'data-role="$1"');
  $('#wbPanelUsage').insertAdjacentHTML('afterbegin',
    roles('<section class="accounting-section" id="lifetimeLedger"><h2></h2><dl class="accounting-metrics" id="ledgerMetrics"></dl><p class="accounting-error" role="status" id="ledgerUnpriced" hidden></p><p class="accounting-error" role="status" id="ledgerDropped" hidden></p></section>'));
  $('#ledgerTables').insertAdjacentHTML('beforeend',
    roles('<section class="accounting-section" id="subscriptionLedger"><h2></h2>' +
    '<form class="accounting-form" id="paymentForm"><label><span id="paymentDateLabel"></span><input id="paymentDate" type="datetime-local" required step="1"></label><label><span id="paymentAmountLabel"></span><input id="paymentAmount" type="number" min="0" max="1000000" step="0.01" value="0" required></label><button id="paymentSubmit" class="primary-button" type="submit"></button><button id="paymentCancel" type="button" hidden></button></form><p id="paymentStatus" role="status"></p><div class="accounting-scroll"><table class="accounting-table"><thead id="subscriptionHeads"></thead><tbody id="subscriptionRows"></tbody></table></div></section>' +
    '<div id="resetLedger"><section class="accounting-section" id="weeklyLedger"><h2></h2><div class="accounting-scroll"><table class="accounting-table"><thead id="weeklyHeads"></thead><tbody id="weeklyRows"></tbody></table></div></section>' +
    '<section class="accounting-section" id="fiveHourLedger"><h2></h2><div class="accounting-scroll"><table class="accounting-table"><thead id="fiveHourHeads"></thead><tbody id="fiveHourRows"></tbody></table></div></section><div class="accounting-pages"><button class="icon-button" type="button" id="ledgerPrevious"></button><span id="ledgerPage"></span><button class="icon-button" type="button" id="ledgerNext"></button></div></div>'));
  $('#paymentForm').onsubmit = async event => {
    event.preventDefault();
    const account = view.account;
    const paidAt = Math.floor(new Date($('#paymentDate').value + '+08:00').getTime() / 1000);
    if (!viewIsCurrent(view) || view.paymentBusy || !Number.isFinite(paidAt)) return;
    view.paymentBusy=true;
    $('#paymentSubmit').disabled = true;
    try {
      await api('/subscriptions', {method: 'POST', body: JSON.stringify({
        id: view.ledgerEditID, account, paid_at: paidAt, amount_usd: Number($('#paymentAmount').value)
      })});
      if (!viewIsCurrent(view)) return;
      resetPaymentForm(view);
      await loadAccounting(view);
      $('#paymentStatus').textContent = '';
    } catch (error) {
      if (viewIsCurrent(view)) $('#paymentStatus').textContent = error.message;
      handleAuth(error);
    } finally {
      view.paymentBusy=false;
      if (viewIsCurrent(view)) renderAccounting(view);
    }
  };
  $('#paymentCancel').onclick = () => resetPaymentForm(view);
  $('#ledgerPrevious').onclick = () => { view.ledgerOffset = Math.max(0, view.ledgerOffset - 50); loadAccounting(view); };
  $('#ledgerNext').onclick = () => { view.ledgerOffset += 50; loadAccounting(view); };
  renderAccounting(view);
}

function resetPaymentForm(view) {
  const $=view.select;
  view.ledgerEditID = 0;
  $('#paymentForm').reset();
  $('#paymentCancel').hidden = true;
  labeledIcon($('#paymentSubmit'),lc('add'),'plus');
}

function ledgerCells(row, values) {
  for (const value of values) {
    const cell = document.createElement('td');
    if (value && typeof value === 'object' && 'text' in value) {
      ledgerValue(cell, value);
      cell.className='metric-cell';
      cell.dataset.unit=value.unit.toLowerCase();
    }
    else cell.textContent = String(value);
    row.append(cell);
  }
}

function ledgerHeads(target, names) {
  const row = document.createElement('tr');
  for (const key of names) {
    const cell = document.createElement('th');
    cell.textContent = key ? lc(key) : '';
    if (['amount','tokens','usd','credits','peak'].includes(key)) cell.className='metric-cell';
    row.append(cell);
  }
  target.replaceChildren(row);
}

function renderLedgerMetrics(target, totals, ready = true) {
  target.replaceChildren();
  for (const [key, value] of [['tokens',totals.total_tokens],['requests',totals.requests],['usd',totals.usd_reference],['credits',totals.credits_reference]]) {
    const group = document.createElement('div');
    group.className='metric metric-'+key;
    const label = document.createElement('dt');
    const number = document.createElement('dd');
    const icon=document.createElement('span');
    icon.className='metric-icon';
    icon.innerHTML=uiIcon({tokens:'layers',requests:'activity',usd:'coins',credits:'credit-card'}[key]);
    label.append(icon,document.createTextNode(lc(key)));
    if (key==='usd' || key==='credits') label.title=lc('reference');
    const amount = ledgerAmount(ready ? value : NaN, key);
    const digits=document.createElement('span');
    digits.className='metric-number';
    digits.textContent=amount.value;
    number.append(digits);
    const unit = document.createElement('span');
    unit.className = 'accounting-unit';
    unit.textContent = amount.unit;
    number.append(unit);
    number.title = amount.title;
    const unpriced=key==='usd' ? totals.usd_unpriced_requests : key==='credits' ? totals.credits_unpriced_requests : 0;
    if (ready && unpriced) number.title += ' (' + lc('incomplete') + ': ' + ledgerAmount(unpriced,'requests').text + ')';
    group.append(label, number);
    target.append(group);
  }
}

function renderAccounting(view) {
  const $=view.select;
  const {ledgerState,ledgerOffset,ledgerEditID}=view;
  if (!$('#lifetimeLedger')) return;
  for (const [selector, key, icon] of [['#lifetimeLedger h2','lifetime','layers'],['#subscriptionLedger h2','subscriptions','credit-card'],
    ['#weeklyLedger h2','weeklyLedger','calendar-days'],['#fiveHourLedger h2','fiveHourLedger','clock-3']]) labeledIcon($(selector),lc(key),icon);
  for (const [selector,key] of [['#paymentDateLabel','paid'],['#paymentAmountLabel','amount']]) $(selector).textContent=lc(key);
  labeledIcon($('#paymentCancel'),lc('cancel'),'x');
  labeledIcon($('#ledgerPrevious'),lc('previous'),'chevron-left',true);
  labeledIcon($('#ledgerNext'),lc('following'),'chevron-right',true);
  $('#subscriptionLedger h2').title=lc('monthlyBasis');
  $('#weeklyLedger h2').title=lc('shared');
  $('#fiveHourLedger h2').title=lc('shared');
  labeledIcon($('#paymentSubmit'),lc(ledgerEditID ? 'save' : 'add'),ledgerEditID ? 'check' : 'plus');
  $('#paymentSubmit').disabled = view.paymentBusy || !ledgerState;
  $('#paymentCancel').disabled = view.paymentBusy;
  for (const input of $('#paymentForm').querySelectorAll('input')) input.disabled=view.paymentBusy;
  ledgerHeads($('#subscriptionHeads'), ['paid','next','status','amount','tokens','usd','credits','quota','']);
  for (const selector of ['#weeklyHeads','#fiveHourHeads']) ledgerHeads($(selector), ['scope','start','end','status','peak','tokens','usd','credits']);
  $('#ledgerPage').textContent = lc('page') + ' ' + (ledgerOffset / 50 + 1);
  $('#ledgerPrevious').disabled = ledgerOffset === 0;
  const data = ledgerState || {};
  const totals = data.lifetime || {};
  renderLedgerMetrics($('#ledgerMetrics'),totals);
  $('#ledgerUnpriced').textContent = lc('missing') + ledgerAmount(totals.usd_unpriced_requests, 'requests').text + ' / ' + ledgerAmount(totals.credits_unpriced_requests, 'requests').text;
  $('#ledgerUnpriced').hidden=!(totals.usd_unpriced_requests || totals.credits_unpriced_requests);
  $('#ledgerDropped').textContent = lc('dropped') + ledgerAmount(data.dropped_usage_events, 'requests').text;
  $('#ledgerDropped').hidden=!data.dropped_usage_events;
  $('#subscriptionRows').replaceChildren();
  const now=Date.now()/1000;
  const subscriptions=[...(data.subscriptions || [])].sort((a,b) => b.paid_at-a.paid_at || (b.id || 0)-(a.id || 0));
  for (const p of subscriptions) {
    const row = document.createElement('tr');
    const t = p.totals;
    const tokens = ledgerAmount(t.total_tokens, 'tokens');
    if (p.partial) tokens.title += ' (' + lc('incomplete') + ')';
    ledgerCells(row, [ledgerDate(p.paid_at), p.next_paid_at ? ledgerDate(p.next_paid_at) : '-', '',
      ledgerAmount(p.amount_usd, 'usd'), tokens,
      t.usd_unpriced_requests ? '-' : ledgerAmount(t.usd_reference, 'usd'), t.credits_unpriced_requests ? '-' : ledgerAmount(t.credits_reference, 'credits'), ledgerQuotaText(p.observed_quota_growth)]);
    ledgerStatusCell(row,2,ledgerPeriodStatus(p,true,now));
    const actions = document.createElement('td');
    actions.className='table-actions';
    const edit = document.createElement('button');
    edit.type = 'button'; edit.className='icon-button';
    labeledIcon(edit,lc('edit'),'pencil',true);
    edit.disabled=view.paymentBusy;
    edit.onclick = () => {
      view.ledgerEditID = p.id;
      const local = new Date((p.paid_at + 8 * 3600) * 1000).toISOString().slice(0, 19);
      $('#paymentDate').value = local;
      $('#paymentAmount').value = p.amount_usd;
      $('#paymentCancel').hidden = false;
      labeledIcon($('#paymentSubmit'),lc('save'),'check');
      $('#paymentDate').focus();
    };
    const remove = document.createElement('button');
    remove.type = 'button'; remove.className='icon-button button-danger';
    labeledIcon(remove,lc('remove'),'trash-2',true);
    remove.disabled=view.paymentBusy;
    remove.onclick = async () => {
      if (!window.confirm(lc('confirm'))) return;
      const account = view.account;
      if (!viewIsCurrent(view) || view.paymentBusy) return;
      view.paymentBusy=true;
      remove.disabled = true;
      renderAccounting(view);
      try {
        await api('/subscriptions', {method: 'DELETE', body: JSON.stringify({account, id: p.id})});
        if (viewIsCurrent(view)) { resetPaymentForm(view); await loadAccounting(view); }
      } catch (error) {
        if (viewIsCurrent(view)) $('#paymentStatus').textContent = error.message;
        handleAuth(error);
      } finally {
        view.paymentBusy=false;
        if (viewIsCurrent(view)) renderAccounting(view);
      }
    };
    actions.append(edit, remove); row.append(actions); $('#subscriptionRows').append(row);
  }
  $('#weeklyRows').replaceChildren();
  $('#fiveHourRows').replaceChildren();
  const cycles=[...(data.resets || [])].sort((a,b) =>
    Number(ledgerPeriodStatus(b,false,now)==='ongoing')-Number(ledgerPeriodStatus(a,false,now)==='ongoing') ||
    b.started_at-a.started_at || (b.id || 0)-(a.id || 0));
  for (const cycle of cycles) {
    const row = document.createElement('tr'), t = cycle.totals;
    const weekly=cycle.scope==='weekly' || cycle.scope==='spark_weekly' ||
      (cycle.window_minutes >= 10020 && cycle.window_minutes <= 10140);
    const target=weekly ? '#weeklyRows' : '#fiveHourRows';
    row.title=lc('shared');
    ledgerCells(row, [cycle.scope, ledgerDate(cycle.started_at), ledgerDate(cycle.ended_at || cycle.reset_at), '',
      ledgerAmount(cycle.peak_percent, 'percent'), ledgerAmount(t.total_tokens, 'tokens'), t.usd_unpriced_requests ? '-' : ledgerAmount(t.usd_reference, 'usd'), t.credits_unpriced_requests ? '-' : ledgerAmount(t.credits_reference, 'credits')]);
    const scope=document.createElement('span');
    scope.className='scope-badge'; scope.dataset.scope=cycle.scope;
    scope.textContent=cycle.scope==='main' ? tr('main') : cycle.scope==='weekly' ? tr('weekly') : cycle.scope;
    row.firstElementChild.replaceChildren(scope);
    ledgerStatusCell(row,3,ledgerPeriodStatus(cycle,false,now));
    $(target).append(row);
  }
  for (const [selector, key, span] of [['#subscriptionRows','noPayments',9],['#weeklyRows','noResets',8],['#fiveHourRows','noResets',8]]) {
    if (!$(selector).children.length) {
      const row = document.createElement('tr'), cell = document.createElement('td');
      cell.colSpan = span; cell.textContent = lc(key); row.append(cell); $(selector).append(row);
    }
  }
  const counts = {};
  for (const row of data.resets || []) counts[row.scope] = (counts[row.scope] || 0) + 1;
  $('#ledgerNext').disabled = !Object.values(counts).some(count => count >= 50);
  $('#resetLedger .accounting-pages').hidden=ledgerOffset===0 && $('#ledgerNext').disabled;
}

async function loadAccounting(view) {
  const $=view.select;
  const generation = ++view.ledgerGeneration;
  const account=view.account;
  view.ledgerLoading=true;
  view.accountError='';
  renderAccounting(view);
  renderAllAccountsUsage();
  try {
    const data = await api('/accounting?' + new URLSearchParams({account, limit: '50', offset: String(view.ledgerOffset)}));
    if (generation !== view.ledgerGeneration || !viewIsCurrent(view)) return;
    view.ledgerState = data;
    renderAccounting(view);
    renderAccountStats(view);
  } catch (error) {
    if (generation !== view.ledgerGeneration || !viewIsCurrent(view)) return;
    view.accountError=error.message;
    renderAccountStats(view);
    handleAuth(error);
  } finally {
    if (generation === view.ledgerGeneration && viewIsCurrent(view)) {
      view.ledgerLoading=false;
      renderAllAccountsUsage();
    }
  }
}
