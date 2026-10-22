import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { getValidationDisplay } from './validationDisplay.mjs';

const t = (key, values) => values ? `${key}: ${values.time}` : key;

test('all validation statuses share safe mobile/desktop display', () => {
  for (const status of ['untested', 'success', 'partial', 'failed', 'disabled']) {
    const result = getValidationDisplay({ ValidationStatus: status, ValidationReason: 'validated' }, t);
    assert.equal(result.status, status);
    assert.equal(result.label, `nodes.table.validationStatus.${status}`);
    assert.equal(result.detail, 'validated');
  }
  assert.equal(getValidationDisplay({ ValidationStatus: 'unknown' }, t).status, 'untested');
});

test('reasons redact every credential-bearing URL scheme and bound output', () => {
  const result = getValidationDisplay({
    ValidationReason: `trojan://secret@example.com:443#name https://host/sub?token=secret password=hidden\n${'a'.repeat(300)}`,
    ValidationCheckAt: 'invalid'
  }, t);
  assert.doesNotMatch(result.detail, /secret|hidden|example|token=|\n/);
  assert.ok(result.detail.length <= 160);
  assert.match(result.detail, /redacted/);
});

test('page wires status to fetching, snapshots, select-all and terminal refresh', () => {
  const source = readFileSync(new URL('./index.jsx', import.meta.url), 'utf8');
  assert.match(source, /filterParams.validationStatus/);
  assert.ok((source.match(/filters.validationStatus/g) || []).length >= 2);
  assert.ok((source.match(/validationStatus: validationStatus/g) || []).length >= 2);
  assert.match(source, /setValidationStatus\(''\)/);
  assert.match(source, /\['completed', 'cancelled', 'error'\]/);
  assert.match(source, /taskType === 'node_revalidate'/);
  assert.match(source, /all \? \{ all: true, filters \} : \{ ids \}/);
  assert.match(source, /nodes.validation.confirm.*count/);
  for (const file of ['NodeTable.jsx', 'NodeCard.jsx']) {
    const component = readFileSync(new URL(`./component/${file}`, import.meta.url), 'utf8');
    assert.match(component, /import \{ getValidationDisplay \} from '..\/validationDisplay.mjs'/);
  }
});

test('validation locale keys exist in both supported languages', () => {
  for (const locale of ['en-US', 'zh-CN']) {
    const language = JSON.parse(readFileSync(new URL(`../../i18n/locales/${locale}.json`, import.meta.url), 'utf8'));
    for (const key of ['title', 'selected', 'all', 'confirm', 'empty', 'failed', 'started']) {
      assert.equal(typeof language.nodes.validation[key], 'string');
    }
  }
});
