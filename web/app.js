'use strict';

// ---------- helpers ----------
const $page = document.getElementById('page');
const token = (document.cookie.match(/(?:^|; )kw=([^;]+)/) || [])[1] || '';

function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k === 'value') el.value = v;
    else if (k in el && typeof v !== 'string') el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const c of kids.flat(Infinity)) if (c != null && c !== false) el.append(c instanceof Node ? c : String(c));
  return el;
}
const mount = (...nodes) => $page.replaceChildren(...nodes.flat(Infinity).filter((n) => n != null && n !== false));

let toastTimer;
function toast(msg, ms = 3200) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.remove('show'), ms);
}

async function api(method, path, body) {
  const res = await fetch('/api' + path, {
    method, headers: { 'X-KeenWay': token, ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `Ошибка ${res.status}`);
  return data;
}

const gb = (n) => (n >= 1e9 ? `${(n / 1e9).toFixed(n >= 1e10 ? 0 : 1)} ГБ` : n >= 1e6 ? `${(n / 1e6).toFixed(0)} МБ` : `${Math.round(n / 1e3)} КБ`);
const plural = (n, a, b, c) => { const m10 = n % 10, m100 = n % 100; return m10 === 1 && m100 !== 11 ? a : m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20) ? b : c; };
const days = (n) => `${n} ${plural(n, 'день', 'дня', 'дней')}`;
const icon = (src, cls = 'icon') => { const i = h('img', { class: cls, alt: '', src: src || 'icon.svg', loading: 'lazy' }); i.onerror = () => { i.src = 'icon.svg'; }; return i; };
const delayClass = (d) => (!d ? 'bad' : d < 300 ? 'good' : d < 800 ? 'warn' : 'bad');
const delayText = (d) => (d ? `${d} мс` : '—');
function seg(options, value, onchange) {
  const wrap = h('div', { class: 'seg' });
  const draw = (v) => wrap.replaceChildren(...options.map(([val, label]) => h('button', { class: val === v ? 'on' : '', onclick: () => { draw(val); onchange(val); } }, label)));
  draw(value);
  return wrap;
}

// ---------- state ----------
const S = { state: null, status: null, devices: null, ui: { templateUpdater: null }, page: 'overview', timers: [] };

async function loadState() {
  const prev = S.state?.settings;
  S.state = await api('GET', '/state');
  // Pages keep a reference to the settings object; refresh it in place so later edits are not lost.
  if (prev) {
    for (const k of Object.keys(prev)) delete prev[k];
    S.state.settings = Object.assign(prev, S.state.settings);
  }
  // Go sends empty lists as null; the pages expect arrays.
  S.state.keys = S.state.keys || [];
  S.state.importedNodes = S.state.importedNodes || [];
  S.state.catalog = S.state.catalog || [];
  const set = S.state.settings;
  for (const k of ['subscriptions', 'services', 'sites', 'devices']) set[k] = set[k] || [];
  document.getElementById('applybar').hidden = !(S.state.currentPending || S.state.buildError) || !S.state.imported;
  const applyText = document.getElementById('applyText');
  document.getElementById('applyBtn').disabled = !!S.state.buildError;
  if (S.state.buildError) applyText.replaceChildren(h('strong', { style: 'color:var(--bad)' }, '⚠️ Конфиг не собирается: '), h('span', { class: 'small' }, S.state.buildError));
  else applyText.replaceChildren(h('strong', {}, 'Есть неприменённые изменения.'), ' ', h('span', { class: 'muted small' }, 'Они вступят в силу после применения на роутере.'));
  const r = S.state.router;
  document.getElementById('routerState').textContent = S.state.panel ? `Панель роутера ${r.host}` : S.state.hasPassword ? `Роутер ${r.host}` : 'Роутер не подключён';
}

let saveTimer, pendingSave;
function saveSettings(immediate) {
  clearTimeout(saveTimer);
  const run = async () => {
    pendingSave = null;
    try {
      await api('PUT', '/settings', { ...S.state.settings, keepSecret: true });
      await loadState();
    } catch (e) { toast('⚠️ ' + e.message, 5000); }
  };
  if (immediate) return run();
  saveTimer = setTimeout(run, 400);
  pendingSave = run;
}

// flushSave sends a pending (debounced) change right away, e.g. before applying.
async function flushSave() {
  if (!pendingSave) return;
  clearTimeout(saveTimer);
  const run = pendingSave;
  pendingSave = null;
  await run();
}

// ---------- navigation ----------
const pages = {};
function go(name) {
  S.timers.forEach(clearInterval);
  S.timers = [];
  S.page = name;
  document.querySelectorAll('nav button').forEach((b) => b.classList.toggle('on', b.dataset.page === name));
  if (!S.state.hasPassword && name !== 'settings') return pages.connect();
  pages[name]().catch((e) => mount(h('div', { class: 'alert bad' }, '⚠️ ', h('span', { class: 'grow' }, e.message))));
}
document.querySelectorAll('nav button').forEach((b) => b.addEventListener('click', () => go(b.dataset.page)));

// ---------- connect (onboarding) ----------
pages.connect = async function () {
  const r = S.state.router;
  const host = h('input', { value: r.host || '192.168.1.1' });
  const user = h('input', { value: r.user || 'admin' });
  const pass = h('input', { type: 'password', placeholder: 'пароль администратора роутера' });
  const remember = h('input', { type: 'checkbox', checked: r.remembered });
  const err = h('div', { class: 'small', style: 'color:var(--bad)' });
  const btn = h('button', { class: 'btn primary', type: 'submit' }, 'Подключиться');
  mount(
    h('h1', {}, 'Подключение к роутеру'),
    h('div', { class: 'card stack', style: 'max-width:520px' },
      h('p', { class: 'muted' }, 'Нужен Keenetic с установленным XKeen и ядром mihomo. Приложение заходит на роутер по SSH и больше никуда пароль не отправляет.'),
      h('form', { class: 'stack', onsubmit: async (e) => {
        e.preventDefault();
        err.textContent = '';
        btn.disabled = true;
        btn.replaceChildren(h('span', { class: 'spinner' }), ' Подключение…');
        try {
          const res = await api('POST', '/router', { host: host.value, user: user.value, password: pass.value, remember: remember.checked });
          S.ui.templateUpdater = res.templateUpdater;
          await loadState();
          toast(res.imported ? `Подключено. Загружен текущий конфиг с роутера (${res.mihomo})` : `Подключено: ${res.mihomo}`, 5000);
          if (res.warnings && res.warnings.length) toast('Не всё удалось перенести: ' + res.warnings.join('; '), 8000);
          go('overview');
        } catch (e2) { err.textContent = e2.message; }
        btn.disabled = false;
        btn.replaceChildren('Подключиться');
      } },
        h('div', { class: 'row' }, h('label', { class: 'field grow' }, h('span', {}, 'IP роутера'), host), h('label', { class: 'field grow' }, h('span', {}, 'Логин'), user)),
        h('label', { class: 'field' }, h('span', {}, 'Пароль'), pass),
        h('label', { class: 'check' }, remember, 'Запомнить пароль на этом компьютере'),
        err, btn)),
  );
  pass.focus();
};

// ---------- overview ----------
pages.overview = async function () {
  mount(h('h1', {}, 'Обзор'), h('div', { class: 'empty' }, h('span', { class: 'spinner' })));
  const st = await api('GET', '/status');
  S.status = st;
  const alerts = [];
  if (!st.online) alerts.push(h('div', { class: 'alert bad' }, '⛔', h('div', { class: 'grow' }, h('strong', {}, 'Панель mihomo не отвечает. '), st.error || '')));
  if (S.ui.templateUpdater ?? S.state.templateUpdater) {
    alerts.push(h('div', { class: 'alert warn' }, '⚠️',
      h('div', { class: 'grow' }, h('strong', {}, 'На роутере включено автообновление шаблона (update-config.sh). '), 'Раз в несколько дней оно перезаписывает группы и правила и сотрёт ваши сайты и правила устройств.'),
      h('button', { class: 'btn', onclick: async (e) => {
        if (!confirm('Отключить задание update-config.sh в планировщике роутера? Обновлением подписок и правил займётся приложение. Копия планировщика сохранится.')) return;
        e.target.disabled = true;
        try { await api('POST', '/updater/disable'); S.ui.templateUpdater = false; toast('Автообновление шаблона отключено'); go('overview'); } catch (er) { toast(er.message); e.target.disabled = false; }
      } }, 'Отключить')));
  }
  if (st.online && (st.notRunning || []).length) {
    alerts.push(h('div', { class: 'alert warn' }, '⚠️',
      h('div', { class: 'grow' }, h('strong', {}, 'Роутер работает на старых настройках. '),
        `Подписка «${st.notRunning.join('», «')}» сохранена в программе, но на роутере ещё не включена. Нажмите «Применить на роутере».`),
      h('button', { class: 'btn primary', onclick: () => runApply(false) }, 'Применить на роутере')));
  }
  const vpn = (st.groups || []).find((g) => g.name === 'VPN');
  const auto = vpn && (st.groups || []).find((g) => g.name === vpn.now);
  const nowNode = auto && auto.now ? `${vpn.now} → ${auto.now}` : vpn ? vpn.now : '—';
  const tr = st.traffic || {};

  mount(
    h('h1', {}, 'Обзор'),
    alerts,
    h('div', { class: 'grid' },
      h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'Роутер'), h('div', { class: 'big' }, st.online ? '🟢' : '🔴'),
        h('div', {}, `${S.state.router.host} · mihomo ${st.version || '?'}`)),
      h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'VPN сейчас идёт через'), h('div', { style: 'font-weight:700;margin-top:6px' }, nowNode),
        h('div', { class: 'row', style: 'margin-top:8px' }, h('button', { class: 'btn sm', onclick: () => go('servers') }, 'Сменить сервер'))),
      h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'Трафик с запуска mihomo'), h('div', { class: 'big' }, gb((tr.upload || 0) + (tr.download || 0))),
        h('div', { class: 'muted small' }, `↑ ${gb(tr.upload || 0)} · ↓ ${gb(tr.download || 0)} · ${tr.connections || 0} соединений`)),
    ),
    st.online ? profilesCard() : null,
    st.online ? routeCard() : null,
    h('h2', {}, 'Подписки'),
    (st.providers || []).length ? h('div', { class: 'grid' }, st.providers.map(subscriptionCard)) : h('div', { class: 'card empty' }, 'Подписок нет. ', h('a', { href: '#', onclick: (e) => { e.preventDefault(); go('keys'); } }, 'Подключить подписку или ключ')),
  );
};

