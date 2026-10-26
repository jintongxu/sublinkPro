import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const source = readFileSync(new URL('./component/SubscriptionFormDialog.jsx', import.meta.url), 'utf8');

test('enabling custom groups only changes the enabled flag and preserves assignments', () => {
  const toggle = source.match(
    /checked=\{Boolean\(formData\.enableNodeTemplateAssignments\)\}([\s\S]*?)label=\{formData\.enableNodeTemplateAssignments/
  );
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

test('assignment candidates include expanded front dependencies', () => {
  const indexSource = readFileSync(new URL('./index.jsx', import.meta.url), 'utf8');
  assert.match(indexSource, /const assignmentNodesList = useMemo\(/, 'index.jsx derives assignment candidates from all selected node ids');
  assert.match(indexSource, /formData\.selectedNodes \|\| \[\]/, 'assignment candidates include expanded dependency ids');
  assert.match(indexSource, /assignmentNodesList=\{assignmentNodesList\}/, 'assignment candidates are passed to the dialog');

  assert.match(source, /assignmentNodesList,/, 'dialog accepts the assignment candidate list');
  assert.match(
    source,
    /assignmentNodesList \|\| \[\]\)\.length > 0 \? assignmentNodesList : selectedNodesList/,
    'dialog prefers the dependency-inclusive list'
  );
  assert.match(source, /前置依赖节点同样出现在候选列表中/, 'helper text explains front dependencies are assignable');
});

test('selected node panel exposes the chosen nodes with their group assignments', () => {
  assert.match(source, /const assignedGroupNamesForNode = \(nodeId\) =>/, 'dialog resolves assigned groups per node');
  assert.match(source, /const removeNodeFromGroup = \(nodeId, groupName\) =>/, 'dialog can drop one group assignment');
  assert.match(
    source,
    /t\('subscriptions\.form\.nodes\.selectedList', \{ count: assignmentCandidateNodes\.length \}\)/,
    'selected node overview lists candidates'
  );
  assert.match(source, /t\('subscriptions\.form\.nodes\.frontDependency'\)/, 'front dependencies are labelled');
});

test('selected node list supports keyword, group, source and front-only filtering', () => {
  const transferSource = readFileSync(new URL('./component/NodeTransferBox.jsx', import.meta.url), 'utf8');
  assert.match(transferSource, /selectedNodeGroupFilter !== 'all'/, 'group filter is applied');
  assert.match(transferSource, /selectedNodeSourceFilter !== 'all'/, 'source filter is applied');
  assert.match(transferSource, /selectedNodeFrontOnly && !String\(node\.DialerProxyName/, 'front-only filter is applied');
  assert.match(transferSource, /const resetSelectedFilters = \(\) =>/, 'filters can be reset');
  const indexSource = readFileSync(new URL('./index.jsx', import.meta.url), 'utf8');
  for (const state of ['selectedNodeGroupFilter', 'selectedNodeSourceFilter', 'selectedNodeFrontOnly']) {
    assert.ok(indexSource.includes(`const [${state}, set`), `${state} state exists in index.jsx`);
  }
  for (const locale of ['en-US', 'zh-CN']) {
    const data = JSON.parse(readFileSync(new URL(`../../i18n/locales/${locale}.json`, import.meta.url), 'utf8'));
    for (const key of ['frontOnly', 'filteredCount', 'resetFilters']) {
      assert.ok(data.subscriptions.transfer[key], `${locale} has transfer.${key}`);
    }
    for (const key of ['selectedList', 'frontDependency', 'unassigned']) {
      assert.ok(data.subscriptions.form.nodes[key], `${locale} has form.nodes.${key}`);
    }
  }
});
