'use strict';
// Servers on a world map (Servers → Map). Needs d3 and topojson-client (web/vendor, loaded by app.js).
// A server's country comes from the flag in its name; a click on a country connects to its fastest server.
(function () {
  // ISO 3166 alpha-2 → [numeric id used by world-atlas, latitude, longitude of a typical hosting city]
  const C = {
    DE: [276, 50.11, 8.68], NL: [528, 52.37, 4.9], SE: [752, 59.33, 18.07], FI: [246, 60.17, 24.94], PL: [616, 52.23, 21.01],
    FR: [250, 48.86, 2.35], GB: [826, 51.51, -0.13], US: [840, 40.71, -74.01], CA: [124, 43.65, -79.38], JP: [392, 35.68, 139.69],
    KR: [410, 37.57, 126.98], SG: [702, 1.35, 103.82], HK: [344, 22.32, 114.17], TR: [792, 41.01, 28.98], AM: [51, 40.18, 44.51],
    KZ: [398, 43.24, 76.89], MD: [498, 47.01, 28.86], LV: [428, 56.95, 24.11], LT: [440, 54.69, 25.28], EE: [233, 59.44, 24.75],
    RU: [643, 55.76, 37.62], UA: [804, 50.45, 30.52], BY: [112, 53.9, 27.56], GE: [268, 41.72, 44.79], AZ: [31, 40.41, 49.87],
    AT: [40, 48.21, 16.37], CH: [756, 47.38, 8.54], IT: [380, 45.46, 9.19], ES: [724, 40.42, -3.7], PT: [620, 38.72, -9.14],
    CZ: [203, 50.08, 14.44], SK: [703, 48.15, 17.11], HU: [348, 47.5, 19.04], RO: [642, 44.43, 26.1], BG: [100, 42.7, 23.32],
    RS: [688, 44.79, 20.45], GR: [300, 37.98, 23.73], NO: [578, 59.91, 10.75], DK: [208, 55.68, 12.57], IE: [372, 53.35, -6.26],
    BE: [56, 50.85, 4.35], LU: [442, 49.61, 6.13], IS: [352, 64.15, -21.94], IL: [376, 32.09, 34.78], AE: [784, 25.2, 55.27],
    IN: [356, 19.08, 72.88], CN: [156, 31.23, 121.47], TW: [158, 25.03, 121.57], AU: [36, -33.87, 151.21], BR: [76, -23.55, -46.63],
    AR: [32, -34.6, -58.38], MX: [484, 19.43, -99.13], ZA: [710, -26.2, 28.05], EG: [818, 30.04, 31.24], CY: [196, 35.19, 33.38],
    HR: [191, 45.81, 15.98], SI: [705, 46.06, 14.51], UZ: [860, 41.3, 69.24], KG: [417, 42.87, 74.59], TH: [764, 13.76, 100.5],
    VN: [704, 10.82, 106.63], ID: [360, -6.21, 106.85], MY: [458, 3.14, 101.69], PH: [608, 14.6, 120.98], NZ: [554, -36.85, 174.76],
  };
  const names = (() => { try { return new Intl.DisplayNames(['ru'], { type: 'region' }); } catch { return null; } })();
  const nameOf = (iso) => (names ? names.of(iso) : iso);
  const isoOf = (flag) => [...flag].map((ch) => String.fromCharCode(ch.codePointAt(0) - 0x1f1e6 + 65)).join('');
  const flagOf = (n) => (n.match(/\p{Regional_Indicator}{2}/u) || [])[0];
  const css = (v) => getComputedStyle(document.documentElement).getPropertyValue(v).trim();
  const HOME_KEY = 'kw-map-home';
  const loadHome = () => { try { return JSON.parse(localStorage.getItem(HOME_KEY) || 'null'); } catch { return null; } };
  const saveHome = (h) => { try { localStorage.setItem(HOME_KEY, JSON.stringify(h)); } catch { /* storage blocked */ } };
  let world = null;

  // countries: group servers by flag → { iso, flag, name, lat, lon, nodes, best, bestDelay, excluded }
  function countries(nodes, delays, isExcluded) {
    const by = {};
    for (const n of nodes) {
      const f = flagOf(n);
      if (!f) continue;
      const iso = isoOf(f);
      if (!C[iso]) continue;
      const c = by[iso] || (by[iso] = { iso, flag: f, name: nameOf(iso), id: C[iso][0], lat: C[iso][1], lon: C[iso][2], nodes: [] });
      c.nodes.push(n);
    }
    for (const c of Object.values(by)) {
      const live = c.nodes.filter((n) => delays[n] > 0).sort((a, b) => delays[a] - delays[b]);
      c.best = live[0] || c.nodes[0];
      c.bestDelay = live.length ? delays[live[0]] : 0;
      c.excluded = c.nodes.every(isExcluded);
    }
    // countries in the auto-choice first, each part by best ping
    return Object.values(by).sort((a, b) => (a.excluded - b.excluded) || ((a.bestDelay || 1e9) - (b.bestDelay || 1e9)));
  }

  async function render(el, { nodes, delays, current, isExcluded, onPick }) {
    if (!world) world = await (await fetch('vendor/countries-110m.json')).json();
    const list = countries(nodes, delays, isExcluded);
    const curFlag = current && flagOf(current);
    const curIso = curFlag && isoOf(curFlag);
    const skipped = nodes.filter((n) => !flagOf(n) || !C[isoOf(flagOf(n))]).length;
    const color = { good: css('--good'), warn: css('--warn'), bad: css('--bad'), muted: css('--muted'), accent: css('--accent') };
    const dclass = (d) => (!d ? 'muted' : d < 300 ? 'good' : d < 800 ? 'warn' : 'bad');

    el.replaceChildren();
    el.className = 'map-wrap';
    const svgEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    el.append(svgEl);
    const side = document.createElement('div');
    side.className = 'map-side';
    const tools = document.createElement('div');
    tools.className = 'map-tools';
    el.append(side, tools);

    const W = el.clientWidth || 900, H = el.clientHeight || 560;
    const svg = d3.select(svgEl).attr('viewBox', `0 0 ${W} ${H}`);
    const g = svg.append('g');
    const proj = d3.geoMercator();
    const path = d3.geoPath(proj);
    // start zoomed to the fast servers (usually Europe); ⌂ shows them all
    const fast = list.filter((c) => !c.excluded && c.bestDelay > 0 && c.bestDelay < 150);
    const focus = fast.length >= 2 ? fast : list;
    const pts = focus.length ? focus.map((c) => [c.lon, c.lat]) : [[-10, 35], [50, 65]];
    const lons = pts.map((p) => p[0]), lats = pts.map((p) => p[1]);
    const box = { type: 'MultiPoint', coordinates: [[Math.min(...lons) - 8, Math.min(...lats) - 6], [Math.max(...lons) + 8, Math.max(...lats) + 6]] };
    const left = W > 760 ? 300 : 16;
    proj.fitExtent([[left, 24], [W - 24, H - 24]], box);

    const has = new Set(list.map((c) => c.id));
    g.append('path').attr('class', 'map-grat').attr('d', path(d3.geoGraticule10()));
    g.selectAll('.map-land').data(topojson.feature(world, world.objects.countries).features).join('path')
      .attr('class', (d) => 'map-land' + (has.has(+d.id) ? ' has' : '') + (curIso && C[curIso] && +d.id === C[curIso][0] ? ' cur' : '')).attr('d', path);
    const route = g.append('path').attr('class', 'map-route');
    const home = loadHome();
    const homeG = g.append('g').attr('class', 'map-home');

    const drawRoute = (c, going) => {
      if (!home || !c) { route.attr('d', null); return; }
      route.datum({ type: 'LineString', coordinates: [home, [c.lon, c.lat]] }).attr('d', path).classed('going', !!going);
    };
    const drawHome = () => {
      homeG.selectAll('*').remove();
      if (!home) return;
      const [x, y] = proj(home);
      homeG.attr('transform', `translate(${x},${y})`);
      homeG.append('circle').attr('r', 6).attr('class', 'dot');
      homeG.append('text').attr('x', 10).attr('y', 4).text('🏠 Дом');
    };

    const pin = g.selectAll('.map-pin').data(list).join('g')
      .attr('class', (c) => `map-pin ${c.iso === curIso ? 'on' : ''} ${c.excluded ? 'off' : ''}`)
      .attr('transform', (c) => { const [x, y] = proj([c.lon, c.lat]); return `translate(${x},${y})`; });
    pin.append('circle').attr('class', 'halo').attr('r', 9);
    pin.append('circle').attr('class', 'dot').attr('r', 8).attr('fill', (c) => (c.iso === curIso ? color.accent : color[dclass(c.bestDelay)]));
    pin.append('text').attr('class', 'cnt').attr('y', 3.5).attr('text-anchor', 'middle').text((c) => c.nodes.length);
    pin.append('title').text((c) => `${c.flag} ${c.name}: ${c.nodes.length} серв., лучший ${c.bestDelay ? c.bestDelay + ' мс' : 'без ответа'}${c.excluded ? ' · не в авто-выборе' : ''}`);
    pin.append('text').attr('class', 'lbl').attr('x', 13).attr('y', 4).text((c) => `${c.flag} ${c.name}${c.bestDelay ? ' · ' + c.bestDelay + ' мс' : ''}`);

    const connect = async (c) => {
      pin.classed('on', (x) => x === c).select('.dot').attr('fill', (x) => (x === c ? color.accent : color[dclass(x.bestDelay)]));
      drawRoute(c, true);
      await onPick(c.best);
    };
    pin.on('click', (e, c) => { e.stopPropagation(); connect(c); });

    // home: "📍" then a click on the map
    let placing = false;
    const homeBtn = document.createElement('button');
    homeBtn.className = 'btn sm';
    homeBtn.textContent = home ? '📍 Сменить дом' : '📍 Отметить дом';
    homeBtn.title = 'Нажмите, затем кликните по карте там, где вы живёте — будет видна линия до сервера';
    homeBtn.onclick = () => { placing = !placing; el.classList.toggle('placing', placing); homeBtn.textContent = placing ? 'Кликните по карте…' : (loadHome() ? '📍 Сменить дом' : '📍 Отметить дом'); };
    svg.on('click', (e) => {
      if (!placing) return;
      const t = d3.zoomTransform(svgEl);
      const [mx, my] = d3.pointer(e, svgEl);
      const ll = proj.invert(t.invert([mx, my]));
      if (!ll) return;
      saveHome(ll);
      location.reload();
    });
    const zbtn = (txt, title, fn) => { const b = document.createElement('button'); b.className = 'btn sm'; b.textContent = txt; b.title = title; b.onclick = fn; return b; };
    const zoom = d3.zoom().scaleExtent([0.6, 14]).on('zoom', (ev) => {
      g.attr('transform', ev.transform);
      pin.attr('transform', (c) => { const [x, y] = proj([c.lon, c.lat]); return `translate(${x},${y}) scale(${1 / ev.transform.k})`; });
      if (home) { const [x, y] = proj(home); homeG.attr('transform', `translate(${x},${y}) scale(${1 / ev.transform.k})`); }
      route.style('stroke-width', 2 / ev.transform.k);
    });
    svg.call(zoom);
    tools.append(homeBtn, zbtn('+', 'Приблизить', () => svg.transition().call(zoom.scaleBy, 1.6)), zbtn('−', 'Отдалить', () => svg.transition().call(zoom.scaleBy, 1 / 1.6)), zbtn('⌂', 'Показать все серверы', () => svg.transition().call(zoom.transform, d3.zoomIdentity)));

    // side list: countries by best ping
    const curC = list.find((c) => c.iso === curIso);
    side.innerHTML = '';
    const head = document.createElement('div');
    head.className = 'map-now';
    head.innerHTML = `<span class="muted small">Сейчас</span><b></b>`;
    head.querySelector('b').textContent = current || '—';
    side.append(head);
    const hint = document.createElement('div');
    hint.className = 'muted small';
    hint.textContent = 'Нажмите на страну на карте или в списке — подключу к её самому быстрому серверу.';
    side.append(hint);
    for (const c of list) {
      const row = document.createElement('button');
      row.className = `map-row ${c.iso === curIso ? 'on' : ''} ${c.excluded ? 'off' : ''}`;
      row.title = c.excluded ? 'Не участвует в авто-выборе, но можно выбрать вручную' : '';
      const d = document.createElement('span');
      d.className = `delay ${dclass(c.bestDelay) === 'muted' ? 'bad' : dclass(c.bestDelay)}`;
      d.textContent = c.bestDelay ? `${c.bestDelay} мс` : '—';
      row.append(`${c.flag} ${c.name} `, Object.assign(document.createElement('small'), { textContent: `×${c.nodes.length}` }), d);
      row.onclick = () => connect(c);
      side.append(row);
    }
    if (skipped) {
      const s = document.createElement('div');
      s.className = 'muted small';
      s.textContent = `Ещё ${skipped} серв. без флага страны — они только в списке.`;
      side.append(s);
    }
    drawHome();
    drawRoute(curC, false);
  }

  window.KWMap = { render };
})();