function subscriptionCard(p) {
  const info = p.subscriptionInfo;
  const used = info ? info.Upload + info.Download : 0;
  const pct = info && info.Total ? Math.min(100, (used / info.Total) * 100) : 0;
  const left = info && info.Expire ? Math.max(0, Math.floor((info.Expire * 1000 - Date.now()) / 864e5)) : null;
  const sev = left !== null && left <= 3 ? 'bad' : left !== null && left <= 7 ? 'warn' : '';
  const updateBtn = h('button', { class: 'btn sm', onclick: async () => {
    updateBtn.disabled = true;
    try { await api('POST', '/provider/update', { name: p.name }); toast('Подписка обновлена'); go('overview'); } catch (e) { toast(e.message); updateBtn.disabled = false; }
  } }, '↻ Обновить сейчас');
  return h('div', { class: 'card' },
    h('div', { class: 'row between' }, h('strong', {}, p.name), h('span', { class: `pill ${p.alive ? 'good' : 'bad'}` }, `${p.alive}/${p.nodes.length} серверов живы`)),
    left !== null
      ? h('div', { style: 'margin-top:8px' }, h('span', { class: 'big', style: sev ? `color:var(--${sev})` : '' }, left), ' ', h('span', { class: 'muted' }, `${plural(left, 'день', 'дня', 'дней')} осталось · до ${new Date(info.Expire * 1000).toLocaleDateString('ru-RU')}`))
      : h('div', { class: 'muted', style: 'margin-top:8px' }, info ? 'Без срока окончания' : 'Провайдер не сообщает срок и трафик'),
    info && h('div', {}, h('div', { class: `bar ${pct > 90 ? 'bad' : pct > 75 ? 'warn' : ''}` }, h('div', { style: `width:${info.Total ? pct : 0}%` })),
      h('div', { class: 'muted small' }, info.Total ? `${gb(used)} из ${gb(info.Total)}` : `${gb(used)} · безлимит`)),
    h('div', { class: 'row between', style: 'margin-top:8px' }, h('span', { class: 'muted small' }, p.updatedAt && !p.updatedAt.startsWith('0001') ? `обновлена ${new Date(p.updatedAt).toLocaleString('ru-RU')}` : ''), updateBtn),
  );
}


// ---------- profiles & route check (overview) ----------
function profilesCard() {
  const row = h('div', { class: 'row' }, (S.state.profiles || []).map((p) => {
    const b = h('button', { class: 'btn', title: p.About, onclick: async () => {
      b.disabled = true;
      try { const r = await api('POST', '/profile', { id: p.ID }); toast(`Режим «${p.Name}»: переключено ${r.changed}`); } catch (e) { toast(e.message, 5000); }
      b.disabled = false;
    } }, p.Name);
    return b;
  }));
  return h('div', { class: 'card stack', style: 'margin-top:12px' },
    h('div', { class: 'row between' }, h('strong', {}, 'Быстрый режим'), h('span', { class: 'muted small' }, 'переключает все плитки сразу, без применения конфига')),
    row);
}

function routeCard() {
  const input = h('input', { placeholder: 'Сайт, например web.telegram.org или ссылка', class: 'grow' });
  const out = h('div');
  const run = async () => {
    if (!input.value.trim()) return input.focus();
    out.replaceChildren(h('span', { class: 'spinner sm' }), ' Проверка…');
    try {
      const r = await api('POST', '/route', { query: input.value });
      const last = r.chain[r.chain.length - 1];
      const direct = last === 'DIRECT';
      const blocked = last === 'REJECT' || last === 'REJECT-DROP';
      const rule = r.rule === 'Match' ? 'не подошло ни одно правило (группа «Остальное»)' : `правило ${r.rule}${r.payload ? ' · ' + r.payload : ''}`;
      out.replaceChildren(h('div', { class: `alert ${blocked ? 'bad' : direct ? 'warn' : 'good'}`, style: 'margin:8px 0 0' },
        blocked ? '⛔' : direct ? '➡️' : '🛡️',
        h('div', { class: 'grow' },
          h('strong', {}, blocked ? 'Заблокирован' : direct ? 'Идёт напрямую, без VPN' : 'Идёт через VPN'),
          h('div', { class: 'small' }, r.chain.join(' → ')),
          h('div', { class: 'small muted' }, rule)),
        direct ? h('button', { class: 'btn sm', onclick: async () => {
          const st = S.state.settings;
          st.sites.unshift({ name: uniqueSiteName(r.host, null), icon: '', domains: [r.host.replace(/^www\./, '')], policy: 'vpn' });
          await saveSettings(true);
          toast('Добавлено в «Свои сайты». Нажмите «Применить на роутере».', 5000);
        } }, 'Пустить через VPN') : null));
    } catch (e) { out.replaceChildren(h('div', { class: 'alert bad', style: 'margin:8px 0 0' }, '⚠️', h('span', { class: 'grow' }, e.message))); }
  };
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });
  return h('div', { class: 'card stack', style: 'margin-top:12px' },
    h('div', { class: 'row between' }, h('strong', {}, 'Через что идёт сайт?'), h('span', { class: 'muted small' }, 'правила для отдельных устройств здесь не учитываются')),
    h('div', { class: 'row' }, input, h('button', { class: 'btn primary', onclick: run }, 'Проверить')), out);
}

// ---------- connection: subscription / keys ----------
const UNITS = [[60, 'мин'], [3600, 'ч'], [86400, 'дн']];
function intervalControl(seconds, onchange) {
  const unit = [...UNITS].reverse().find(([u]) => seconds % u === 0 && seconds >= u) || UNITS[1];
  const num = h('input', { type: 'number', min: 1, value: Math.max(1, Math.round(seconds / unit[0])), style: 'width:80px' });
  const sel = h('select', { style: 'width:auto' }, UNITS.map(([u, l]) => h('option', { value: u, selected: u === unit[0] }, l)));
  const fire = () => onchange(Math.max(60, Number(num.value || 1) * Number(sel.value)));
  num.onchange = fire;
  sel.onchange = fire;
  return h('div', { class: 'row' }, h('span', { class: 'small muted' }, 'обновлять каждые'), num, sel);
}

function subNameFromURL(url, existing) {
  let base = 'Подписка';
  try { base = new URL(url).hostname.split('.').slice(-2, -1)[0] || base; } catch {}
  base = base.replace(/[^\p{L}\p{N}_ -]/gu, '').slice(0, 24) || 'Подписка';
  let name = base, i = 2;
  while (existing.some((s) => s.name === name)) name = `${base}-${i++}`;
  return name;
}
const maskURL = (u) => u.replace(/(\/\/[^/]+\/).{6,}/, '$1••••••');

async function checkSub(s, out) {
  out.textContent = 'Проверка…';
  try {
    const r = await api('POST', '/subscriptions/check', { url: s.url, headers: s.headers });
    out.textContent = `${r.title ? r.title + ' · ' : ''}${r.nodes.length} серверов` + (r.hasInfo ? ` · ${gb(r.used)}${r.total ? ' из ' + gb(r.total) : ''}` + (r.daysLeft >= 0 ? ` · осталось ${days(r.daysLeft)}` : ' · без срока') : '');
  } catch (er) { out.textContent = '⚠️ ' + er.message; }
}

