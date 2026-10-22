import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const source = readFileSync(new URL('./component/SubscriptionFormDialog.jsx', import.meta.url), 'utf8');

test('enabling custom groups only changes the enabled flag and preserves assignments', () => {
  const toggle = source.match(/checked=\{Boolean\(formData\.enableNodeTemplateAssignments\)\}([\s\S]*?)label=\{formData\.enableNodeTemplateAssignments/);
  assert.ok(toggle, 'custom assignment toggle exists');
  assert.match(toggle[1], /\.\.\.prev,/);
  assert.match(toggle[1], /enableNodeTemplateAssignments: event\.target\.checked/);
  assert.doesNotMatch(toggle[1], /nodeTemplateAssignments\s*:/);
  assert.doesNotMatch(source, /assignNewNodesToAllGroups/);
});

test('explicit group choices still update membership and helper text is bilingual', () => {
  assert.ok(source.includes('if (selected.has(Number(node.ID))) withoutCurrentGroup.push(assignmentMenuGroup);'));
  assert.match(source, /t\('subscriptions\.form\.manualGroupAssignmentHelper'\)/);
  assert.doesNotMatch(source, /新添加节点默认加入全部代理组/);
  for (const locale of ['en-US', 'zh-CN']) {
    const data = JSON.parse(readFileSync(new URL(`../../i18n/locales/${locale}.json`, import.meta.url), 'utf8'));
    assert.ok(data.subscriptions.form.manualGroupAssignmentHelper);
  }
});
