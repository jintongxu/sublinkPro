import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolveSubscriptionServerUrl, selectQuickCopyShare, buildAutoSubscriptionUrl } from './subscriptionLink.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const origin = 'https://ui.example';

test('configured subscription domain takes precedence and normalizes trailing slashes', () => {
  assert.equal(resolveSubscriptionServerUrl({ systemDomain: ' https://sub.example/base/// ', origin, development: true, apiUrl: 'http://localhost:8000' }), 'https://sub.example/base');
  assert.equal(resolveSubscriptionServerUrl({ systemDomain: 'sub.example/', origin }), 'http://sub.example');
});

test('production uses page origin rather than build-time backend URL', () => {
  assert.equal(resolveSubscriptionServerUrl({ origin, apiUrl: 'http://localhost:8000' }), origin);
});

test('development substitutes page hostname for loopback backend while preserving port', () => {
  for (const host of ['localhost', '127.0.0.1', '[::1]']) {
    assert.equal(resolveSubscriptionServerUrl({ origin, development: true, apiUrl: `http://${host}:19001/api`, hostname: '192.0.2.10' }), 'http://192.0.2.10:19001');
  }
  assert.equal(resolveSubscriptionServerUrl({ origin, development: true, apiUrl: 'https://api.example/api', hostname: 'ui.example' }), 'https://api.example');
  assert.equal(resolveSubscriptionServerUrl({ origin, development: true, apiUrl: 'http://[' }), origin);
  assert.equal(resolveSubscriptionServerUrl({ origin, development: true }), origin);
});

test('quick copy prefers an available default share without mutating the list', () => {
  const shares = [{ token: 'custom', enabled: true }, { token: 'default', enabled: true, is_legacy: true }];
  assert.equal(selectQuickCopyShare(shares), shares[1]);
  assert.equal(shares[0].token, 'custom');
});

test('quick copy excludes disabled, missing-token, expired and invalid-date shares', () => {
  const now = new Date('2026-10-22T12:00:00Z');
  const fallback = { token: 'available', enabled: true };
  const shares = [
    { token: 'disabled', enabled: false, is_legacy: true },
    { enabled: true },
    { token: 'expired-days', enabled: true, expire_type: 1, expire_days: 1, created_at: '2026-10-20T12:00:00Z' },
    { token: 'expired-date', enabled: true, expire_type: 2, expire_at: '2026-10-21T12:00:00Z' },
    { token: 'invalid', enabled: true, expire_type: 2, expire_at: 'invalid' },
    fallback
  ];
  assert.equal(selectQuickCopyShare(shares, now), fallback);
  assert.equal(selectQuickCopyShare(shares.slice(0, -1), now), null);
  assert.equal(selectQuickCopyShare([], now), null);
});

test('expiration boundary matches server After semantics and zero days never expires', () => {
  const now = new Date('2026-10-22T12:00:00Z');
  for (const policy of [
    { expire_type: 1, expire_days: 1, created_at: '2026-10-21T12:00:00Z' },
    { expire_type: 2, expire_at: now.toISOString() },
    { expire_type: 1, expire_days: 0 }
  ]) {
    const share = { token: 'boundary', enabled: true, ...policy };
    assert.equal(selectQuickCopyShare([share], now), share);
  }
});

test('auto URL encodes token, stays at consume root and has no explicit client', () => {
  const url = new URL(buildAutoSubscriptionUrl('https://sub.example///', 'test +&/?#'));
  assert.equal(url.pathname, '/c/');
  assert.equal(url.searchParams.get('token'), 'test +&/?#');
  assert.deepEqual([...url.searchParams.keys()], ['token']);
});

test('desktop and mobile wire quick copy before preview with accessible loading state', () => {
  const source = read('./index.jsx');
  assert.equal((source.match(/onCopySubscriptionLink=\{handleCopySubscriptionLink\}/g) || []).length, 2);
  const handler = source.slice(source.indexOf('const handleCopySubscriptionLink'), source.indexOf('const handleClient'));
  assert.match(handler, /Promise\.all\(\[getShares\(sub.ID\), getSystemDomain\(\)\]\)/);
  assert.match(handler, /if \(!share\)/);
  assert.doesNotMatch(handler, /addShare|console\./);
  assert.match(read('../../api/settings.js'), /export function getSystemDomain/);
  for (const file of ['SubscriptionTable', 'SubscriptionMobileCard']) {
    const component = read(`./component/${file}.jsx`);
    assert.ok(component.indexOf('onCopySubscriptionLink(sub)') < component.indexOf('onPreview(sub)'));
    assert.match(component, /aria-label=\{t\('subscriptions.table.tooltips.copyAutoLink'\)\}/);
    assert.match(component, /disabled=\{quickCopyingSubId !== null\}/);
  }
  for (const locale of ['en-US', 'zh-CN']) {
    const data = JSON.parse(read(`../../i18n/locales/${locale}.json`));
    assert.ok(data.subscriptions.table.tooltips.copyAutoLink);
    assert.ok(data.subscriptions.page.messages.noAvailableShare);
    assert.ok(data.subscriptions.page.messages.copyLinkFailed);
  }
});