function subCard(s, idx) {
  const st = S.state;
  const info = h('div', { class: 'small muted' });
  const urlLine = h('div', { class: 'mono small muted', style: 'word-break:break-all' }, maskURL(s.url));
  const editBox = h('div', { class: 'row', hidden: true });
  const newURL = h('input', { placeholder: 'Новая ссылка подписки https://…', class: 'grow' });
  editBox.append(newURL,
    h('button', { class: 'btn primary', onclick: async () => {
      const items = await api('POST', '/parse', { text: newURL.value.trim() }).catch((e) => { toast(e.message); return []; });
      const sub = items.find((i) => i.kind === 'subscription' && !i.error);
      if (!sub) return toast('Это не ссылка подписки. Ключ (vless://…) вставьте в поле ниже.', 5000);
      s.url = sub.url;
      await saveSettings(true);
      toast('Ссылка подписки заменена');
      go('keys');
    } }, 'Сохранить'),
    h('button', { class: 'btn', onclick: () => { editBox.hidden = true; } }, 'Отмена'));
  return h('div', { class: 'card stack' },
    h('div', { class: 'row between' },
      h('div', { class: 'row' }, h('span', { class: 'pill accent' }, 'подписка'),
        h('input', { value: s.name, title: 'Название подписки', style: 'width:200px;font-weight:600', onchange: (e) => { s.name = e.target.value.trim(); saveSettings(); } })),
      h('div', { class: 'row' },
        h('button', { class: 'btn sm', onclick: () => { editBox.hidden = false; newURL.focus(); } }, '✎ Сменить ссылку'),
        h('button', { class: 'btn sm', onclick: (e) => { e.target.disabled = true; checkSub(s, info).finally(() => { e.target.disabled = false; }); } }, 'Проверить'),
        h('button', { class: 'btn sm danger', onclick: () => { if (confirm(`Удалить подписку «${s.name}»?`)) { st.settings.subscriptions.splice(idx, 1); saveSettings(true).then(() => go('keys')); } } }, 'Удалить'))),
    urlLine, editBox,
    intervalControl(s.interval || 3600, (sec) => { s.interval = sec; saveSettings(); }),
    info);
}

pages.keys = async function () {
  const st = S.state;
  const subs = st.settings.subscriptions;
  const keyRows = st.keys.length ? await api('POST', '/parse', { text: st.keys.join('\n') }) : [];
  const hasAny = subs.length || keyRows.length || st.importedNodes.length;

  // Step 2: paste a new link or key
  const ta = h('textarea', { rows: 4, placeholder: 'Вставьте ссылку подписки https://… или ключ vless://, hysteria2://, trojan://, ss://, vmess://, happ://add/…, конфиг WireGuard / AmneziaWG. Можно несколько строк.' });
  const result = h('div');
  const done = async (msg) => { ta.value = ''; await loadState(); toast(msg); go('keys'); };
  const parseBtn = h('button', { class: 'btn primary', onclick: async () => {
    if (!ta.value.trim()) return toast('Сначала вставьте ссылку или ключ');
    let items;
    try { items = await api('POST', '/parse', { text: ta.value }); } catch (e) { return toast(e.message, 5000); }
    const good = items.filter((i) => !i.error);
    const newSubs = good.filter((i) => i.kind === 'subscription');
    const newKeys = good.filter((i) => i.kind === 'proxy').map((i) => i.raw);
    const addSubs = () => newSubs.forEach((i) => subs.push({ name: subNameFromURL(i.url, subs), url: i.url, interval: 3600 }));
    const act = (fn, msg) => async () => { try { await fn(); await saveSettings(true); await done(msg); } catch (e) { toast(e.message, 5000); } };
    const buttons = [];
    if (good.length) {
      if (subs.length && newSubs.length === 1) {
        // The common case: the old subscription ran out, put the new link in its place.
        const which = subs.length > 1 ? h('select', { style: 'width:auto' }, subs.map((s, i) => h('option', { value: i }, s.name))) : null;
        buttons.push(h('button', { class: 'btn primary', onclick: act(async () => {
          subs[which ? Number(which.value) : 0].url = newSubs[0].url;
          if (newKeys.length) await api('PUT', '/keys', { keys: [...st.keys, ...newKeys] });
        }, 'Подписка заменена') }, which ? 'Заменить подписку' : `Заменить «${subs[0].name}»`), which);
      }
      buttons.push(h('button', { class: `btn ${buttons.length ? '' : 'primary'}`, onclick: act(async () => {
        addSubs();
        if (newKeys.length) await api('PUT', '/keys', { keys: [...st.keys, ...newKeys] });
      }, `Добавлено: ${good.length}`) }, subs.length && newSubs.length ? 'Добавить как ещё одну' : `Добавить (${good.length})`));
      if (!newSubs.length && st.keys.length) {
        buttons.push(h('button', { class: 'btn', onclick: act(async () => { await api('PUT', '/keys', { keys: newKeys }); }, 'Ключи заменены') }, 'Заменить старые ключи'));
      }
    }
    result.replaceChildren(h('div', { class: 'stack', style: 'margin-top:12px' },
      items.map((i) => h('div', { class: 'row' },
        h('span', { class: `pill ${i.error ? 'bad' : i.kind === 'subscription' ? 'accent' : 'good'}` }, i.error ? 'ошибка' : i.kind === 'subscription' ? 'подписка' : i.type),
        h('span', { class: 'grow' }, i.error ? h('span', {}, h('span', { class: 'mono small' }, i.raw.slice(0, 60)), ' — ', h('span', { style: 'color:var(--bad)' }, i.error)) : i.kind === 'subscription' ? maskURL(i.url) : `${i.name} · ${i.server}`))),
      buttons.length ? h('div', { class: 'row' }, buttons) : null));
  } }, 'Распознать');

  mount(
    h('h1', {}, 'Подключение'),
    h('p', { class: 'muted' }, 'Здесь меняется подписка или ключ VPN. Сервисы, сайты и правила при этом не трогаются.'),

    h('h2', {}, '1. Сейчас подключено'),
    hasAny ? [
      subs.map(subCard),
      (keyRows.length || st.importedNodes.length) ? h('div', { class: 'card' }, h('table', {},
        h('tr', {}, h('th', {}, 'Ключ'), h('th', {}, 'Тип'), h('th', {}, 'Сервер'), h('th', {})),
        keyRows.map((k, i) => h('tr', {}, h('td', {}, k.name || '—'), h('td', {}, h('span', { class: 'pill' }, k.type || '?')), h('td', { class: 'mono small' }, k.server || ''),
          h('td', {}, h('button', { class: 'btn sm danger', onclick: async () => { if (!confirm(`Удалить ключ «${k.name}»?`)) return; const keys = [...st.keys]; keys.splice(i, 1); await api('PUT', '/keys', { keys }); await loadState(); go('keys'); } }, 'Удалить')))),
        st.importedNodes.map((k) => h('tr', {}, h('td', {}, k.name, ' ', h('span', { class: 'pill' }, 'из роутера')), h('td', {}, h('span', { class: 'pill' }, k.type)), h('td', { class: 'mono small' }, k.server), h('td', {}))))) : null,
    ] : h('div', { class: 'card empty' }, 'Пока ничего не подключено — вставьте ссылку ниже'),

    h('h2', {}, hasAny ? '2. Сменить или добавить' : '2. Вставьте ссылку или ключ'),
    h('div', { class: 'card stack' }, ta, h('div', { class: 'row' }, parseBtn,
      h('span', { class: 'muted small' }, subs.length ? 'Новую подписку можно поставить вместо старой или добавить второй.' : 'Ключ сразу попадёт в конфиг по умолчанию.')), result),

    h('h2', {}, '3. Отдельные сайты (по желанию)'),
    h('p', { class: 'muted' }, 'Если какой-то сайт не открывается или, наоборот, должен идти без VPN — добавьте его домен. Добавить или изменить можно в любой момент, в разделе «Свои сайты».'),
    siteEditor(null),
  );
};

// ---------- services ----------
pages.services = async function () {
  const st = S.state.settings;
  const enabled = new Set(st.services);
  st.serviceVPN = st.serviceVPN || {};
  const save = () => { st.services = S.state.catalog.map((c) => c.id).filter((id) => enabled.has(id)); saveSettings(); };
  const tile = (svc) => {
    const via = svc.id in st.serviceVPN ? st.serviceVPN[svc.id] : svc.viaVPN;
    const card = h('div', { class: `card tile ${enabled.has(svc.id) ? '' : 'off'}` });
    const box = h('input', { type: 'checkbox', checked: enabled.has(svc.id), onchange: (e) => {
      if (e.target.checked) enabled.add(svc.id); else enabled.delete(svc.id);
      card.classList.toggle('off', !e.target.checked);
      save();
    } });
    card.append(h('label', { class: 'check' }, box), icon(svc.icon), h('div', { class: 'grow' },
      h('div', { style: 'font-weight:600' }, svc.name),
      svc.includes ? h('div', { class: 'muted small clamp', title: svc.includes }, svc.includes) : null,
      h('div', { style: 'margin-top:6px' }, seg([[true, 'VPN'], [false, 'Напрямую']], via, (v) => { if (v === svc.viaVPN) delete st.serviceVPN[svc.id]; else st.serviceVPN[svc.id] = v; saveSettings(); }))));
    return card;
  };
  const cats = [];
  for (const svc of S.state.catalog) {
    const c = svc.category || 'Прочее';
    let g = cats.find((x) => x.name === c);
    if (!g) cats.push(g = { name: c, items: [] });
    g.items.push(svc);
  }
  const allOn = S.state.catalog.every((c) => enabled.has(c.id));
  mount(
    h('div', { class: 'row between' }, h('h1', {}, 'Сервисы'),
      h('button', { class: 'btn', onclick: async () => {
        if (allOn) S.state.catalog.forEach((c) => { if (!c.default) enabled.delete(c.id); else enabled.add(c.id); });
        else S.state.catalog.forEach((c) => enabled.add(c.id));
        st.services = S.state.catalog.map((c) => c.id).filter((id) => enabled.has(id));
        await saveSettings(true);
        go('services');
      } }, allOn ? 'Вернуть набор по умолчанию' : 'Включить все')),
    h('p', { class: 'muted' }, 'Каждый включённый сервис — отдельная плитка в zashboard. «VPN» или «Напрямую» — что выбрано по умолчанию; переключать можно и потом в zashboard. Выключенный сервис идёт по общим правилам.'),
    cats.map((g) => [h('h2', {}, g.name), h('div', { class: 'grid small' }, g.items.map(tile))]),
    h('h2', {}, 'Дополнительно'),
    h('div', { class: 'card stack' },
      h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: st.blockAds, onchange: (e) => { st.blockAds = e.target.checked; saveSettings(); } }), 'Блокировать рекламу (список zxc-rv/ad-filter)'),
      h('div', { class: 'row' }, h('span', {}, 'Режим выбора сервера:'),
        seg([['simple', 'Простой: VPN + самый быстрый'], ['extended', 'Расширенный: Auto-Best + Fallback + ручной']], st.mode, (v) => { st.mode = v; saveSettings(); }))),
  );
};

