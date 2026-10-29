// Behaviour of the RePlan pages' script in a real browser, against the
// throwaway OpenProject held by
//   REPLAN_OPENPROJECT_RUNTIME_PORT=18765 REPLAN_OPENPROJECT_RUNTIME_HOLD=1800 \
//     scripts/check-openproject-plugin-throwaway-runtime.sh
// Not part of CI: it needs Playwright and a Chromium.
//   PLAYWRIGHT=/path/to/node_modules/playwright CHROME=/path/to/chrome \
//     node test/browser/navigation_check.cjs
// It checks that OpenProject runs the script (its nonce), folding and its
// memory, the time slider, jumps, the load profile's waterline, full screen
// and the task tabs.
const { chromium } = require(process.env.PLAYWRIGHT || 'playwright');
const BASE = process.env.BASE || 'http://localhost:18765';
const PROJECT = process.env.PROJECT || 'replan-permission-check';
const G = `${BASE}/projects/${PROJECT}/replan_overlay/gantt`;
const results = []; const ok = (name, cond, extra) => results.push([cond ? 'ok  ' : 'FAIL', name, extra ?? '']);
(async () => {
  const b = await chromium.launch({ executablePath: process.env.CHROME || undefined, args: ['--no-proxy-server'] });
  const ctx = await b.newContext({ viewport: { width: 1400, height: 900 }, locale: 'en-US' });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', e => { if (!/HALError/.test(String(e))) errors.push(String(e)); });
  // OpenProject's own console noise (its connect-src entry, its API before
  // sign-in) is not the plugin's; a refused inline script would be.
  p.on('console', m => { if (m.type() === 'error' && /Refused to (execute|apply)/i.test(m.text())) errors.push(m.text()); });
  await p.goto(BASE + '/login');
  await Promise.all([p.waitForNavigation(), p.evaluate(() => { const u = document.querySelector('#username'); u.value = 'replan-member-check'; u.form.querySelector('#password').value = 'Replan-member-check-2026'; u.form.submit(); })]);
  await p.goto(G);
  ok('script runs (data-js set)', await p.evaluate(() => !!document.querySelector('[data-replan-gantt-root]').dataset.js));
  ok('no CSP or script errors', errors.length === 0, errors.join(' | ').slice(0, 300));
  ok('time slider off by default', await p.evaluate(() => document.querySelector('[data-replan-pan]').hidden));
  const h0 = await p.evaluate(() => document.querySelector('[data-replan-gantt]').clientHeight);
  await p.click('[data-replan-toggle="controls"]');
  await p.click('[data-replan-toggle="header"]');
  await p.waitForTimeout(200);
  const h1 = await p.evaluate(() => document.querySelector('[data-replan-gantt]').clientHeight);
  ok('folding controls and header gives the chart room', h1 > h0 + 100, `${h0} -> ${h1}`);
  ok('page header hidden', await p.evaluate(() => getComputedStyle(document.querySelector('.PageHeader')).display === 'none'));
    await p.reload();
  ok('folded state remembered after reload', await p.evaluate(() => document.querySelector('[data-replan-controls]').hidden && document.body.classList.contains('replan-compact')));
  await p.click('[data-replan-toggle="controls"]'); await p.click('[data-replan-toggle="header"]');
  await p.check('[data-replan-toggle="slider"]');
  ok('slider shown when toggled', await p.evaluate(() => !document.querySelector('[data-replan-pan]').hidden));
  await p.evaluate(() => { const r = document.querySelector('[data-replan-pan] input'); r.value = 1000; r.dispatchEvent(new Event('input')); });
  ok('slider moves to the far right', await p.evaluate(() => { const s = document.querySelector('[data-replan-gantt]'); return Math.abs(s.scrollLeft - (s.scrollWidth - s.clientWidth)) < 2; }));
  await p.uncheck('[data-replan-toggle="slider"]');
  await p.click('[data-replan-jump="start"]');
  ok('jump to range start', await p.evaluate(() => document.querySelector('[data-replan-gantt]').scrollLeft === 0));
  await p.click('[data-replan-jump="status"]');
  const st = await p.evaluate(() => { const s = document.querySelector('[data-replan-gantt]'); const line = document.querySelector('.replan-gantt__axis .replan-gantt__status').getBoundingClientRect().left; const r = s.getBoundingClientRect(); return { inView: line > r.left + 250 && line < r.right, scrollLeft: s.scrollLeft }; });
  ok('jump to status date shows the status line', st.inView, JSON.stringify(st));
  await p.evaluate(() => { document.querySelector('[data-replan-gantt]').scrollTop = 500; });
  await p.click('[data-replan-jump="top"]');
  ok('jump to top', await p.evaluate(() => document.querySelector('[data-replan-gantt]').scrollTop === 0));
  await p.click('[data-replan-jump="load"]');
  const load = await p.evaluate(() => { const s = document.querySelector('[data-replan-gantt]'); const g = document.querySelector('[data-replan-load]').getBoundingClientRect(); const r = s.getBoundingClientRect(); return { top: Math.round(g.top - r.top), scrollTop: s.scrollTop }; });
  ok('jump to the load profile puts it under the axis', load.scrollTop > 0 && load.top >= 40 && load.top <= 50, JSON.stringify(load));
  const water = await p.evaluate(() => { const row = document.querySelector('.replan-gantt__load-row .replan-gantt__track').getBoundingClientRect(); const w = document.querySelector('.replan-gantt__waterline').getBoundingClientRect(); return Math.round((row.bottom - 4 - w.top) / (row.height - 8) * 100); });
  ok('waterline at 55% of the load row', Math.abs(water - 55) <= 2, `${water}%`);
  const hump = await p.evaluate(() => { const h = document.querySelector('.replan-gantt__week-hump.-over'); const w = h.closest('.replan-gantt__load-row').querySelector('.replan-gantt__waterline').getBoundingClientRect(); return Math.round(h.getBoundingClientRect().bottom - w.top); });
  ok('a red hump starts at the waterline', Math.abs(hump) <= 1, `${hump}px`);
  await p.click('[data-replan-toggle="fullscreen"]');
  await p.waitForTimeout(400);
  const full = await p.evaluate(() => { const r = document.querySelector('[data-replan-gantt-root]'); return document.fullscreenElement === r || r.classList.contains('-maximized'); });
  ok('full screen or maximized', full);
  await p.keyboard.press('Escape'); await p.waitForTimeout(400);
  if (await p.evaluate(() => !!document.fullscreenElement)) await p.evaluate(() => document.exitFullscreen());
  await p.waitForTimeout(300);
  ok('Esc leaves full screen', await p.evaluate(() => { const r = document.querySelector('[data-replan-gantt-root]'); return !document.fullscreenElement && !r.classList.contains('-maximized'); }));

  // Tabs: open six task pages from the Gantt.
  const links = await p.evaluate(() => [...new Set([...document.querySelectorAll('.replan-gantt__name a')].map(a => a.getAttribute('href')))].slice(0, 6));
  for (const href of links) {
    await p.goto(G);
    await Promise.all([p.waitForURL('**' + href), p.evaluate(h => document.querySelector(`.replan-gantt__name a[href="${h}"]`).click(), href)]);
    await p.waitForLoadState('load'); await p.waitForTimeout(300);
  }
  const tabs = await p.evaluate(() => [...document.querySelectorAll('.replan-nav__tab')].map(t => t.querySelector('a').getAttribute('href')));
  ok('at most five task tabs', tabs.length === 5, tabs.length);
  ok('the least recently used tab was dropped', !tabs.includes(links[0]) && tabs.includes(links[5]));
  ok('current task tab marked', await p.evaluate(() => !!document.querySelector('.replan-nav__tab[aria-current="page"]')));
  await p.goto(G);
  ok('tabs shown on the Gantt too', (await p.evaluate(() => document.querySelectorAll('.replan-nav__tab').length)) === 5);
  await Promise.all([p.waitForURL('**' + links[5]), p.evaluate(h => document.querySelector(`.replan-nav__tab a[href="${h}"]`).click(), links[5])]);
  await p.waitForTimeout(300);
  await Promise.all([p.waitForURL(u => !String(u).includes('/work_packages/')), p.click('.replan-nav__tab[aria-current="page"] .replan-nav__close')]);
  ok('closing the current tab returns to where it was opened from', p.url().startsWith(G), p.url());
  ok('the closed tab is gone', (await p.evaluate(() => document.querySelectorAll('.replan-nav__tab').length)) === 4);
  // An unknown work package opens no tab and drops one remembered for it.
  await p.evaluate(k => { const t = JSON.parse(localStorage.getItem(k) || '[]'); t.push({ wp: '999999999', title: 'gone', href: '/x', from: null, used: 1 }); localStorage.setItem(k, JSON.stringify(t)); }, `replan.tabs.${PROJECT}`);
  await p.goto(`${BASE}/projects/${PROJECT}/replan_overlay/work_packages/999999999`);
  ok('unknown work package opens no tab', await p.evaluate(() => !document.querySelector('.replan-nav__tab[aria-current="page"]')));
  ok('a tab remembered for it is dropped', await p.evaluate(k => !JSON.parse(localStorage.getItem(k) || '[]').some(t => t.wp === '999999999'), `replan.tabs.${PROJECT}`));
  ok('the other tabs stay', (await p.evaluate(() => document.querySelectorAll('.replan-nav__tab').length)) === 4);
  ok('no script errors at the end', errors.length === 0, errors.join(' | ').slice(0, 300));
  await b.close();
  for (const r of results) console.log(r.join(' '));
  process.exit(results.some(r => r[0] === 'FAIL') ? 1 : 0);
})().catch(e => { console.error(e); process.exit(2); });
