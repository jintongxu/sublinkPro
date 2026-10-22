import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('subscription policy default, edit, save and preview are wired', () => {
  const source = read('./index.jsx');
  assert.equal((source.match(/ValidationPolicy: 'all'/g) || []).length, 2);
  assert.match(source, /ValidationPolicy: sub.ValidationPolicy \|\| 'all'/);
  assert.equal((source.match(/ValidationPolicy: formData.ValidationPolicy \|\| 'all'/g) || []).length, 2);
  const api = read('../../api/subscriptions.js');
  assert.equal((api.match(/formData.append\(key, data\[key\]\)/g) || []).length, 2);
});

test('form has exact policy enum and both languages document structural semantics', () => {
  const form = read('./component/SubscriptionFormDialog.jsx');
  assert.match(form, /\['all', 'exclude_failed', 'only_success'\]/);
  assert.match(form, /ValidationPolicy: e.target.value/);
  for (const locale of ['en-US', 'zh-CN']) {
    const data = JSON.parse(read(`../../i18n/locales/${locale}.json`));
    const policy = data.subscriptions.form.nodeFilter;
    assert.deepEqual(Object.keys(policy.validationPolicyOptions), ['all', 'exclude_failed', 'only_success']);
    assert.ok(policy.validationPolicyHelper);
    assert.ok(data.subscriptions.preview.validationExcluded);
  }
});

test('preview displays separate policy and dependency exclusions', () => {
  const source = read('./component/NodePreviewDialog.jsx');
  assert.match(source, /data.ValidationExcludedCount/);
  assert.match(source, /data.ValidationDependencyExcludedCount/);
  assert.match(source, /subscriptions.preview.validationExcluded/);
});