// ---------- own sites ----------
const ROUTES = [['vpn', 'Через VPN'], ['direct', 'Напрямую, без VPN'], ['group', 'Отдельная плитка в zashboard (с иконкой)'], ['block', 'Заблокировать']];
const QURE = 'https://github.com/Koolson/Qure/tree/master/IconSet/Color';
const favicon = (d) => `https://www.google.com/s2/favicons?domain=${encodeURIComponent(d)}&sz=128`;
function cleanDomain(v) {
  v = v.trim().toLowerCase();
  if (!v) return '';
  try { if (/^[a-z]+:\/\//.test(v)) v = new URL(v).hostname; } catch {}
  return v.replace(/^\*\./, '').replace(/^www\./, '').replace(/[/:].*$/, '').replace(/\.$/, '');
}
const splitList = (v) => v.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean);
function uniqueSiteName(base, self) {
  const taken = new Set(S.state.settings.sites.filter((x) => x !== self).map((x) => x.name));
  let name = base, i = 2;
  while (taken.has(name)) name = `${base} ${i++}`;
  return name;
}

// siteEditor edits one entry of settings.sites. With site == null it is a "new site"
// form that only joins the list when saved (an empty site would not pass the check).
function siteEditor(site, opts = {}) {
  const isNew = !site;
  const st = S.state.settings;
  const s = site || { name: '', icon: '', domains: [], ips: [], ports: [], policy: 'vpn', target: '' };
  s.domains = s.domains || [];
  const changed = () => { if (!isNew) saveSettings(); };

  const preview = icon(s.icon, 'icon lg');
  const name = h('input', { value: s.name, placeholder: 'Например: Кинопоиск', onchange: (e) => {
    const v = e.target.value.trim();
    if (!v && !isNew) { e.target.value = s.name; toast('Название не может быть пустым'); return; }
    if (/[,\n]/.test(v) || v.length > 40) { e.target.value = s.name; toast('Название: до 40 символов, без запятых'); return; }
    s.name = v; changed();
  } });

  // Domains as chips + an "add" field; works the same for a new site and later.
  const chips = h('div', { class: 'chips' });
  const drawChips = () => chips.replaceChildren(...(s.domains.length ? s.domains.map((d, i) => h('span', { class: 'chip' }, d,
    h('button', { title: 'Убрать', onclick: () => { s.domains.splice(i, 1); drawChips(); changed(); } }, '×'))) : [h('span', { class: 'muted small' }, 'Доменов пока нет')]));
  drawChips();
  const domIn = h('input', { placeholder: 'kinopoisk.ru или ссылка на сайт', class: 'grow' });
  const addDomain = () => {
    const add = splitList(domIn.value).map(cleanDomain).filter((d) => d && !s.domains.includes(d));
    if (!add.length) return domIn.focus();
    if (add.some((d) => !/^[a-z0-9.-]+\.[a-z0-9-]{2,}$/.test(d))) return toast('Похоже, это не домен. Пример: youtube.com', 4000);
    s.domains.push(...add);
    domIn.value = '';
    if (!s.name) { s.name = add[0]; name.value = s.name; }
    drawChips();
    changed();
    domIn.focus();
  };
  domIn.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); addDomain(); } });

  // Where the traffic goes.
  const current = s.target ? `t:${s.target}` : s.policy || 'vpn';
  const iconBox = h('div', { class: 'stack', hidden: current !== 'group' });
  const route = h('select', { onchange: (e) => {
    const v = e.target.value;
    if (v.startsWith('t:')) { s.target = v.slice(2); s.policy = 'vpn'; } else { s.target = ''; s.policy = v; }
    iconBox.hidden = v !== 'group';
    changed();
  } },
  ROUTES.map(([v, l]) => h('option', { value: v, selected: current === v }, l)),
  h('optgroup', { label: 'Вместе с сервисом (как он настроен)' }, S.state.catalog.filter((c) => st.services.includes(c.id)).map((c) => h('option', { value: `t:${c.id}`, selected: current === `t:${c.id}` }, c.name))));

  const iconIn = h('input', { value: s.icon || '', placeholder: 'https://…/icon.png', class: 'grow', onchange: (e) => setIcon(e.target.value.trim()) });
  const setIcon = (v) => { s.icon = v; iconIn.value = v; preview.src = v || 'icon.svg'; changed(); };
  iconBox.append(
    h('label', { class: 'field' }, h('span', {}, 'Иконка плитки (по желанию)'), h('div', { class: 'row' }, iconIn,
      h('button', { class: 'btn sm', onclick: () => s.domains[0] ? setIcon(favicon(s.domains[0])) : toast('Сначала добавьте домен') }, 'Взять значок сайта'))),
    h('details', { class: 'help' }, h('summary', {}, 'Как поставить свою иконку'),
      h('ol', {},
        h('li', {}, 'Проще всего — кнопка «Взять значок сайта»: подставится значок первого домена.'),
        h('li', {}, 'Красивые готовые иконки: откройте ', h('a', { href: QURE, target: '_blank', rel: 'noopener' }, 'набор Qure'), ', найдите нужную (например Netflix.png), нажмите на неё, потом «Raw» и скопируйте адрес из строки браузера.'),
        h('li', {}, 'Подойдёт любая прямая ссылка на картинку PNG или SVG, лучше квадратную, от 64×64. Ссылка должна открываться без входа в аккаунт.'),
        h('li', {}, 'Иконку видно в zashboard на плитке этого сайта. Если оставить пустым — будет стандартная.'))));

  // Rarely needed: IP ranges and ports.
  const lines = (a) => (a || []).join('\n');
  const adv = h('details', { class: 'help', open: !!((s.ips || []).length || (s.ports || []).length) }, h('summary', {}, 'IP-адреса и порты (для опытных)'),
    h('div', { class: 'row', style: 'align-items:flex-start;margin-top:8px' },
      h('label', { class: 'field grow' }, h('span', {}, 'IP или подсети, по одной в строке'), h('textarea', { rows: 3, value: lines(s.ips), placeholder: '203.0.113.0/24', onchange: (e) => { s.ips = splitList(e.target.value); changed(); } })),
      h('label', { class: 'field grow' }, h('span', {}, 'Порты'), h('textarea', { rows: 3, value: lines(s.ports), placeholder: '30000-31000', onchange: (e) => { s.ports = splitList(e.target.value); changed(); } }))));

  const actions = isNew
    ? h('div', { class: 'row' }, h('button', { class: 'btn primary', onclick: async () => {
      if (domIn.value.trim()) addDomain();
      if (!s.domains.length && !(s.ips || []).length && !(s.ports || []).length) return toast('Добавьте хотя бы один домен');
      s.name = uniqueSiteName(s.name || s.domains[0] || 'Мой сайт', s);
      st.sites.unshift(s);
      try { await saveSettings(true); toast(`Сайт «${s.name}» добавлен`); go(opts.after || S.page); } catch (e) { st.sites.shift(); toast(e.message, 5000); }
    } }, '＋ Добавить сайт'), opts.onCancel ? h('button', { class: 'btn', onclick: opts.onCancel }, 'Отмена') : null)
    : h('div', { class: 'row between' }, h('span', { class: 'muted small' }, 'Изменения сохраняются сами'),
      h('button', { class: 'btn sm danger', onclick: () => { if (confirm(`Удалить «${s.name}»?`)) { st.sites.splice(st.sites.indexOf(s), 1); saveSettings(true).then(() => go('sites')); } } }, 'Удалить'));

  return h('div', { class: 'card stack site' },
    h('div', { class: 'row' }, preview, h('label', { class: 'field grow' }, h('span', {}, 'Название'), name),
      h('label', { class: 'field', style: 'min-width:260px' }, h('span', {}, 'Куда направлять'), route)),
    h('div', { class: 'field' }, h('span', { class: 'lbl' }, 'Домены — поддомены включаются сами'), chips,
      h('div', { class: 'row', style: 'margin-top:8px' }, domIn, h('button', { class: 'btn', onclick: addDomain }, '＋ Домен'))),
    iconBox, adv, actions);
}

pages.sites = async function () {
  const st = S.state.settings;
  const slot = h('div');
  const openNew = () => { slot.replaceChildren(siteEditor(null, { after: 'sites', onCancel: () => slot.replaceChildren() })); slot.querySelector('input').focus(); };
  mount(
    h('div', { class: 'row between' }, h('h1', {}, 'Свои сайты'), h('button', { class: 'btn primary', onclick: openNew }, '＋ Добавить сайт')),
    h('p', { class: 'muted' }, 'Сайты отсюда важнее сервисов. Домены можно дописывать в любой момент — кнопка «＋ Домен» в нужной карточке.'),
    slot,
    st.sites.length ? st.sites.map((site) => siteEditor(site)) : h('div', { class: 'card empty' }, 'Своих сайтов пока нет. ', h('a', { href: '#', onclick: (e) => { e.preventDefault(); openNew(); } }, 'Добавить первый')),
  );
};

// ---------- devices ----------
pages.devices = async function () {
  const st = S.state.settings;
  st.devices = st.devices || [];
  mount(h('h1', {}, 'Устройства'), h('div', { class: 'empty' }, h('span', { class: 'spinner' }), ' Получение данных с роутера…'));
  let devs = [];
  let note = null;
  try { devs = await api('GET', '/devices'); S.devices = devs; } catch (e) { note = h('div', { class: 'alert warn' }, '⚠️', h('span', { class: 'grow' }, 'Список устройств недоступен: ' + e.message + '. Можно добавить IP вручную.')); }
  const ruleFor = (ip) => st.devices.find((d) => d.ip === ip);
  const setRule = (name, ip, policy) => {
    st.devices = st.devices.filter((d) => d.ip !== ip);
    if (policy) st.devices.push({ name, ip, policy });
    saveSettings();
  };
  const policySelect = (name, ip) => h('select', { style: 'width:auto', onchange: (e) => setRule(name, ip, e.target.value) },
    [['', 'Как у всех'], ['direct', 'Всегда напрямую'], ['vpn', 'Всегда через VPN'], ['block', 'Без интернета']].map(([v, l]) => h('option', { value: v, selected: (ruleFor(ip)?.policy || '') === v }, l)));
  const known = new Set(devs.map((d) => d.ip));
  const manualName = h('input', { placeholder: 'Название', style: 'width:200px' });
  const manualIP = h('input', { placeholder: '192.168.1.50', style: 'width:160px' });
  mount(
    h('h1', {}, 'Устройства'),
    h('p', { class: 'muted' }, 'Правило для устройства важнее всего остального: «Всегда напрямую» — VPN для него выключен, «Всегда через VPN» — весь его трафик идёт через VPN.'),
    note,
    h('div', { class: 'card' }, h('table', {},
      h('tr', {}, h('th', {}, 'Устройство'), h('th', {}, 'IP'), h('th', {}, 'MAC'), h('th', {}, 'Правило'), h('th', {})),
      devs.filter((d) => d.ip).map((d) => h('tr', {},
        h('td', {}, d.active ? '🟢 ' : '⚪ ', d.name || d.hostname || 'без имени'),
        h('td', { class: 'mono small' }, d.ip), h('td', { class: 'mono small muted' }, d.mac),
        h('td', {}, policySelect(d.name || d.hostname || d.mac, d.ip)),
        h('td', {}, h('button', { class: 'btn sm', title: 'Чтобы правило не слетело, когда роутер выдаст устройству другой IP', onclick: async (e) => {
          if (!confirm(`Закрепить за «${d.name || d.mac}» адрес ${d.ip} в DHCP роутера?`)) return;
          e.target.disabled = true;
          try { await api('POST', '/devices/pin', { mac: d.mac, ip: d.ip }); toast('IP закреплён'); } catch (er) { toast(er.message); }
          e.target.disabled = false;
        } }, '📌 Закрепить IP')))),
      st.devices.filter((d) => !known.has(d.ip)).map((d) => h('tr', {}, h('td', {}, d.name, ' ', h('span', { class: 'pill' }, 'вручную')), h('td', { class: 'mono small' }, d.ip), h('td', {}), h('td', {}, policySelect(d.name, d.ip)), h('td', {}))),
    )),
    h('div', { class: 'card row' }, h('strong', {}, 'Добавить вручную:'), manualName, manualIP,
      h('button', { class: 'btn', onclick: () => { if (!/^\d+\.\d+\.\d+\.\d+$/.test(manualIP.value.trim())) return toast('Укажите IP, например 192.168.1.50'); setRule(manualName.value.trim() || manualIP.value.trim(), manualIP.value.trim(), 'direct'); go('devices'); } }, 'Добавить')),
  );
};

// ---------- servers ----------
pages.servers = async function () {
  mount(h('h1', {}, 'Серверы'), h('div', { class: 'empty' }, h('span', { class: 'spinner' })));
  const st = await api('GET', '/status');
  if (!st.online) return mount(h('h1', {}, 'Серверы'), h('div', { class: 'alert bad' }, '⛔ ', h('span', { class: 'grow' }, st.error)));
  const all = st.groups || [];
  const byName = Object.fromEntries(all.map((g) => [g.name, g]));
  const vpn = byName.VPN;
  // The group that lists every server: "⚙️ Manual" in extended mode, VPN itself in simple mode.
  const manual = all.find((g) => /Manual/.test(g.name) && g.type === 'Selector' && g.name !== 'VPN');
  const pool = manual || vpn;
  const isGroup = (n) => byName[n] || n === 'DIRECT' || n === 'REJECT' || n === 'PASS';
  const nodes = pool ? pool.options.filter((o) => !isGroup(o)) : [];
  const delays = {};
  for (const p of st.providers || []) for (const n of p.nodes) if (n.delay) delays[n.name] = n.delay;

  // What the VPN really uses now: follow the chain VPN → Auto-Best → server.
  const chain = [];
  for (let g = vpn, i = 0; g && i < 5; g = byName[g.now], i++) chain.push(g.now);
  const current = chain[chain.length - 1];

  const choose = async (group, option) => { await api('POST', '/select', { group, option }); };
  const pick = async (node) => {
    try {
      if (manual) { await choose(manual.name, node); await choose('VPN', manual.name); } else await choose('VPN', node);
      toast(`VPN → ${node}`);
      go('servers');
    } catch (e) { toast(e.message); }
  };

  // Auto-choice filter: words/flags of servers that Auto-Best / Fallback must skip.
  const settings = S.state.settings;
  const excluded = () => settings.autoExclude ?? S.state.defaultExclude ?? [];
  const isExcluded = (n) => excluded().some((w) => w && n.includes(w));
  const flagOf = (n) => (n.match(/\p{Regional_Indicator}{2}/u) || [])[0];
  const flags = {};
  for (const n of nodes) { const f = flagOf(n); if (f) flags[f] = (flags[f] || 0) + 1; }
  const setExcluded = (list) => { settings.autoExclude = list; saveSettings(); drawFilter(); draw(); };
  const filterBox = h('div', { class: 'chips' });
  const wordIn = h('input', { placeholder: 'исключить по слову, например LTE', style: 'max-width:260px' });
  const addWord = () => { const w = wordIn.value.trim(); if (w && !excluded().includes(w)) setExcluded([...excluded(), w]); wordIn.value = ''; };
  wordIn.addEventListener('keydown', (e) => { if (e.key === 'Enter') addWord(); });
  const drawFilter = () => {
    const ex = excluded();
    const chips = Object.entries(flags).sort((a, b) => b[1] - a[1]).map(([f, cnt]) => {
      const off = ex.includes(f);
      return h('button', { class: `chip toggle ${off ? 'off' : ''}`, title: off ? 'Не участвует в авто-выборе — нажмите, чтобы вернуть' : 'Нажмите, чтобы исключить из авто-выбора', onclick: () => setExcluded(off ? ex.filter((x) => x !== f) : [...ex, f]) }, `${f} ${cnt}`);
    });
    const words = ex.filter((w) => !flags[w]).map((w) => h('span', { class: 'chip' }, `без «${w}»`, h('button', { onclick: () => setExcluded(ex.filter((x) => x !== w)) }, '×')));
    filterBox.replaceChildren(...chips, ...words);
  };
  drawFilter();

  const search = h('input', { placeholder: `Поиск среди ${nodes.length} серверов…`, style: 'max-width:320px' });
  const tbody = h('tbody');
  const tbodyOff = h('tbody');
  const offSummary = h('summary', {});
  const offBox = h('details', { class: 'help', style: 'margin-top:10px' }, offSummary, h('table', { style: 'margin-top:8px' }, tbodyOff));
  let testing = false;
  const draw = () => {
    const q = search.value.trim().toLowerCase();
    const rank = (n) => (delays[n] > 0 ? delays[n] : testing && !(n in delays) ? 1e6 : 1e7);
    const list = nodes.filter((n) => !q || n.toLowerCase().includes(q)).sort((x, y) => rank(x) - rank(y));
    const row = (n) => {
      const d = delays[n];
      const cell = testing && !(n in delays) ? h('span', { class: 'spinner sm' }) : d > 0 ? delayText(d) : d === 0 ? 'нет ответа' : '—';
      return h('tr', { class: n === current ? 'current-row' : '' },
        h('td', {}, n === current ? '✅ ' : '', n),
        h('td', { class: `delay ${delayClass(d)}`, style: 'white-space:nowrap' }, cell),
        h('td', { style: 'text-align:right' }, n === current ? h('span', { class: 'pill good' }, 'сейчас') : h('button', { class: 'btn sm', onclick: () => pick(n) }, 'Выбрать')));
    };
    const on = list.filter((n) => !isExcluded(n)), off = list.filter(isExcluded);
    tbody.replaceChildren(...on.map(row));
    tbodyOff.replaceChildren(...off.map(row));
    offSummary.textContent = `Не участвуют в авто-выборе (${off.length}) — их можно выбрать вручную`;
    offBox.hidden = !off.length;
  };
  search.oninput = draw;

  const testBtn = h('button', { class: 'btn', onclick: () => runTest() }, '⚡ Пинг всех');
  const runTest = async () => {
    if (!pool || testing) return;
    testing = true;
    for (const n of nodes) delete delays[n];
    testBtn.disabled = true;
    testBtn.replaceChildren(h('span', { class: 'spinner sm' }), ' Пинг…');
    draw();
    try {
      const res = await api('POST', '/test', { group: pool.name });
      for (const n of nodes) delays[n] = res[n] || 0;
    } catch (e) { toast(e.message); }
    testing = false;
    testBtn.disabled = false;
    testBtn.replaceChildren('⚡ Пинг всех');
    draw();
  };

  const modes = vpn ? vpn.options.filter((o) => byName[o]) : [];
  mount(
    h('div', { class: 'row between' }, h('h1', {}, 'Серверы'), testBtn),
    vpn && h('div', { class: 'card stack' },
      h('div', { class: 'muted small' }, 'VPN сейчас идёт через'),
      h('div', { style: 'font-weight:700;font-size:1.1rem' }, chain.join(' → ')),
      modes.length > 1 && h('div', { class: 'row' }, h('span', { class: 'small muted' }, 'Режим:'),
        seg(modes.map((m) => [m, m]), vpn.now, async (m) => { try { await choose('VPN', m); toast(`VPN → ${m}`); go('servers'); } catch (e) { toast(e.message); } }))),
    h('div', { class: 'row between', style: 'margin-top:18px' }, h('h2', { style: 'margin:0' }, `Все серверы (${nodes.length})`), search),
    nodes.length ? h('div', { class: 'card stack', style: 'margin-top:10px' },
      h('div', { class: 'row between' }, h('strong', {}, 'Авто-выбор берёт серверы из стран:'), h('span', { class: 'muted small' }, 'нажмите на страну, чтобы исключить; изменения вступят в силу после применения')),
      h('div', { class: 'muted small' }, '🇷🇺 по умолчанию исключена: российские серверы в подписках — это обход белых списков на мобильном интернете. У них самый маленький пинг, поэтому авто-выбор всегда брал бы их, но заблокированные сайты через них не открываются.'),
      filterBox, h('div', { class: 'row' }, wordIn, h('button', { class: 'btn sm', onclick: addWord }, 'Исключить'))) : null,
    h('div', { class: 'card', style: 'margin-top:10px' }, nodes.length ? [h('table', {}, tbody), offBox] : h('div', { class: 'empty' }, 'Серверов нет — добавьте подписку или ключ')),
    h('h2', {}, 'Сервисы'),
    h('div', { class: 'grid small' }, all.filter((g) => !g.hidden && g.name !== 'VPN' && g !== manual && g.type === 'Selector').map((g) => h('div', { class: 'card tile' }, icon(g.icon),
      h('div', { class: 'grow' }, h('div', { style: 'font-weight:600' }, g.name),
        h('select', { onchange: async (e) => { try { await choose(g.name, e.target.value); toast(`${g.name} → ${e.target.value}`); } catch (er) { toast(er.message); } } }, g.options.map((o) => h('option', { value: o, selected: o === g.now }, o))))))),
  );
  draw();
  runTest(); // ping right away when the tab opens
};

// ---------- traffic ----------
pages.traffic = async function () {
  let prev = null;
  const names = {};
  for (const d of S.devices || []) names[d.ip] = d.name || d.hostname;
  for (const d of S.state.settings.devices || []) names[d.ip] = names[d.ip] || d.name;
  const draw = async () => {
    const st = await api('GET', '/status');
    if (!st.online || !st.traffic) return mount(h('h1', {}, 'Трафик'), h('div', { class: 'alert bad' }, '⛔ ', h('span', { class: 'grow' }, st.error || 'нет данных')));
    const t = st.traffic;
    const now = Date.now();
    let rate = null;
    if (prev) { const dt = (now - prev.at) / 1000; rate = { up: (t.upload - prev.up) / dt, down: (t.download - prev.down) / dt }; }
    prev = { at: now, up: t.upload, down: t.download };
    const table = (rows, label) => h('table', {},
      h('tr', {}, h('th', {}, label), h('th', {}, 'Соединений'), h('th', {}, 'Трафик'), h('th', {}, 'Через VPN')),
      rows.slice(0, 15).map((u) => h('tr', {}, h('td', {}, label === 'Устройство' ? `${names[u.key] || ''} ${u.key}`.trim() : u.key), h('td', {}, u.connections), h('td', {}, gb(u.upload + u.download)), h('td', {}, gb(u.viaVPN)))));
    mount(
      h('h1', {}, 'Трафик'),
      h('div', { class: 'grid' },
        h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'Скорость сейчас'), h('div', { class: 'big' }, rate ? `${gb(Math.max(0, rate.down))}/с` : '…'), h('div', { class: 'muted small' }, rate ? `↑ ${gb(Math.max(0, rate.up))}/с` : 'подсчёт…')),
        h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'Всего с запуска mihomo'), h('div', { class: 'big' }, gb(t.upload + t.download)), h('div', { class: 'muted small' }, `↑ ${gb(t.upload)} · ↓ ${gb(t.download)}`)),
        h('div', { class: 'card' }, h('div', { class: 'muted small' }, 'Открытых соединений'), h('div', { class: 'big' }, t.connections))),
      h('h2', {}, 'По устройствам (открытые соединения)'), h('div', { class: 'card' }, table(t.byDevice, 'Устройство')),
      h('h2', {}, 'По сервисам'), h('div', { class: 'card' }, table(t.byService, 'Сервис')),
    );
  };
  await draw();
  S.timers.push(setInterval(() => S.page === 'traffic' && draw().catch(() => {}), 3000));
};

// ---------- settings ----------

// ---------- settings: Telegram notifications ----------
function notifyCard() {
  const card = h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Уведомления в Telegram'), h('span', { class: 'spinner sm' }));
  api('GET', '/notify').then((n) => {
    const token = h('input', { placeholder: n.hasToken ? `сохранён: ${n.token} — чтобы сменить, вставьте новый` : 'вставьте токен, вида 123456789:AAH…', autocomplete: 'off', spellcheck: false });
    const chat = h('input', { value: n.chatId || '', placeholder: 'например 123456789', style: 'max-width:220px' });
    const days = h('input', { type: 'number', min: 1, max: 30, value: n.days || 3, style: 'width:80px' });
    const status = h('div', { class: 'small' });
    const findBtn = h('button', { class: 'btn sm', onclick: async () => {
      findBtn.disabled = true;
      try { const r = await api('POST', '/notify/chat', { token: token.value }); chat.value = r.chatId; status.textContent = `Найден чат: ${r.name}`; } catch (e) { status.textContent = '⚠️ ' + e.message; }
      findBtn.disabled = false;
    } }, 'Найти мой chat id');
    const save = async (enabled) => {
      status.replaceChildren(h('span', { class: 'spinner sm' }), enabled ? ' Установка на роутер и тестовое сообщение…' : ' Отключение…');
      try {
        const r = await api('PUT', '/notify', { enabled, token: token.value, chatId: chat.value, days: Number(days.value) || 3 });
        status.textContent = enabled
          ? (r.result === 'crond-started' ? '✅ Включено, тестовое сообщение отправлено. На роутере запущен планировщик задач (crond).' : '✅ Включено, тестовое сообщение отправлено.')
          : 'Уведомления отключены и удалены с роутера.';
        setTimeout(() => go('settings'), 2500);
      } catch (e) { status.textContent = '⚠️ ' + e.message; }
    };
    card.replaceChildren(
      h('div', { class: 'row between' }, h('h2', { style: 'margin:0' }, 'Уведомления в Telegram'), h('span', { class: `pill ${n.enabled ? 'good' : ''}` }, n.enabled ? 'включены' : 'выключены')),
      h('p', { class: 'muted small' }, 'Роутер сам раз в 30 минут проверяет подписку и серверы и пишет в Telegram: подписка заканчивается, трафик почти израсходован, серверы не отвечают и когда снова заработали. Компьютер для этого не нужен.'),
      h('ol', { class: 'small steps-list' },
        h('li', {}, 'Создайте бота: в Telegram откройте ', h('a', { href: 'https://t.me/BotFather', target: '_blank', rel: 'noopener' }, '@BotFather'), ' → /newbot → придумайте имя.'),
        h('li', {}, 'Откройте своего нового бота и нажмите «Старт».'),
        h('li', {}, 'Вставьте токен бота сюда и нажмите «Включить».')),
      h('label', { class: 'field' }, h('span', {}, 'Токен Telegram-бота'), token),
      h('div', { class: 'row' }, h('button', { class: 'btn primary', onclick: () => save(true) }, n.enabled ? 'Сохранить и проверить' : 'Включить'),
        n.enabled ? h('button', { class: 'btn danger', onclick: () => { if (confirm('Отключить уведомления и удалить их с роутера?')) save(false); } }, 'Отключить') : null),
      status,
      h('details', { class: 'help' }, h('summary', {}, 'Дополнительно'),
        h('div', { class: 'row', style: 'margin-top:8px;align-items:flex-end' },
          h('label', { class: 'field' }, h('span', {}, 'Предупреждать о конце подписки за, дней'), days),
          h('label', { class: 'field' }, h('span', {}, 'Chat id (находится сам)'), chat), findBtn),
        h('p', { class: 'muted small' }, 'Если на роутере не запущен планировщик задач (crond), он будет запущен. Вместе с ним начнут работать уже настроенные на роутере задания XKeen (еженедельное обновление).')));
  }).catch((e) => card.append(h('div', { class: 'alert bad' }, e.message)));
  return card;
}

// ---------- settings: backup to a file ----------
function backupCard() {
  const file = h('input', { type: 'file', accept: '.json,application/json', hidden: true, onchange: async () => {
    const f = file.files[0];
    if (!f) return;
    try {
      const data = JSON.parse(await f.text());
      if (!confirm('Заменить текущие настройки программы настройками из файла? На роутер они попадут после «Применить на роутере».')) return;
      await api('POST', '/restore', data);
      await loadState();
      toast('Настройки загружены из файла');
      go('settings');
    } catch (e) { toast('⚠️ ' + e.message, 5000); }
    file.value = '';
  } });
  return h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Резервная копия настроек'),
    h('p', { class: 'muted small' }, 'Все настройки программы в одном файле: подписки, ключи, сервисы, сайты, устройства. Пароль роутера и токен бота в файл не попадают. В файле есть ссылки подписок — не публикуйте его.'),
    h('div', { class: 'row' },
      h('button', { class: 'btn', onclick: async () => {
        try {
          const data = await api('GET', '/backup');
          const a = h('a', { href: URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })), download: `keenway-settings-${new Date().toISOString().slice(0, 10)}.json` });
          document.body.append(a); a.click(); a.remove();
          toast('Файл сохранён в «Загрузки»');
        } catch (e) { toast(e.message); }
      } }, '⬇ Сохранить в файл'),
      h('button', { class: 'btn', onclick: () => file.click() }, '⬆ Загрузить из файла'), file));
}

pages.settings = async function () {
  if (!S.state.hasPassword) return pages.connect();
  const st = S.state;
  const secretOut = h('span', { class: 'mono' });
  mount(
    h('h1', {}, 'Настройки'),
    st.panel
      ? h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Веб-панель роутера'),
          h('p', { class: 'muted small' }, 'KeenWay работает прямо на роутере и открывается с любого устройства в домашней сети. Сменить пароль: keenway panel password, выключить панель: keenway panel off (в терминале роутера).'),
          h('div', {}, h('button', { class: 'btn', onclick: () => { document.cookie = 'kw=; Max-Age=0; path=/'; location.href = '/'; } }, 'Выйти')))
      : h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Роутер'),
          h('div', {}, `${st.router.host} · вход ${st.router.user} · пароль ${st.router.remembered ? 'сохранён' : 'только на эту сессию'}`),
          h('button', { class: 'btn', onclick: () => pages.connect() }, 'Подключиться заново / другой роутер')),
    h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Пароль панели mihomo / zashboard (порт 9090)'),
      h('p', { class: 'muted small' }, 'Без пароля любое устройство в домашней сети может переключать VPN и видеть соединения. Если пароль забыт — его всегда можно посмотреть здесь: приложению достаточно доступа к роутеру.'),
      st.hasSecret
        ? h('div', { class: 'row' }, h('span', { class: 'pill good' }, 'пароль установлен'), secretOut,
            h('button', { class: 'btn sm', onclick: async () => { secretOut.textContent = (await api('POST', '/secret/reveal')).secret; } }, 'Показать'),
            h('button', { class: 'btn sm', onclick: () => setSecret(prompt('Новый пароль панели:', genSecret())) }, 'Сменить'),
            h('button', { class: 'btn sm danger', onclick: () => { if (confirm('Убрать пароль с панели?')) setSecret(''); } }, 'Убрать'))
        : h('div', { class: 'row' }, h('span', { class: 'pill warn' }, 'без пароля'), h('button', { class: 'btn', onclick: () => setSecret(prompt('Пароль для панели (zashboard спросит его один раз):', genSecret())) }, 'Поставить пароль'))),
    h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Резервные копии на роутере'),
      h('p', { class: 'muted small' }, 'Перед каждым применением старый конфиг сохраняется. Если после применения пропадёт интернет, роутер сам вернёт прошлый конфиг через 5 минут.'),
      h('div', { id: 'backups', class: 'small muted' }, h('button', { class: 'btn sm', onclick: async (e) => {
        const list = await api('GET', '/backups');
        e.target.parentElement.replaceChildren(...(list.length ? list.map((b) => h('div', { class: 'mono' }, b.split('/').pop())) : ['Копий пока нет']));
      } }, 'Показать список'))),
    notifyCard(),
    backupCard(),
    versionCard(),
    h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'Конфиг'),
      h('div', { class: 'row' }, h('button', { class: 'btn', onclick: showPreview }, 'Посмотреть итоговый config.yaml'))),
  );
};

function genSecret() {
  const a = new Uint8Array(12);
  crypto.getRandomValues(a);
  return Array.from(a, (b) => 'abcdefghjkmnpqrstuvwxyz23456789'[b % 31]).join('');
}

async function setSecret(value) {
  if (value === null) return;
  try {
    await api('PUT', '/settings', { ...S.state.settings, secret: value, keepSecret: false });
    await loadState();
    toast(value ? 'Пароль сохранён — примени изменения на роутере' : 'Пароль убран — примени изменения на роутере', 5000);
    go('settings');
  } catch (e) { toast(e.message); }
}

// ---------- apply ----------
async function showPreview() {
  try {
    const { yaml } = await api('GET', '/preview');
    const modal = h('div', { class: 'modal', onclick: (e) => { if (e.target === modal) modal.remove(); } },
      h('div', { class: 'card', style: 'width:min(900px,100%)' }, h('div', { class: 'row between' }, h('h2', { style: 'margin:0' }, 'config.yaml'), h('button', { class: 'btn', onclick: () => modal.remove() }, 'Закрыть')), h('pre', { class: 'yaml' }, yaml)));
    document.body.append(modal);
  } catch (e) { toast(e.message, 6000); }
}

// ---------- updates of KeenWay ----------
S.version = null;
async function checkVersion() {
  try { S.version = await api('GET', '/version'); } catch { return; }
  const btn = document.getElementById('updateBtn');
  btn.hidden = !S.version.newer;
  btn.textContent = `⬆️ Обновить до ${S.version.latest}`;
}
function showUpdate() {
  const v = S.version || {};
  const status = h('div', { class: 'muted small' });
  const goBtn = h('button', { class: 'btn primary', onclick: async () => {
    goBtn.disabled = true;
    status.textContent = 'Скачивание и установка…';
    try {
      const r = await api('POST', '/update');
      if (r.newWindow) {
        status.textContent = `Готово: KeenWay ${r.to} открывается в новом окне. Это окно можно закрыть.`;
        setTimeout(() => window.close(), 4000);
        return;
      }
      status.textContent = 'Обновление ставится на роутер, панель перезапустится (до минуты)…';
      const started = Date.now();
      const wait = async () => {
        try { const nv = await api('GET', '/version'); if (nv.current === r.to) { location.reload(); return; } } catch { /* restarting */ }
        if (Date.now() - started > 180_000) { status.textContent = 'Панель не вернулась за 3 минуты. Проверьте на роутере: keenway panel status'; goBtn.disabled = false; return; }
        setTimeout(wait, 3000);
      };
      setTimeout(wait, 5000);
    } catch (e) { status.textContent = '⚠️ ' + e.message; goBtn.disabled = false; }
  } }, `Обновить до ${v.latest}`);
  const modal = h('div', { class: 'modal', onclick: (e) => { if (e.target === modal) modal.remove(); } },
    h('div', { class: 'card stack' },
      h('h2', { style: 'margin:0' }, `Новая версия KeenWay ${v.latest}`),
      h('p', { class: 'muted' }, `Сейчас установлена ${v.current}. Настройки и конфиг роутера при обновлении не меняются.`),
      v.notes ? h('pre', { class: 'notes' }, v.notes) : null,
      v.url ? h('a', { href: v.url, target: '_blank', rel: 'noopener' }, 'Что нового на GitHub') : null,
      status,
      h('div', { class: 'row', style: 'justify-content:flex-end' }, h('button', { class: 'btn', onclick: () => modal.remove() }, 'Позже'), goBtn)));
  document.body.append(modal);
}
function versionCard() {
  const out = h('div', { class: 'row' });
  const render = () => {
    const v = S.version;
    out.replaceChildren(
      h('span', {}, `Версия ${v?.current || '?'}`),
      !v ? null : v.newer ? h('button', { class: 'btn primary sm', onclick: showUpdate }, `Обновить до ${v.latest}`)
        : v.error ? h('span', { class: 'muted small' }, `проверить обновления не удалось: ${v.error}`)
        : h('span', { class: 'pill good' }, 'последняя'),
      h('button', { class: 'btn sm', onclick: async () => { await checkVersion(); render(); } }, 'Проверить обновления'));
  };
  render();
  return h('div', { class: 'card stack' }, h('h2', { style: 'margin-top:0' }, 'KeenWay'), out,
    h('div', {}, h('button', { class: 'btn sm', onclick: showDonate }, '💜 Поддержать проект')));
}

// ---------- support ----------
const DONATE_CARD = '2202 2085 5093 6505';
function showDonate() {
  const copy = h('button', { class: 'btn primary', onclick: async () => {
    try { await navigator.clipboard.writeText(DONATE_CARD.replace(/ /g, '')); copy.textContent = 'Скопировано ✓'; } catch { toast('Не удалось скопировать — выделите номер вручную'); }
  } }, 'Скопировать номер');
  const modal = h('div', { class: 'modal', onclick: (e) => { if (e.target === modal) modal.remove(); } },
    h('div', { class: 'card stack donate-card' },
      h('h2', { style: 'margin:0' }, '💜 Поддержать KeenWay'),
      h('p', { class: 'muted' }, 'KeenWay бесплатный. Если он вам пригодился, можно поддержать разработку переводом на карту — любой суммой.'),
      h('div', { class: 'card-number' }, DONATE_CARD),
      h('div', { class: 'row', style: 'justify-content:flex-end' }, h('button', { class: 'btn', onclick: () => modal.remove() }, 'Закрыть'), copy),
      h('p', { class: 'muted small', style: 'margin:0' }, 'Спасибо!')));
  document.body.append(modal);
}

const APPLY_PLAN = [
  ['connect', 'Подключение к роутеру', 'Подключение к роутеру по SSH'],
  ['upload', 'Загрузка и проверка конфига', 'Отправка нового конфига на роутер и проверка в mihomo — работающий конфиг пока не меняется'],
  ['activate', 'Включение нового конфига', 'Сохранение копии прежнего конфига и переключение mihomo на новый'],
  ['health', 'Проверка интернета и VPN', 'Запуск mihomo и загрузка списков сайтов, проверка интернета и серверов VPN (обычно до минуты)'],
];
async function runApply(dry) {
  await flushSave();
  const plan = dry ? APPLY_PLAN.slice(0, 2) : APPLY_PLAN;
  const rows = {};
  const steps = h('ul', { class: 'steps' }, plan.map(([id, name]) => {
    const mark = h('span', { class: 'mark' }, '○');
    const info = h('div', { class: 'muted small' });
    rows[id] = { li: h('li', { class: 'pending' }, mark, h('span', {}, h('span', { class: 'name' }, name), info)), mark, info };
    return rows[id].li;
  }));
  const status = h('div', { class: 'muted small' });
  const setCurrent = (idx) => {
    const step = plan[idx];
    if (!step) return status.replaceChildren();
    const r = rows[step[0]];
    r.li.className = 'current';
    r.mark.replaceChildren(h('span', { class: 'spinner sm' }));
    status.replaceChildren(step[2] + '…');
  };
  const close = h('button', { class: 'btn', disabled: true, onclick: () => { modal.remove(); loadState().then(() => go(S.page)); } }, 'Закрыть');
  const cancelBtn = h('button', { class: 'btn danger', onclick: async () => {
    if (!dry && !confirm('Отменить применение? Если новый конфиг уже включён, роутер вернётся к прежним настройкам.')) return;
    cancelBtn.disabled = true;
    cancelBtn.textContent = 'Отмена…';
    status.replaceChildren(h('span', { class: 'spinner sm' }), ' Отмена и возврат прежних настроек…');
    try { await api('POST', '/apply/cancel'); } catch (e) { toast(e.message); }
  } }, 'Отменить');
  const modal = h('div', { class: 'modal' }, h('div', { class: 'card' },
    h('h2', { style: 'margin-top:0' }, dry ? 'Проверка конфига' : 'Применение настроек'),
    h('p', { class: 'muted small' }, dry ? 'Роутер проверит новый конфиг, но работать продолжит на текущем.' : 'Если что-то пойдёт не так, роутер сам вернётся к прежним настройкам.'),
    steps, status, h('div', { class: 'row', style: 'justify-content:flex-end;margin-top:12px' }, cancelBtn, close)));
  document.body.append(modal);
  setCurrent(0);
  try {
    const res = await fetch('/api/apply' + (dry ? '?dry=1' : ''), { method: 'POST', headers: { 'X-KeenWay': token } });
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = '';
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      let i;
      while ((i = buf.indexOf('\n\n')) >= 0) {
        const chunk = buf.slice(0, i);
        buf = buf.slice(i + 2);
        const ev = /event: (\w+)/.exec(chunk)?.[1];
        const data = JSON.parse(/data: (.*)/.exec(chunk)?.[1] || '{}');
        if (ev === 'step' && data.id !== 'done') {
          let r = rows[data.id];
          if (!r) { // steps outside the plan, e.g. rolling back
            const mark = h('span', { class: 'mark' });
            const info = h('div', { class: 'muted small' });
            r = rows[data.id] = { li: h('li', {}, mark, h('span', {}, h('span', { class: 'name' }, data.name), info)), mark, info };
            steps.append(r.li);
          }
          r.li.className = data.ok ? 'ok' : 'fail';
          r.mark.replaceChildren(data.ok ? '✅' : '❌');
          r.info.replaceChildren(data.info || '');
          const idx = plan.findIndex(([id]) => id === data.id);
          if (data.ok && idx >= 0) setCurrent(idx + 1); else status.replaceChildren();
        }
        if (ev === 'done') {
          for (const r of Object.values(rows)) if (r.li.className === 'current' || r.li.className === 'pending') { r.li.className = 'skipped'; r.mark.replaceChildren('–'); }
          let box;
          cancelBtn.hidden = true;
          if (data.canceled) {
            box = h('div', { class: 'alert warn' }, '↩️', h('span', { class: 'grow' }, 'Применение отменено. Роутер работает на прежних настройках.'));
          } else if (data.ok) {
            const warned = Object.values(rows).some((r) => r.info.textContent.startsWith('⚠️'));
            box = h('div', { class: `alert ${warned ? 'warn' : 'good'}` }, warned ? '⚠️' : '🎉', h('span', { class: 'grow' }, dry ? 'Конфиг в порядке — роутер его принял. Можно применять.' : warned ? 'Настройки применены, но серверы VPN пока не отвечают.' : 'Настройки применены.'));
            if (!dry && !warned) setTimeout(() => { if (modal.isConnected) { modal.remove(); toast('✅ Настройки применены'); loadState().then(() => go(S.page)); } }, 1500);
          } else {
            box = h('div', { class: 'alert bad' }, '⛔', h('div', { class: 'grow' },
              h('strong', {}, data.error),
              data.untouched ? h('div', { class: 'small', style: 'margin-top:4px' }, 'На роутере ничего не изменилось — он работает на прежних настройках.') : null,
              data.details ? h('details', { class: 'help', style: 'margin-top:6px' }, h('summary', {}, 'Подробности для опытных'), h('pre', { class: 'yaml', style: 'white-space:pre-wrap' }, data.details)) : null));
          }
          status.replaceChildren(box);
        }
      }
    }
  } catch (e) {
    status.replaceChildren(h('div', { class: 'alert bad' }, '⛔', h('span', { class: 'grow' }, e.message)));
  }
  cancelBtn.hidden = true;
  close.disabled = false;
}
document.getElementById('applyBtn').onclick = () => runApply(false);
document.getElementById('updateBtn').onclick = showUpdate;
document.getElementById('donateBtn').onclick = (e) => { e.preventDefault(); showDonate(); };
document.getElementById('dryBtn').onclick = () => runApply(true);
document.getElementById('previewBtn').onclick = async () => { await flushSave(); showPreview(); };

// ---------- start ----------
setInterval(() => api('POST', '/ping').catch(() => {}), 10000);
api('POST', '/ping').catch(() => {});
loadState().then(() => { checkVersion(); const want = location.hash.slice(1); if (want === 'donate') showDonate(); go(S.state.hasPassword ? (pages[want] ? want : 'overview') : 'settings'); }).catch((e) => mount(h('div', { class: 'alert bad' }, e.message)));
