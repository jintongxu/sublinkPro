import { useMemo, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import TextField from '@mui/material/TextField';
import Stack from '@mui/material/Stack';
import Alert from '@mui/material/Alert';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Checkbox from '@mui/material/Checkbox';
import FormControlLabel from '@mui/material/FormControlLabel';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';
import Autocomplete from '@mui/material/Autocomplete';
import Tooltip from '@mui/material/Tooltip';
import InputAdornment from '@mui/material/InputAdornment';
import Grid from '@mui/material/Grid';
import ButtonGroup from '@mui/material/ButtonGroup';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import Chip from '@mui/material/Chip';
import Popover from '@mui/material/Popover';

// icons
import BuildIcon from '@mui/icons-material/Build';
import EditNoteIcon from '@mui/icons-material/EditNote';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import SettingsIcon from '@mui/icons-material/Settings';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import FilterListIcon from '@mui/icons-material/FilterList';
import TextFieldsIcon from '@mui/icons-material/TextFields';
import SecurityIcon from '@mui/icons-material/Security';
import VisibilityIcon from '@mui/icons-material/Visibility';
import AddIcon from '@mui/icons-material/Add';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import SearchIcon from '@mui/icons-material/Search';

import NodeRenameBuilder from './NodeRenameBuilder';
import { previewSubscriptionNodeName } from './nodeRenameRuleUtils';
import NodeNamePreprocessor from 'components/NodeNamePreprocessor';
import NodeNameFilter from 'components/NodeNameFilter';
import NodeTagFilter from './NodeTagFilter';
import NodeProtocolFilter from 'components/NodeProtocolFilter';
import NodeTransferBox from './NodeTransferBox';
import DeduplicationConfig from './DeduplicationConfig';
import FilterAltIcon from '@mui/icons-material/FilterAlt';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getReadableTextTokens, getSurfaceTokens } from 'themes/surfaceTokens';
import { withAlpha } from 'utils/colorUtils';
import { getFraudScoreIcon, QUALITY_STATUS_OPTIONS } from 'utils/fraudScore';
import { getDelayIcon, getSpeedIcon } from 'utils/nodeMetricIcons';
import { formatCountry } from 'utils/countryDisplay';
import {
  formatUnlockProviderLabel,
  getNodeUnlockSummaryDisplay,
  getUnlockProviderOptions,
  getUnlockRenameVariables,
  getUnlockRuleModeOptions,
  getUnlockStatusOptions,
  createEmptyUnlockRule
} from 'views/nodes/utils';

const normalizeCountryCode = (value) => (typeof value === 'string' ? value.trim().toUpperCase() : '');

const normalizeCountryCodeList = (values) => {
  if (!Array.isArray(values)) return [];
  return Array.from(new Set(values.map((value) => normalizeCountryCode(value)).filter(Boolean)));
};

const hasPreprocessRules = (value) => {
  if (!value) return false;
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) && parsed.length > 0;
  } catch {
    return false;
  }
};

export default function SubscriptionFormDialog({
  open,
  isEdit,
  formData,
  setFormData,
  chainRules = [],
  templates,
  scripts,
  selectorNodes,
  selectorNodesTotal,
  selectorNodesLoading,
  availableNodePage,
  availableNodePageCount,
  onAvailableNodePageChange,
  selectedNodesList,
  assignmentNodesList,
  groupNodeCounts,
  allNodeTotal,
  groupOptions,
  airportOptions,
  sourceOptions,
  countryOptions,
  tagOptions,
  nodeGroupFilter,
  setNodeGroupFilter,
  nodeSourceFilter,
  setNodeSourceFilter,
  nodeSearchQuery,
  setNodeSearchQuery,
  nodeCountryFilter,
  setNodeCountryFilter,
  checkedAvailable,
  checkedSelected,
  mobileTab,
  setMobileTab,
  selectedNodeSearch,
  setSelectedNodeSearch,
  selectedNodeGroupFilter = 'all',
  setSelectedNodeGroupFilter = () => {},
  selectedNodeSourceFilter = 'all',
  setSelectedNodeSourceFilter = () => {},
  selectedNodeFrontOnly = false,
  setSelectedNodeFrontOnly = () => {},
  namingMode,
  setNamingMode,
  onClose,
  onSubmit,
  onPreview,
  previewLoading,
  onAddNode,
  onRemoveNode,
  onAddAllVisible,
  onRemoveAll,
  onToggleAvailable,
  onToggleSelected,
  onAddChecked,
  onRemoveChecked,
  onToggleAllAvailable,
  onToggleAllSelected
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { isDark } = useResolvedColorScheme();
  const matchDownMd = useMediaQuery(theme.breakpoints.down('md'));
  const { palette, dialogSurface, dialogSurfaceGradient, mutedPanelSurface, nestedPanelSurface, panelBorder } = getSurfaceTokens(
    theme,
    isDark
  );
  const { primaryText, secondaryText, tertiaryText } = getReadableTextTokens(theme, isDark);
  const [countryWhitelistInput, setCountryWhitelistInput] = useState('');
  const [countryBlacklistInput, setCountryBlacklistInput] = useState('');

  const [assignmentMenuGroup, setAssignmentMenuGroup] = useState(null);
  const [assignmentMenuAnchor, setAssignmentMenuAnchor] = useState(null);
  const [assignmentDraftNodeIds, setAssignmentDraftNodeIds] = useState([]);
  const [assignmentNodeSearch, setAssignmentNodeSearch] = useState('');
  const [assignmentNodeGroupFilter, setAssignmentNodeGroupFilter] = useState('all');
  const [assignmentNodeSourceFilter, setAssignmentNodeSourceFilter] = useState('all');
  // 仅记录当前编辑会话中点击“完成”后确认的组内节点顺序。
  const [assignmentGroupOrder, setAssignmentGroupOrder] = useState({});

  const [expandedPanels, setExpandedPanels] = useState({
    basic: true,
    nodes: true,
    filter: false,
    dedup: false,
    naming: false,
    advanced: false
  });

  const handlePanelChange = (panel) => (event, isExpanded) => {
    void event;
    setExpandedPanels((prev) => ({
      ...prev,
      [panel]: isExpanded
    }));
  };

  const clashTemplates = useMemo(() => {
    return templates.filter((t) => !t.category || t.category === 'clash');
  }, [templates]);

  const surgeTemplates = useMemo(() => {
    return templates.filter((t) => t.category === 'surge');
  }, [templates]);

  const selectedClashTemplate = useMemo(() => {
    const selectedPath = formData.clash || '';
    const selectedFile = selectedPath.split(/[\\/]/).pop();
    return (
      clashTemplates.find((template) => {
        const file = template.file || template.File || template.filename || template.name || template.Name;
        return file === selectedFile;
      }) || clashTemplates[0]
    );
  }, [clashTemplates, formData.clash]);

  const clashProxyGroupMetadata = useMemo(() => {
    const content = selectedClashTemplate?.text || selectedClashTemplate?.Text || '';
    const groups = [];
    const lines = content.split(/\r?\n/);
    let inProxyGroups = false;
    let sectionIndent = -1;
    let currentGroup = null;
    const flushGroup = () => {
      if (currentGroup?.name && currentGroup.type !== 'relay' && !groups.some((group) => group.name === currentGroup.name)) {
        const references = Object.entries(currentGroup.references)
          .filter(([, value]) => value !== undefined && value !== '')
          .map(([key, value]) => `${key}${value === true ? '' : `=${value}`}`);
        groups.push({ ...currentGroup, references });
      }
      currentGroup = null;
    };
    for (const line of lines) {
      if (!line.trim() || line.trimStart().startsWith('#')) continue;
      const indent = line.match(/^\s*/)?.[0].length || 0;
      if (!inProxyGroups) {
        if (/^\s*proxy-groups\s*:/.test(line)) {
          inProxyGroups = true;
          sectionIndent = indent;
        }
        continue;
      }
      if (indent <= sectionIndent && !/^\s*-/.test(line)) {
        flushGroup();
        break;
      }
      const nameMatch = line.match(/^\s*-\s+name\s*:\s*(.*?)\s*$/);
      if (nameMatch) {
        flushGroup();
        currentGroup = {
          name: nameMatch[1]
            .trim()
            .replace(/^(['"])(.*)\1$/, '$2')
            .trim(),
          type: '',
          references: {}
        };
        continue;
      }
      if (!currentGroup) continue;
      const typeMatch = line.match(/^\s+type\s*:\s*(.*?)\s*$/);
      if (typeMatch) {
        currentGroup.type = typeMatch[1]
          .trim()
          .replace(/^(['"])(.*)\1$/, '$2')
          .trim();
        continue;
      }
      const referenceMatch = line.match(/^\s+(include-all(?:-proxies|-providers)?|use|filter|exclude-filter|exclude-type)\s*:\s*(.*?)\s*$/);
      if (referenceMatch) {
        const [, key, rawValue] = referenceMatch;
        const value = rawValue.trim().replace(/^(['"])(.*)\1$/, '$2');
        currentGroup.references[key] = value || true;
      }
    }
    flushGroup();
    return groups;
  }, [selectedClashTemplate]);

  const chainProxyGroupTargets = useMemo(() => {
    const targets = new Map();
    const nodeNames = new Map(
      (selectedNodesList || []).map((node) => [Number(node.ID), node.Name || node.EffectiveName || `节点 #${node.ID}`])
    );
    const addTarget = (groupName, target) => {
      if (!groupName || !target) return;
      const values = targets.get(groupName) || new Set();
      values.add(target);
      targets.set(groupName, values);
    };
    chainRules
      .filter((rule) => rule.enabled !== false)
      .forEach((rule) => {
        try {
          const chain = typeof rule.chainConfig === 'string' ? JSON.parse(rule.chainConfig) : rule.chainConfig;
          const target = typeof rule.targetConfig === 'string' ? JSON.parse(rule.targetConfig) : rule.targetConfig;
          const sourceGroups = (Array.isArray(chain) ? chain : [])
            .filter((item) => item?.type === 'template_group' && item.groupName)
            .map((item) => item.groupName);
          const targetText =
            target?.type === 'template_group'
              ? target.groupName
              : target?.nodeId
                ? nodeNames.get(Number(target.nodeId)) || `节点 #${target.nodeId}`
                : target?.nodeIds?.length
                  ? target.nodeIds.map((id) => nodeNames.get(Number(id)) || `节点 #${id}`).join('、')
                  : target?.type || '目标';
          sourceGroups.forEach((groupName) => addTarget(groupName, targetText));
        } catch {
          // Ignore malformed legacy rules; the chain dialog remains the source of truth.
        }
      });
    return targets;
  }, [chainRules, selectedNodesList]);

  const chainTargetNodeGroups = useMemo(() => {
    const groupsByNode = new Map();
    chainRules
      .filter((rule) => rule.enabled !== false)
      .forEach((rule) => {
        try {
          const chain = typeof rule.chainConfig === 'string' ? JSON.parse(rule.chainConfig) : rule.chainConfig;
          const target = typeof rule.targetConfig === 'string' ? JSON.parse(rule.targetConfig) : rule.targetConfig;
          const sourceGroups = (Array.isArray(chain) ? chain : [])
            .filter((item) => item?.type === 'template_group' && item.groupName)
            .map((item) => item.groupName);
          const targetIds =
            target?.type === 'specified_node' && target.nodeId
              ? [target.nodeId]
              : target?.type === 'specified_nodes' && Array.isArray(target.nodeIds)
                ? target.nodeIds
                : [];
          targetIds.forEach((nodeId) => {
            const values = groupsByNode.get(Number(nodeId)) || new Set();
            sourceGroups.forEach((groupName) => values.add(groupName));
            groupsByNode.set(Number(nodeId), values);
          });
        } catch {
          // Ignore malformed legacy rules.
        }
      });
    return groupsByNode;
  }, [chainRules]);

  const assignmentCandidateNodes = useMemo(() => {
    const seen = new Set();
    const source = (assignmentNodesList || []).length > 0 ? assignmentNodesList : selectedNodesList;
    return (source || []).filter((node) => {
      const id = Number(node.ID);
      if (!Number.isInteger(id) || id <= 0 || seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }, [assignmentNodesList, selectedNodesList]);

  // 自动展开的前置依赖节点不属于用户显式选择的节点，但仍可手动分配到代理组。
  const explicitAssignmentNodeIds = useMemo(() => new Set((selectedNodesList || []).map((node) => Number(node.ID))), [selectedNodesList]);

  const assignmentNodeGroupOptions = useMemo(
    () => Array.from(new Set(assignmentCandidateNodes.map((node) => String(node.Group || '')).filter(Boolean))).sort(),
    [assignmentCandidateNodes]
  );
  const assignmentNodeSourceOptions = useMemo(
    () => Array.from(new Set(assignmentCandidateNodes.map((node) => String(node.Source || '')).filter(Boolean))).sort(),
    [assignmentCandidateNodes]
  );

  const assignmentFilteredNodes = useMemo(() => {
    const keyword = assignmentNodeSearch.trim().toLowerCase();
    return assignmentCandidateNodes.filter((node) => {
      if (assignmentNodeGroupFilter !== 'all' && String(node.Group || '') !== assignmentNodeGroupFilter) return false;
      if (assignmentNodeSourceFilter !== 'all' && String(node.Source || '') !== assignmentNodeSourceFilter) return false;
      if (!keyword) return true;
      return [node.Name, node.EffectiveName, node.LinkName, node.Group, node.Source]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword));
    });
  }, [assignmentCandidateNodes, assignmentNodeGroupFilter, assignmentNodeSearch, assignmentNodeSourceFilter]);

  // 完成确认后再次打开同一组时，把已确认节点排在未选节点前面。
  // 未点击“完成”前只更新 draft，不改变这里的排序结果。
  const assignmentDisplayNodes = useMemo(() => {
    const selectedIds = new Set(assignmentDraftNodeIds.map(Number));
    const confirmedOrder = assignmentGroupOrder[assignmentMenuGroup] || [];
    const confirmedSet = new Set(confirmedOrder.map(Number));
    const orderIndex = new Map(confirmedOrder.map((id, index) => [Number(id), index]));
    return [...assignmentFilteredNodes].sort((left, right) => {
      const leftId = Number(left.ID);
      const rightId = Number(right.ID);
      const leftConfirmedSelected = selectedIds.has(leftId) && confirmedSet.has(leftId);
      const rightConfirmedSelected = selectedIds.has(rightId) && confirmedSet.has(rightId);
      if (leftConfirmedSelected !== rightConfirmedSelected) return leftConfirmedSelected ? -1 : 1;
      if (leftConfirmedSelected && rightConfirmedSelected) {
        return (orderIndex.get(leftId) ?? Number.MAX_SAFE_INTEGER) - (orderIndex.get(rightId) ?? Number.MAX_SAFE_INTEGER);
      }
      return 0;
    });
  }, [assignmentDraftNodeIds, assignmentFilteredNodes, assignmentGroupOrder, assignmentMenuGroup]);

  const assignedNodeIdsForGroup = (groupName) =>
    assignmentCandidateNodes
      .filter((node) => {
        const rawGroups = formData.nodeTemplateAssignments?.[String(node.ID)];
        const groups = Array.isArray(rawGroups) ? rawGroups : rawGroups ? [rawGroups] : [];
        return groups.includes(groupName);
      })
      .map((node) => Number(node.ID));

  const orderedAssignedNodeIdsForGroup = (groupName) => {
    const assignedIds = assignedNodeIdsForGroup(groupName);
    const assignedSet = new Set(assignedIds);
    const remembered = assignmentGroupOrder[groupName] || [];
    return [...remembered.filter((id) => assignedSet.has(Number(id))).map(Number), ...assignedIds.filter((id) => !remembered.includes(id))];
  };

  const assignedGroupNamesForNode = (nodeId) => {
    const rawGroups = formData.nodeTemplateAssignments?.[String(nodeId)];
    return (Array.isArray(rawGroups) ? rawGroups : rawGroups ? [rawGroups] : []).filter(Boolean);
  };

  const removeNodeFromGroup = (nodeId, groupName) => {
    setFormData((prev) => {
      const next = { ...(prev.nodeTemplateAssignments || {}) };
      const key = String(nodeId);
      const remaining = assignedGroupNamesForNode(nodeId).filter((name) => name !== groupName);
      if (remaining.length > 0) next[key] = remaining;
      else delete next[key];
      return { ...prev, nodeTemplateAssignments: next };
    });
  };

  const closeAssignmentMenu = () => {
    setAssignmentMenuGroup(null);
    setAssignmentMenuAnchor(null);
    setAssignmentNodeSearch('');
    setAssignmentNodeGroupFilter('all');
    setAssignmentNodeSourceFilter('all');
  };

  const applyGroupAssignmentDraft = () => {
    if (!assignmentMenuGroup) return;
    const selected = new Set(assignmentDraftNodeIds.map(Number));
    setFormData((prev) => {
      const next = { ...(prev.nodeTemplateAssignments || {}) };
      assignmentCandidateNodes.forEach((node) => {
        const nodeId = String(node.ID);
        const groups = Array.isArray(next[nodeId]) ? next[nodeId] : next[nodeId] ? [next[nodeId]] : [];
        const withoutCurrentGroup = groups.filter((name) => name !== assignmentMenuGroup);
        if (selected.has(Number(node.ID))) withoutCurrentGroup.push(assignmentMenuGroup);
        if (withoutCurrentGroup.length > 0) next[nodeId] = Array.from(new Set(withoutCurrentGroup));
        else delete next[nodeId];
      });
      return { ...prev, nodeTemplateAssignments: next };
    });
    setAssignmentGroupOrder((current) => ({ ...current, [assignmentMenuGroup]: Array.from(selected) }));
    closeAssignmentMenu();
  };

  const unlockProviderOptions = getUnlockProviderOptions();
  const unlockRenameVariables = getUnlockRenameVariables();
  const unlockRules = useMemo(() => (Array.isArray(formData.unlockRules) ? formData.unlockRules : []), [formData.unlockRules]);
  const normalizedCountryOptions = useMemo(() => normalizeCountryCodeList(countryOptions), [countryOptions]);
  const normalizedCountryWhitelist = useMemo(() => normalizeCountryCodeList(formData.CountryWhitelist), [formData.CountryWhitelist]);
  const normalizedCountryBlacklist = useMemo(() => normalizeCountryCodeList(formData.CountryBlacklist), [formData.CountryBlacklist]);
  const selectedAirportOptions = useMemo(() => {
    const airportByID = new Map((airportOptions || []).map((airport) => [Number(airport.id ?? airport.ID), airport]));
    return (formData.selectedAirports || []).map((id) => airportByID.get(Number(id)) || { id: Number(id), name: `#${id}` });
  }, [airportOptions, formData.selectedAirports]);

  const updateCountryFilterField = (field, values) => {
    setFormData({ ...formData, [field]: normalizeCountryCodeList(values) });
  };

  const handleCountryFilterKeyDown = (event, field, inputValue, setInputValue) => {
    if (event.key !== 'Enter') return;
    const normalizedInput = normalizeCountryCode(inputValue);
    if (!normalizedInput) return;
    event.preventDefault();
    updateCountryFilterField(field, [...normalizeCountryCodeList(formData[field]), normalizedInput]);
    setInputValue('');
  };

  useEffect(() => {
    if (open && groupOptions && formData.selectedGroups?.length > 0) {
      const validGroups = formData.selectedGroups.filter((g) => groupOptions.includes(g));
      if (validGroups.length !== formData.selectedGroups.length) {
        setFormData((prev) => ({ ...prev, selectedGroups: validGroups }));
      }
    }
  }, [groupOptions, open, formData.selectedGroups, setFormData]);

  const normalizedSelectorNodes = useMemo(() => selectorNodes || [], [selectorNodes]);
  const selectorLoadingText = selectorNodesLoading ? t('subscriptions.form.nodes.loading') : '';

  const availableNodes = useMemo(() => {
    return normalizedSelectorNodes.filter((node) => {
      if (nodeGroupFilter !== 'all' && node.Group !== nodeGroupFilter) return false;
      if (nodeSourceFilter !== 'all' && node.Source !== nodeSourceFilter) return false;
      if (nodeSearchQuery) {
        const query = nodeSearchQuery.toLowerCase();
        const unlockSummary = getNodeUnlockSummaryDisplay(node, { limit: 4 });
        const unlockText = unlockSummary?.items
          ?.map((item) => [item.providerLabel, item.statusLabel, item.region, item.reason, item.detail].filter(Boolean).join(' '))
          .join(' ')
          .toLowerCase();
        if (!node.Name?.toLowerCase().includes(query) && !node.Group?.toLowerCase().includes(query) && !unlockText?.includes(query)) {
          return false;
        }
      }
      if (nodeCountryFilter.length > 0) {
        if (!node.LinkCountry || !nodeCountryFilter.includes(node.LinkCountry)) {
          return false;
        }
      }
      return !formData.selectedNodes.includes(node.ID);
    });
  }, [normalizedSelectorNodes, nodeGroupFilter, nodeSourceFilter, nodeSearchQuery, nodeCountryFilter, formData.selectedNodes]);

  const selectorNodesCount = selectorNodesTotal || availableNodes.length;

  const filterRulesCount = useMemo(() => {
    let count = 0;
    if (formData.DelayTime > 0) count++;
    if (formData.MinSpeed > 0) count++;
    if (formData.CountryWhitelist?.length > 0) count++;
    if (formData.CountryBlacklist?.length > 0) count++;
    if (formData.tagWhitelist) count++;
    if (formData.tagBlacklist) count++;
    if (formData.protocolWhitelist) count++;
    if (formData.protocolBlacklist) count++;
    if (formData.nodeNameWhitelist) count++;
    if (formData.nodeNameBlacklist) count++;
    if (formData.MaxFraudScore > 0) count++;
    if (formData.QualityStatus) count++;
    if (formData.ValidationPolicy && formData.ValidationPolicy !== 'all') count++;
    if (formData.ResidentialType) count++;
    if (formData.IPType) count++;
    if (unlockRules.some((rule) => rule.provider || rule.status || rule.keyword)) count++;
    return count;
  }, [formData, unlockRules]);

  const updateUnlockRule = (index, patch) => {
    const nextRules = unlockRules.map((rule, ruleIndex) => (ruleIndex === index ? { ...rule, ...patch } : rule));
    setFormData({ ...formData, unlockRules: nextRules });
  };

  const addUnlockRule = () => {
    setFormData({ ...formData, unlockRules: [...unlockRules, createEmptyUnlockRule()] });
  };

  const removeUnlockRule = (index) => {
    const nextRules = unlockRules.filter((_, ruleIndex) => ruleIndex !== index);
    setFormData({ ...formData, unlockRules: nextRules });
  };

  const advancedSettingsCount = useMemo(() => {
    let count = 0;
    if (formData.selectedScripts?.length > 0) count++;
    if (formData.IPWhitelist) count++;
    if (formData.IPBlacklist) count++;
    return count;
  }, [formData]);

  const helperCaptionSx = {
    display: 'block',
    mt: 1,
    color: secondaryText
  };

  const insetHighlight = isDark ? `inset 0 1px 0 ${withAlpha(palette.common.white, 0.03)}` : 'none';
  const accordionHoverBorder = withAlpha(palette.primary.main, isDark ? 0.3 : 0.16);
  const accordionExpandedBorder = withAlpha(palette.primary.main, isDark ? 0.36 : 0.2);
  const accordionSummaryHoverSurface = isDark ? withAlpha(palette.background.paper, 0.22) : withAlpha(palette.primary.main, 0.04);
  const accordionDetailsSurface = isDark
    ? `linear-gradient(180deg, ${withAlpha(palette.background.paper, 0.08)} 0%, ${dialogSurface} 100%)`
    : dialogSurface;
  const helperPanelSx = {
    mt: 1,
    p: 1.5,
    bgcolor: nestedPanelSurface,
    borderRadius: 1.5,
    border: '1px solid',
    borderColor: panelBorder,
    boxShadow: insetHighlight
  };

  const accordionSx = {
    mb: 1.5,
    '&:before': { display: 'none' },
    bgcolor: dialogSurface,
    backgroundImage: isDark ? `linear-gradient(180deg, ${withAlpha(palette.background.paper, 0.12)} 0%, ${dialogSurface} 100%)` : 'none',
    border: '1px solid',
    borderColor: panelBorder,
    boxShadow: insetHighlight,
    borderRadius: '12px !important',
    overflow: 'hidden',
    transition: 'border-color 0.2s ease, box-shadow 0.2s ease',
    '&:hover': {
      borderColor: accordionHoverBorder
    },
    '&.Mui-expanded': {
      margin: '0 0 12px 0',
      borderColor: accordionExpandedBorder
    }
  };

  const accordionSummarySx = {
    minHeight: 56,
    px: 0.5,
    color: primaryText,
    bgcolor: mutedPanelSurface,
    transition: 'background-color 0.2s ease, border-color 0.2s ease',
    '& .MuiAccordionSummary-expandIconWrapper': {
      color: tertiaryText,
      transition: 'color 0.2s ease, transform 0.2s ease'
    },
    '&:hover': {
      bgcolor: accordionSummaryHoverSurface,
      '& .MuiAccordionSummary-expandIconWrapper': {
        color: secondaryText
      }
    },
    '&.Mui-expanded': {
      minHeight: 56,
      bgcolor: nestedPanelSurface,
      borderBottom: '1px solid',
      borderColor: panelBorder,
      '& .MuiAccordionSummary-expandIconWrapper': {
        color: primaryText
      }
    },
    '& .MuiAccordionSummary-content': {
      alignItems: 'center',
      gap: 1.5,
      '&.Mui-expanded': {
        margin: '12px 0'
      }
    }
  };

  const accordionDetailsSx = {
    px: matchDownMd ? 2 : 2.5,
    py: 2.25,
    bgcolor: dialogSurface,
    backgroundImage: accordionDetailsSurface
  };

  const autocompleteChipSx = {
    '& .MuiAutocomplete-tag': {
      bgcolor: isDark ? withAlpha(palette.primary.main, 0.16) : undefined,
      color: isDark ? primaryText : undefined,
      border: isDark ? '1px solid' : undefined,
      borderColor: isDark ? withAlpha(palette.primary.main, 0.3) : undefined,
      '& .MuiChip-deleteIcon': {
        color: isDark ? withAlpha(palette.primary.main, 0.7) : undefined,
        transition: 'color 0.2s',
        '&:hover': {
          color: isDark ? palette.primary.main : undefined
        }
      }
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      fullScreen={matchDownMd}
      slotProps={{
        paper: {
          sx: matchDownMd
            ? {
                borderRadius: 0,
                border: '1px solid',
                borderColor: panelBorder,
                bgcolor: dialogSurface,
                backgroundImage: dialogSurfaceGradient
              }
            : {
                borderRadius: 2.5,
                border: '1px solid',
                borderColor: panelBorder,
                bgcolor: dialogSurface,
                backgroundImage: dialogSurfaceGradient
              }
        }
      }}
    >
      <DialogTitle
        sx={{
          pb: 1.5,
          color: primaryText,
          bgcolor: mutedPanelSurface,
          borderBottom: '1px solid',
          borderColor: panelBorder
        }}
      >
        {isEdit ? t('subscriptions.form.title.edit') : t('subscriptions.form.title.add')}
      </DialogTitle>
      <DialogContent sx={{ px: matchDownMd ? 2 : 3, pt: 2.5, pb: 2, bgcolor: 'transparent' }}>
        <Box sx={{ mt: 1 }}>
          {/* ========== {t('subscriptions.form.sections.basic')} ========== */}
          <Accordion expanded={expandedPanels.basic} onChange={handlePanelChange('basic')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <SettingsIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.basic')}
              </Typography>
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <Stack spacing={2.5}>
                <TextField
                  fullWidth
                  label={t('subscriptions.form.basic.name')}
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />

                <Grid container spacing={2}>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel shrink>{t('subscriptions.form.basic.clashTemplate')}</InputLabel>
                      <Select
                        variant={'outlined'}
                        value={formData.clash}
                        label={t('subscriptions.form.basic.clashTemplate')}
                        onChange={(e) => setFormData({ ...formData, clash: e.target.value })}
                        displayEmpty
                      >
                        <MenuItem value="">
                          <Typography color="text.secondary">{t('subscriptions.form.basic.clashTemplateNone')}</Typography>
                        </MenuItem>
                        {clashTemplates.map((t) => (
                          <MenuItem key={t.file} value={`./template/${t.file}`}>
                            {t.file}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    {clashTemplates.length === 0 && (
                      <Alert severity="warning" sx={{ mt: 1 }}>
                        <Typography variant="caption">{t('subscriptions.form.basic.clashTemplateHelper')}</Typography>
                      </Alert>
                    )}
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel shrink>{t('subscriptions.form.basic.surgeTemplate')}</InputLabel>
                      <Select
                        value={formData.surge}
                        label={t('subscriptions.form.basic.surgeTemplate')}
                        onChange={(e) => setFormData({ ...formData, surge: e.target.value })}
                        displayEmpty
                      >
                        <MenuItem value="">
                          <Typography color="text.secondary">{t('subscriptions.form.basic.clashTemplateNone')}</Typography>
                        </MenuItem>
                        {surgeTemplates.map((t) => (
                          <MenuItem key={t.file} value={`./template/${t.file}`}>
                            {t.file}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    {surgeTemplates.length === 0 && (
                      <Alert severity="warning" sx={{ mt: 1 }}>
                        <Typography variant="caption">{t('subscriptions.form.basic.surgeTemplateHelper')}</Typography>
                      </Alert>
                    )}
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label={t('subscriptions.form.basic.updateInterval')}
                      type="text"
                      slotProps={{ htmlInput: { inputMode: 'numeric', pattern: '[0-9]*', max: 8760 } }}
                      value={formData.UpdateInterval}
                      onChange={(e) => {
                        const val = e.target.value;
                        if (val === '' || /^\d+$/.test(val)) {
                          setFormData({ ...formData, UpdateInterval: val === '' ? '' : Number(val) });
                        }
                      }}
                      onBlur={(e) => {
                        const val = Math.min(8760, Math.max(0, Number(e.target.value) || 0));
                        setFormData({ ...formData, UpdateInterval: val });
                      }}
                      helperText={t('subscriptions.form.basic.updateIntervalHelper')}
                    />
                  </Grid>
                </Grid>

                <Stack direction="row" spacing={2} flexWrap="wrap">
                  <FormControlLabel
                    control={<Checkbox checked={formData.udp} onChange={(e) => setFormData({ ...formData, udp: e.target.checked })} />}
                    label={t('subscriptions.form.basic.forceUdp')}
                  />
                  <FormControlLabel
                    control={<Checkbox checked={formData.cert} onChange={(e) => setFormData({ ...formData, cert: e.target.checked })} />}
                    label={t('subscriptions.form.basic.skipCertVerify')}
                  />
                  <Tooltip title={t('subscriptions.form.basic.replaceHostTooltip')} placement="top" arrow>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={formData.replaceServerWithHost}
                          onChange={(e) => setFormData({ ...formData, replaceServerWithHost: e.target.checked })}
                        />
                      }
                      label={t('subscriptions.form.basic.replaceHost')}
                    />
                  </Tooltip>
                  <Tooltip title={t('subscriptions.form.basic.realtimeUsageTooltip')} placement="top" arrow>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={formData.refreshUsageOnRequest}
                          onChange={(e) => setFormData({ ...formData, refreshUsageOnRequest: e.target.checked })}
                        />
                      }
                      label={t('subscriptions.form.basic.realtimeUsage')}
                    />
                  </Tooltip>
                </Stack>
              </Stack>
            </AccordionDetails>
          </Accordion>

          {/* ========== {t('subscriptions.form.sections.nodeSelection')} ========== */}
          <Accordion expanded={expandedPanels.nodes} onChange={handlePanelChange('nodes')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <AccountTreeIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.nodeSelection')}
              </Typography>
              {!expandedPanels.nodes &&
                (formData.selectedNodes.length > 0 || formData.selectedGroups.length > 0 || formData.selectedAirports.length > 0) && (
                  <Chip
                    size="small"
                    label={t('subscriptions.form.nodeSelection.summary', {
                      nodeCount: formData.selectedNodes.length,
                      groupCount: formData.selectedGroups.length,
                      airportCount: formData.selectedAirports.length
                    })}
                    color="primary"
                    variant="outlined"
                    sx={{ ml: 1 }}
                  />
                )}
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <Stack spacing={2.5}>
                {/* Selection Mode */}
                <Box>
                  <RadioGroup
                    row
                    value={formData.selectionMode}
                    onChange={(e) => setFormData({ ...formData, selectionMode: e.target.value })}
                  >
                    <FormControlLabel value="nodes" control={<Radio />} label={t('subscriptions.form.nodeSelection.modeManual')} />
                    <FormControlLabel value="groups" control={<Radio />} label={t('subscriptions.form.nodeSelection.modeDynamic')} />
                    <FormControlLabel value="mixed" control={<Radio />} label={t('subscriptions.form.nodeSelection.modeMixed')} />
                  </RadioGroup>
                  <Typography variant="caption" sx={helperCaptionSx}>
                    {formData.selectionMode === 'nodes' && t('subscriptions.form.nodeSelection.modeManualHelper')}
                    {formData.selectionMode === 'groups' && t('subscriptions.form.nodeSelection.modeDynamicHelper')}
                    {formData.selectionMode === 'mixed' && t('subscriptions.form.nodeSelection.modeMixedHelper')}
                  </Typography>
                </Box>

                {/* Group Selection */}
                {(formData.selectionMode === 'groups' || formData.selectionMode === 'mixed') && (
                  <Stack spacing={2}>
                    <Autocomplete
                      multiple
                      options={groupOptions}
                      value={formData.selectedGroups}
                      onChange={(event, newValue) => {
                        void event;
                        setFormData({ ...formData, selectedGroups: newValue });
                      }}
                      sx={autocompleteChipSx}
                      renderInput={(params) => (
                        <TextField
                          {...params}
                          label={t('subscriptions.form.nodeSelection.groupSelect')}
                          helperText={t('subscriptions.form.nodeSelection.groupSelectHelper')}
                        />
                      )}
                      renderOption={(props, option) => (
                        <li {...props}>
                          {t('subscriptions.form.nodeSelection.groupOption', { name: option, count: groupNodeCounts[option] || 0 })}
                        </li>
                      )}
                    />
                    <Autocomplete
                      multiple
                      options={airportOptions || []}
                      value={selectedAirportOptions}
                      onChange={(event, newValue) => {
                        void event;
                        setFormData({
                          ...formData,
                          selectedAirports: newValue.map((airport) => Number(airport.id ?? airport.ID)).filter((id) => Number.isInteger(id))
                        });
                      }}
                      getOptionLabel={(option) => option?.name || option?.Name || ''}
                      isOptionEqualToValue={(option, value) => Number(option?.id ?? option?.ID) === Number(value?.id ?? value?.ID)}
                      sx={autocompleteChipSx}
                      renderInput={(params) => (
                        <TextField
                          {...params}
                          label={t('subscriptions.form.nodeSelection.airportSelect')}
                          helperText={t('subscriptions.form.nodeSelection.airportSelectHelper')}
                        />
                      )}
                      renderOption={(props, option) => (
                        <li {...props}>
                          {t('subscriptions.form.nodeSelection.airportOption', {
                            name: option.name || option.Name,
                            count: option.nodeCount || option.NodeCount || 0
                          })}
                        </li>
                      )}
                    />
                  </Stack>
                )}

                {/* {t('subscriptions.form.sections.nodeSelection')} */}
                {(formData.selectionMode === 'nodes' || formData.selectionMode === 'mixed') && (
                  <>
                    <Grid container spacing={2}>
                      <Grid item xs={6} sm={3}>
                        <FormControl fullWidth size="small">
                          <InputLabel>{t('subscriptions.form.nodeSelection.nodeGroupFilter')}</InputLabel>
                          <Select
                            value={nodeGroupFilter}
                            label={t('subscriptions.form.nodeSelection.nodeGroupFilter')}
                            onChange={(e) => setNodeGroupFilter(e.target.value)}
                          >
                            <MenuItem value="all">{t('subscriptions.form.nodeSelection.nodeGroupAll', { count: allNodeTotal })}</MenuItem>
                            {groupOptions.map((g) => (
                              <MenuItem key={g} value={g}>
                                {g} ({groupNodeCounts[g] || 0})
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>
                      </Grid>
                      <Grid item xs={6} sm={3}>
                        <FormControl fullWidth size="small">
                          <InputLabel>{t('subscriptions.form.nodeSelection.nodeSourceFilter')}</InputLabel>
                          <Select
                            value={nodeSourceFilter}
                            label={t('subscriptions.form.nodeSelection.nodeSourceFilter')}
                            onChange={(e) => setNodeSourceFilter(e.target.value)}
                          >
                            <MenuItem value="all">{t('subscriptions.form.nodeSelection.nodeSourceAll')}</MenuItem>
                            {sourceOptions.map((s) => (
                              <MenuItem key={s} value={s}>
                                {s}
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>
                      </Grid>
                      <Grid item xs={6} sm={3}>
                        <Autocomplete
                          multiple
                          size="small"
                          options={countryOptions}
                          value={nodeCountryFilter}
                          onChange={(e, newValue) => setNodeCountryFilter(newValue)}
                          getOptionLabel={(option) => formatCountry(option)}
                          renderInput={(params) => (
                            <TextField
                              {...params}
                              label={t('subscriptions.form.nodeSelection.nodeCountryFilter')}
                              sx={{
                                '& .MuiInputBase-root': {
                                  paddingRight: '65px !important'
                                }
                              }}
                            />
                          )}
                          renderOption={(props, option) => <li {...props}>{formatCountry(option)}</li>}
                          renderTags={(value, getTagProps) =>
                            value.map((option, index) => {
                              const { key, ...tagProps } = getTagProps({ index });
                              return (
                                <Chip
                                  key={key}
                                  label={formatCountry(option)}
                                  size="small"
                                  sx={{
                                    bgcolor: isDark ? withAlpha(palette.primary.main, 0.12) : undefined,
                                    borderColor: isDark ? withAlpha(palette.primary.main, 0.3) : undefined
                                  }}
                                  {...tagProps}
                                />
                              );
                            })
                          }
                          limitTags={2}
                        />
                      </Grid>
                      <Grid item xs={6} sm={3}>
                        <TextField
                          fullWidth
                          size="small"
                          label={t('subscriptions.form.nodeSelection.nodeSearch')}
                          value={nodeSearchQuery}
                          onChange={(e) => setNodeSearchQuery(e.target.value)}
                        />
                      </Grid>
                    </Grid>

                    <NodeTransferBox
                      availableNodes={availableNodes}
                      selectedNodes={formData.selectedNodes}
                      selectedNodesList={selectedNodesList}
                      chainTargetNodeGroups={chainTargetNodeGroups}
                      checkedAvailable={checkedAvailable}
                      checkedSelected={checkedSelected}
                      selectedNodeSearch={selectedNodeSearch}
                      onSelectedNodeSearchChange={setSelectedNodeSearch}
                      selectedNodeGroupFilter={selectedNodeGroupFilter}
                      onSelectedNodeGroupFilterChange={setSelectedNodeGroupFilter}
                      selectedNodeSourceFilter={selectedNodeSourceFilter}
                      onSelectedNodeSourceFilterChange={setSelectedNodeSourceFilter}
                      selectedNodeFrontOnly={selectedNodeFrontOnly}
                      onSelectedNodeFrontOnlyChange={setSelectedNodeFrontOnly}
                      selectorNodesTotal={selectorNodesCount}
                      selectorNodesLoading={selectorNodesLoading}
                      availableNodePage={availableNodePage}
                      availableNodePageCount={availableNodePageCount}
                      onAvailableNodePageChange={onAvailableNodePageChange}
                      mobileTab={mobileTab}
                      onMobileTabChange={setMobileTab}
                      matchDownMd={matchDownMd}
                      onAddNode={onAddNode}
                      onRemoveNode={onRemoveNode}
                      onAddAllVisible={onAddAllVisible}
                      onRemoveAll={onRemoveAll}
                      onToggleAvailable={onToggleAvailable}
                      onToggleSelected={onToggleSelected}
                      onAddChecked={onAddChecked}
                      onRemoveChecked={onRemoveChecked}
                      onToggleAllAvailable={onToggleAllAvailable}
                      onToggleAllSelected={onToggleAllSelected}
                    />
                    {selectorLoadingText && (
                      <Alert severity="info" variant="outlined" sx={{ mt: 1 }}>
                        {selectorLoadingText}
                      </Alert>
                    )}
                    {assignmentCandidateNodes.length > 0 && (
                      <Box sx={helperPanelSx}>
                        <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
                          <Box>
                            <Typography variant="subtitle2">自定义代理组节点分配</Typography>
                            <Typography variant="caption" sx={{ color: secondaryText }}>
                              {t('subscriptions.form.manualGroupAssignmentHelper')}
                            </Typography>
                          </Box>
                          <Stack direction="row" spacing={1} alignItems="center" sx={{ flexShrink: 0 }}>
                            {formData.enableNodeTemplateAssignments && Object.keys(formData.nodeTemplateAssignments || {}).length > 0 && (
                              <Button
                                size="small"
                                color="inherit"
                                onClick={() => {
                                  closeAssignmentMenu();
                                  setFormData((prev) => ({ ...prev, nodeTemplateAssignments: {} }));
                                }}
                              >
                                清空全部分配
                              </Button>
                            )}
                            <FormControlLabel
                              control={
                                <Switch
                                  checked={Boolean(formData.enableNodeTemplateAssignments)}
                                  onChange={(event) => {
                                    closeAssignmentMenu();
                                    setFormData((prev) => ({
                                      ...prev,
                                      enableNodeTemplateAssignments: event.target.checked
                                    }));
                                  }}
                                />
                              }
                              label={formData.enableNodeTemplateAssignments ? '已启用' : '使用默认'}
                              labelPlacement="start"
                              sx={{ m: 0 }}
                            />
                          </Stack>
                        </Stack>
                        {!formData.enableNodeTemplateAssignments && (
                          <Alert severity="info" variant="outlined" sx={{ mt: 1.25, py: 0.25 }}>
                            当前使用原来的生成方式：节点按模板的 include-all、筛选器和占位符规则自动进入代理组。
                          </Alert>
                        )}
                        {formData.enableNodeTemplateAssignments && (
                          <Alert severity="info" variant="outlined" sx={{ my: 1.5, py: 0.25 }}>
                            新增节点不会自动加入代理组，需要时可在下方按代理组手动选择。已有节点的分配保持不变，自动展开的前置依赖由链式代理管理。
                            前置依赖节点同样出现在候选列表中并标记为「前置依赖」；把前置节点分配到某个代理组后，只有该组会保留它，其它代理组仍会排除前置节点。
                          </Alert>
                        )}
                        {formData.enableNodeTemplateAssignments && (
                          <Stack spacing={1}>
                            {clashProxyGroupMetadata.map((group) => {
                              const groupName = group.name;
                              const assignedNodeIds = orderedAssignedNodeIdsForGroup(groupName);
                              const nodeById = new Map(assignmentCandidateNodes.map((node) => [Number(node.ID), node]));
                              const assignedNodes = assignedNodeIds.map((id) => nodeById.get(id)).filter(Boolean);
                              return (
                                <Stack
                                  key={groupName}
                                  direction={{ xs: 'column', sm: 'row' }}
                                  spacing={1}
                                  alignItems={{ sm: 'center' }}
                                  sx={{
                                    p: 1.25,
                                    border: '1px solid',
                                    borderColor: panelBorder,
                                    borderRadius: 1.5,
                                    bgcolor: mutedPanelSurface
                                  }}
                                >
                                  <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography variant="body2" noWrap sx={{ color: primaryText, fontWeight: 500 }}>
                                      {groupName}
                                      {chainProxyGroupTargets.has(groupName) && (
                                        <Chip
                                          size="small"
                                          color="warning"
                                          variant="outlined"
                                          label={`链式前置 → ${Array.from(chainProxyGroupTargets.get(groupName)).join('、')}`}
                                          sx={{ ml: 0.75, verticalAlign: 'middle' }}
                                        />
                                      )}
                                    </Typography>
                                    <Typography variant="caption" sx={{ color: tertiaryText, display: 'block' }}>
                                      直接分配 {assignedNodeIds.length} / {assignmentCandidateNodes.length} 个节点
                                    </Typography>
                                    {assignedNodes.length > 0 && (
                                      <Stack
                                        direction="row"
                                        spacing={0.5}
                                        useFlexGap
                                        flexWrap="wrap"
                                        sx={{
                                          mt: 0.75,
                                          maxHeight: 76,
                                          overflowY: 'auto',
                                          scrollbarWidth: 'thin',
                                          scrollbarColor: `${withAlpha(palette.text.primary, 0.28)} transparent`,
                                          '&::-webkit-scrollbar': { width: 6 },
                                          '&::-webkit-scrollbar-thumb': {
                                            backgroundColor: withAlpha(palette.text.primary, 0.24),
                                            borderRadius: 999
                                          }
                                        }}
                                      >
                                        {assignedNodes.map((node) => {
                                          const nodeId = Number(node.ID);
                                          const nodeName = node.Name || node.EffectiveName || `Node #${nodeId}`;
                                          return (
                                            <Tooltip key={nodeId} title={nodeName} arrow>
                                              <Chip
                                                size="small"
                                                color="primary"
                                                variant="outlined"
                                                label={nodeName}
                                                sx={{ maxWidth: 180, '& .MuiChip-label': { overflow: 'hidden', textOverflow: 'ellipsis' } }}
                                              />
                                            </Tooltip>
                                          );
                                        })}
                                      </Stack>
                                    )}
                                  </Box>
                                  <Stack direction="row" spacing={1} alignItems="center">
                                    {assignedNodeIds.length > 0 && (
                                      <Chip size="small" color="primary" variant="outlined" label={assignedNodeIds.length} />
                                    )}
                                    <Button
                                      size="small"
                                      variant={assignedNodeIds.length > 0 ? 'contained' : 'outlined'}
                                      onClick={(event) => {
                                        setAssignmentMenuGroup(groupName);
                                        setAssignmentGroupOrder((current) =>
                                          current[groupName] ? current : { ...current, [groupName]: assignedNodeIds }
                                        );
                                        setAssignmentDraftNodeIds(assignedNodeIds);
                                        setAssignmentNodeSearch('');
                                        setAssignmentNodeGroupFilter('all');
                                        setAssignmentNodeSourceFilter('all');
                                        setAssignmentMenuAnchor(event.currentTarget);
                                      }}
                                    >
                                      选择节点
                                    </Button>
                                  </Stack>
                                </Stack>
                              );
                            })}
                          </Stack>
                        )}
                        {formData.enableNodeTemplateAssignments && (
                          <Box sx={{ mt: 2 }}>
                            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
                              <Typography variant="subtitle2" sx={{ color: primaryText }}>
                                {t('subscriptions.form.nodes.selectedList', { count: assignmentCandidateNodes.length })}
                              </Typography>
                              <Chip
                                size="small"
                                variant="outlined"
                                color="success"
                                label={assignmentCandidateNodes.filter((node) => assignedGroupNamesForNode(node.ID).length > 0).length}
                              />
                            </Stack>
                            <Box
                              sx={{
                                maxHeight: 220,
                                overflowY: 'auto',
                                p: 1,
                                border: '1px solid',
                                borderColor: panelBorder,
                                borderRadius: 1.5,
                                bgcolor: mutedPanelSurface
                              }}
                            >
                              {assignmentCandidateNodes.map((node) => {
                                const nodeId = Number(node.ID);
                                const nodeGroups = assignedGroupNamesForNode(nodeId);
                                return (
                                  <Stack
                                    key={nodeId}
                                    direction="row"
                                    spacing={1}
                                    alignItems="center"
                                    sx={{ py: 0.5, px: 0.75, borderRadius: 1, '&:hover': { bgcolor: nestedPanelSurface } }}
                                  >
                                    <Typography variant="body2" noWrap sx={{ color: primaryText, minWidth: 0, flex: '0 1 40%' }}>
                                      {node.Name || node.EffectiveName || `Node #${nodeId}`}
                                    </Typography>
                                    <Stack
                                      direction="row"
                                      spacing={0.5}
                                      alignItems="center"
                                      sx={{ flex: '1 1 60%', flexWrap: 'wrap', rowGap: 0.5 }}
                                    >
                                      {!explicitAssignmentNodeIds.has(nodeId) && (
                                        <Chip
                                          size="small"
                                          variant="outlined"
                                          color="secondary"
                                          label={t('subscriptions.form.nodes.frontDependency')}
                                        />
                                      )}
                                      {nodeGroups.length > 0 ? (
                                        nodeGroups.map((groupName) => (
                                          <Chip
                                            key={groupName}
                                            size="small"
                                            color="primary"
                                            variant="outlined"
                                            label={groupName}
                                            onDelete={() => removeNodeFromGroup(nodeId, groupName)}
                                          />
                                        ))
                                      ) : (
                                        <Typography variant="caption" sx={{ color: tertiaryText }}>
                                          {t('subscriptions.form.nodes.unassigned')}
                                        </Typography>
                                      )}
                                    </Stack>
                                  </Stack>
                                );
                              })}
                              {assignmentCandidateNodes.length === 0 && (
                                <Typography variant="body2" sx={{ p: 2, textAlign: 'center', color: secondaryText }}>
                                  {t('subscriptions.form.nodes.selectedListEmpty')}
                                </Typography>
                              )}
                            </Box>
                          </Box>
                        )}
                        <Popover
                          open={Boolean(formData.enableNodeTemplateAssignments && assignmentMenuGroup && assignmentMenuAnchor)}
                          anchorEl={assignmentMenuAnchor}
                          onClose={closeAssignmentMenu}
                          anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                          transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                          slotProps={{
                            paper: {
                              sx: {
                                mt: 0.75,
                                width: { xs: 360, sm: 520 },
                                maxWidth: 'calc(100vw - 32px)',
                                borderRadius: 2,
                                border: '1px solid',
                                borderColor: panelBorder,
                                boxShadow: theme.shadows[12],
                                overflow: 'hidden'
                              }
                            }
                          }}
                        >
                          <Box sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: panelBorder }}>
                            <Typography variant="subtitle2" noWrap>
                              {assignmentMenuGroup}
                            </Typography>
                            <Typography variant="caption" sx={{ color: secondaryText }}>
                              已选择 {assignmentDraftNodeIds.length} 个节点；节点可以同时属于多个代理组
                            </Typography>
                            <TextField
                              size="small"
                              fullWidth
                              placeholder="搜索节点、分组或来源"
                              value={assignmentNodeSearch}
                              onChange={(event) => setAssignmentNodeSearch(event.target.value)}
                              sx={{ mt: 1.25 }}
                              slotProps={{
                                input: {
                                  startAdornment: (
                                    <InputAdornment position="start">
                                      <SearchIcon fontSize="small" />
                                    </InputAdornment>
                                  )
                                }
                              }}
                            />
                            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ mt: 1 }}>
                              <FormControl size="small" fullWidth>
                                <InputLabel>节点分组</InputLabel>
                                <Select
                                  value={assignmentNodeGroupFilter}
                                  label="节点分组"
                                  onChange={(event) => setAssignmentNodeGroupFilter(event.target.value)}
                                >
                                  <MenuItem value="all">全部分组</MenuItem>
                                  {assignmentNodeGroupOptions.map((group) => (
                                    <MenuItem key={group} value={group}>
                                      {group}
                                    </MenuItem>
                                  ))}
                                </Select>
                              </FormControl>
                              <FormControl size="small" fullWidth>
                                <InputLabel>节点来源</InputLabel>
                                <Select
                                  value={assignmentNodeSourceFilter}
                                  label="节点来源"
                                  onChange={(event) => setAssignmentNodeSourceFilter(event.target.value)}
                                >
                                  <MenuItem value="all">全部来源</MenuItem>
                                  {assignmentNodeSourceOptions.map((source) => (
                                    <MenuItem key={source} value={source}>
                                      {source}
                                    </MenuItem>
                                  ))}
                                </Select>
                              </FormControl>
                            </Stack>
                            <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 1 }}>
                              <Typography variant="caption" sx={{ color: secondaryText, flex: 1 }}>
                                当前显示 {assignmentDisplayNodes.length} / {assignmentCandidateNodes.length} 个节点 · 已选{' '}
                                {assignmentDraftNodeIds.length} 个
                              </Typography>
                              <Button
                                size="small"
                                onClick={() =>
                                  setAssignmentDraftNodeIds((current) =>
                                    Array.from(new Set([...current, ...assignmentFilteredNodes.map((node) => Number(node.ID))]))
                                  )
                                }
                              >
                                全选当前结果
                              </Button>
                              <Button size="small" color="inherit" onClick={() => setAssignmentDraftNodeIds([])}>
                                清空
                              </Button>
                            </Stack>
                          </Box>
                          <Box
                            sx={{
                              maxHeight: 'min(52vh, 420px)',
                              overflowY: 'auto',
                              scrollbarWidth: 'thin',
                              scrollbarColor: `${withAlpha(palette.text.primary, 0.28)} transparent`,
                              '&::-webkit-scrollbar': { width: 7 },
                              '&::-webkit-scrollbar-track': { backgroundColor: 'transparent', margin: '8px 0' },
                              '&::-webkit-scrollbar-thumb': {
                                backgroundColor: withAlpha(palette.text.primary, 0.24),
                                borderRadius: 999,
                                border: '2px solid transparent',
                                backgroundClip: 'content-box'
                              }
                            }}
                          >
                            {assignmentDisplayNodes.map((node) => {
                              const nodeId = Number(node.ID);
                              const checked = assignmentDraftNodeIds.includes(nodeId);
                              return (
                                <MenuItem
                                  key={nodeId}
                                  selected={checked}
                                  onClick={() =>
                                    setAssignmentDraftNodeIds((current) =>
                                      current.includes(nodeId) ? current.filter((id) => id !== nodeId) : [...current, nodeId]
                                    )
                                  }
                                  sx={{ minHeight: 52, py: 0.75 }}
                                >
                                  <Checkbox checked={checked} sx={{ p: 0.75, mr: 1 }} />
                                  <Box sx={{ minWidth: 0, flex: 1 }}>
                                    <Typography variant="body2" noWrap>
                                      {node.Name || node.EffectiveName || `Node #${nodeId}`}
                                    </Typography>
                                    <Stack direction="row" spacing={0.75} alignItems="center">
                                      {node.Group && (
                                        <Typography variant="caption" sx={{ color: tertiaryText }}>
                                          {node.Group}
                                        </Typography>
                                      )}
                                      {!explicitAssignmentNodeIds.has(nodeId) && (
                                        <Chip size="small" variant="outlined" color="secondary" label="前置依赖" />
                                      )}
                                      {node.DialerProxyName && (
                                        <Chip size="small" variant="outlined" color="warning" label={`经前置：${node.DialerProxyName}`} />
                                      )}
                                      {chainTargetNodeGroups.has(nodeId) && (
                                        <Chip
                                          size="small"
                                          variant="outlined"
                                          color="info"
                                          label={`链式落地 · 前置组：${Array.from(chainTargetNodeGroups.get(nodeId)).join('、')}`}
                                        />
                                      )}
                                    </Stack>
                                  </Box>
                                </MenuItem>
                              );
                            })}
                            {assignmentFilteredNodes.length === 0 && (
                              <Typography variant="body2" sx={{ p: 3, textAlign: 'center', color: secondaryText }}>
                                没有匹配的节点
                              </Typography>
                            )}
                          </Box>
                          <Stack
                            direction="row"
                            spacing={1}
                            justifyContent="flex-end"
                            sx={{ px: 2, py: 1.25, borderTop: '1px solid', borderColor: panelBorder, bgcolor: mutedPanelSurface }}
                          >
                            <Button size="small" onClick={closeAssignmentMenu}>
                              取消
                            </Button>
                            <Button size="small" variant="contained" onClick={applyGroupAssignmentDraft}>
                              完成
                            </Button>
                          </Stack>
                        </Popover>
                      </Box>
                    )}
                  </>
                )}
              </Stack>
            </AccordionDetails>
          </Accordion>

          {/* ========== {t('subscriptions.form.sections.nodeFilter')} ========== */}
          <Accordion expanded={expandedPanels.filter} onChange={handlePanelChange('filter')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <FilterListIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.nodeFilter')}
              </Typography>
              {!expandedPanels.filter && filterRulesCount > 0 && (
                <Chip
                  size="small"
                  label={t('subscriptions.form.nodeFilter.activeRules', { count: filterRulesCount })}
                  color="warning"
                  variant="outlined"
                  sx={{ ml: 1 }}
                />
              )}
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <Stack spacing={2.5}>
                {/* Delay and Speed Filter */}
                <Grid container spacing={2}>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label={t('subscriptions.form.nodeFilter.maxDelay')}
                      type="text"
                      inputProps={{ inputMode: 'numeric', pattern: '[0-9]*' }}
                      value={formData.DelayTime}
                      onChange={(e) => {
                        const val = e.target.value;
                        if (val === '' || /^\d+$/.test(val)) {
                          setFormData({ ...formData, DelayTime: val === '' ? '' : Number(val) });
                        }
                      }}
                      onBlur={(e) => {
                        const val = Math.max(0, Number(e.target.value) || 0);
                        setFormData({ ...formData, DelayTime: val });
                      }}
                      InputProps={{ endAdornment: <InputAdornment position="end">ms</InputAdornment> }}
                      helperText={t('subscriptions.form.nodeFilter.maxDelayHelper')}
                    />
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label={t('subscriptions.form.nodeFilter.minSpeed')}
                      type="text"
                      inputProps={{ inputMode: 'numeric', pattern: '[0-9]*\\.?[0-9]*' }}
                      value={formData.MinSpeed}
                      onChange={(e) => {
                        const val = e.target.value;
                        if (val === '' || /^\d*\.?\d*$/.test(val)) {
                          setFormData({ ...formData, MinSpeed: val === '' ? '' : val });
                        }
                      }}
                      onBlur={(e) => {
                        const val = Math.max(0, parseFloat(e.target.value) || 0);
                        setFormData({ ...formData, MinSpeed: val });
                      }}
                      InputProps={{ endAdornment: <InputAdornment position="end">MB/s</InputAdornment> }}
                      helperText={t('subscriptions.form.nodeFilter.minSpeedHelper')}
                    />
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <TextField
                      fullWidth
                      label={t('subscriptions.form.nodeFilter.maxFraudScore')}
                      type="text"
                      inputProps={{ inputMode: 'numeric', pattern: '[0-9]*' }}
                      value={formData.MaxFraudScore}
                      onChange={(e) => {
                        const val = e.target.value;
                        if (val === '' || /^\d+$/.test(val)) {
                          setFormData({ ...formData, MaxFraudScore: val === '' ? '' : Number(val) });
                        }
                      }}
                      onBlur={(e) => {
                        const val = Math.max(0, Number(e.target.value) || 0);
                        setFormData({ ...formData, MaxFraudScore: val });
                      }}
                      helperText={t('subscriptions.form.nodeFilter.maxFraudScoreHelper')}
                    />
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel>{t('subscriptions.form.nodeFilter.validationPolicy')}</InputLabel>
                      <Select
                        value={formData.ValidationPolicy || 'all'}
                        label={t('subscriptions.form.nodeFilter.validationPolicy')}
                        onChange={(e) => setFormData({ ...formData, ValidationPolicy: e.target.value })}
                      >
                        {['all', 'exclude_failed', 'only_success'].map((value) => (
                          <MenuItem key={value} value={value}>
                            {t(`subscriptions.form.nodeFilter.validationPolicyOptions.${value}`)}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    <Typography variant="caption" sx={helperCaptionSx}>
                      {t('subscriptions.form.nodeFilter.validationPolicyHelper')}
                    </Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel>{t('subscriptions.form.nodeFilter.qualityStatus')}</InputLabel>
                      <Select
                        value={formData.QualityStatus || ''}
                        label={t('subscriptions.form.nodeFilter.qualityStatus')}
                        onChange={(e) => setFormData({ ...formData, QualityStatus: e.target.value })}
                      >
                        {QUALITY_STATUS_OPTIONS.map((option) => (
                          <MenuItem key={option.value} value={option.value}>
                            {option.label}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    <Typography variant="caption" sx={helperCaptionSx}>
                      {t('subscriptions.form.nodeFilter.qualityStatusHelper')}
                    </Typography>
                  </Grid>
                  <Grid item xs={12}>
                    <Stack spacing={1.5}>
                      <Typography variant="subtitle2">{t('subscriptions.form.nodeFilter.unlockRules')}</Typography>
                      <Alert severity="info" variant="outlined">
                        {t('subscriptions.form.nodeFilter.unlockRulesDesc')}
                      </Alert>
                      <Grid container spacing={1.5} alignItems="center">
                        <Grid item xs={12} md={4}>
                          <FormControl fullWidth size="small">
                            <InputLabel>{t('subscriptions.form.nodeFilter.ruleRelation')}</InputLabel>
                            <Select
                              value={formData.UnlockRuleMode || 'or'}
                              label={t('subscriptions.form.nodeFilter.ruleRelation')}
                              onChange={(e) => setFormData({ ...formData, UnlockRuleMode: e.target.value })}
                            >
                              {getUnlockRuleModeOptions().map((option) => (
                                <MenuItem key={option.value} value={option.value}>
                                  {option.label}
                                </MenuItem>
                              ))}
                            </Select>
                          </FormControl>
                        </Grid>
                        <Grid item xs={12} md={8}>
                          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {formData.UnlockRuleMode === 'and'
                              ? t('subscriptions.form.nodeFilter.ruleRelationAnd')
                              : t('subscriptions.form.nodeFilter.ruleRelationOr')}
                          </Typography>
                        </Grid>
                      </Grid>
                      {unlockRules.length > 0 ? (
                        unlockRules.map((rule, index) => (
                          <Grid container spacing={1.5} key={`unlock-rule-${index}`} alignItems="flex-start">
                            <Grid item xs={12} md={4}>
                              <Autocomplete
                                options={unlockProviderOptions}
                                value={unlockProviderOptions.find((item) => item.value === rule.provider) || null}
                                onChange={(_, newValue) => updateUnlockRule(index, { provider: newValue?.value || '' })}
                                getOptionLabel={(option) => option?.label || formatUnlockProviderLabel(option?.value || '')}
                                renderOption={(props, option) => (
                                  <li {...props} key={option.value}>
                                    <Box>
                                      <Typography variant="body2">{option.label}</Typography>
                                      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                        {option.description || option.value}
                                      </Typography>
                                    </Box>
                                  </li>
                                )}
                                renderInput={(params) => (
                                  <TextField {...params} label="Provider" helperText={t('subscriptions.form.nodeFilter.providerHelper')} />
                                )}
                              />
                            </Grid>
                            <Grid item xs={12} md={3}>
                              <FormControl fullWidth>
                                <InputLabel>{t('subscriptions.form.nodeFilter.status')}</InputLabel>
                                <Select
                                  value={rule.status || ''}
                                  label={t('subscriptions.form.nodeFilter.status')}
                                  onChange={(e) => updateUnlockRule(index, { status: e.target.value })}
                                >
                                  {getUnlockStatusOptions(true).map((option) => (
                                    <MenuItem key={option.value || 'all'} value={option.value}>
                                      {option.label}
                                    </MenuItem>
                                  ))}
                                </Select>
                              </FormControl>
                            </Grid>
                            <Grid item xs={12} md={4}>
                              <TextField
                                fullWidth
                                label={t('subscriptions.form.nodeFilter.keyword')}
                                value={rule.keyword || ''}
                                onChange={(e) => updateUnlockRule(index, { keyword: e.target.value })}
                                helperText={t('subscriptions.form.nodeFilter.keywordHelper')}
                              />
                            </Grid>
                            <Grid item xs={12} md={1}>
                              <Button
                                fullWidth
                                color="error"
                                variant="outlined"
                                startIcon={<DeleteOutlineIcon />}
                                onClick={() => removeUnlockRule(index)}
                              >
                                {t('subscriptions.form.nodeFilter.delete')}
                              </Button>
                            </Grid>
                          </Grid>
                        ))
                      ) : (
                        <Alert severity="info" variant="outlined">
                          {t('subscriptions.form.nodeFilter.unlockInactive')}
                        </Alert>
                      )}
                      <Box>
                        <Button startIcon={<AddIcon />} variant="outlined" onClick={addUnlockRule}>
                          {t('subscriptions.form.nodeFilter.addUnlockRule')}
                        </Button>
                      </Box>
                    </Stack>
                  </Grid>
                </Grid>

                {/* Landing IP Country Filter */}
                <Grid container spacing={2}>
                  <Grid item xs={12} sm={6}>
                    <Autocomplete
                      multiple
                      freeSolo
                      options={normalizedCountryOptions}
                      value={normalizedCountryWhitelist}
                      inputValue={countryWhitelistInput}
                      onInputChange={(event, newInputValue) => {
                        void event;
                        setCountryWhitelistInput(newInputValue);
                      }}
                      onChange={(event, newValue) => {
                        void event;
                        updateCountryFilterField('CountryWhitelist', newValue);
                      }}
                      getOptionLabel={(option) => formatCountry(normalizeCountryCode(option))}
                      isOptionEqualToValue={(option, value) => normalizeCountryCode(option) === normalizeCountryCode(value)}
                      renderInput={(params) => (
                        <TextField
                          {...params}
                          label={t('subscriptions.form.nodeFilter.countryWhitelist')}
                          helperText={t('subscriptions.form.nodeFilter.countryWhitelistHelper')}
                          onKeyDown={(event) =>
                            handleCountryFilterKeyDown(event, 'CountryWhitelist', countryWhitelistInput, setCountryWhitelistInput)
                          }
                        />
                      )}
                      renderOption={(props, option) => <li {...props}>{formatCountry(normalizeCountryCode(option))}</li>}
                    />
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <Autocomplete
                      multiple
                      freeSolo
                      options={normalizedCountryOptions}
                      value={normalizedCountryBlacklist}
                      inputValue={countryBlacklistInput}
                      onInputChange={(event, newInputValue) => {
                        void event;
                        setCountryBlacklistInput(newInputValue);
                      }}
                      onChange={(event, newValue) => {
                        void event;
                        updateCountryFilterField('CountryBlacklist', newValue);
                      }}
                      getOptionLabel={(option) => formatCountry(normalizeCountryCode(option))}
                      isOptionEqualToValue={(option, value) => normalizeCountryCode(option) === normalizeCountryCode(value)}
                      renderInput={(params) => (
                        <TextField
                          {...params}
                          label={t('subscriptions.form.nodeFilter.countryBlacklist')}
                          helperText={t('subscriptions.form.nodeFilter.countryWhitelistHelper')}
                          onKeyDown={(event) =>
                            handleCountryFilterKeyDown(event, 'CountryBlacklist', countryBlacklistInput, setCountryBlacklistInput)
                          }
                        />
                      )}
                      renderOption={(props, option) => <li {...props}>{formatCountry(normalizeCountryCode(option))}</li>}
                    />
                  </Grid>
                </Grid>

                <Grid container spacing={2}>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel>{t('subscriptions.form.nodeFilter.residential')}</InputLabel>
                      <Select
                        value={formData.ResidentialType || ''}
                        label={t('subscriptions.form.nodeFilter.residential')}
                        onChange={(e) => setFormData({ ...formData, ResidentialType: e.target.value })}
                      >
                        <MenuItem value="">{t('subscriptions.form.nodeFilter.all')}</MenuItem>
                        <MenuItem value="residential">{t('subscriptions.form.nodeFilter.residentialIp')}</MenuItem>
                        <MenuItem value="datacenter">{t('subscriptions.form.nodeFilter.datacenterIp')}</MenuItem>
                        <MenuItem value="untested">{t('subscriptions.form.nodeFilter.untested')}</MenuItem>
                      </Select>
                    </FormControl>
                    <Typography variant="caption" sx={helperCaptionSx}>
                      {t('subscriptions.form.nodeFilter.residentialHelper')}
                    </Typography>
                  </Grid>
                  <Grid item xs={12} sm={6}>
                    <FormControl fullWidth>
                      <InputLabel>{t('subscriptions.form.nodeFilter.ipType')}</InputLabel>
                      <Select
                        value={formData.IPType || ''}
                        label={t('subscriptions.form.nodeFilter.ipType')}
                        onChange={(e) => setFormData({ ...formData, IPType: e.target.value })}
                      >
                        <MenuItem value="">{t('subscriptions.form.nodeFilter.all')}</MenuItem>
                        <MenuItem value="native">{t('subscriptions.form.nodeFilter.nativeIp')}</MenuItem>
                        <MenuItem value="broadcast">{t('subscriptions.form.nodeFilter.broadcastIp')}</MenuItem>
                        <MenuItem value="untested">{t('subscriptions.form.nodeFilter.untested')}</MenuItem>
                      </Select>
                    </FormControl>
                    <Typography variant="caption" sx={helperCaptionSx}>
                      {t('subscriptions.form.nodeFilter.ipTypeHelper')}
                    </Typography>
                  </Grid>
                </Grid>

                {/* Node Tag Filter */}
                <NodeTagFilter
                  tagOptions={tagOptions}
                  whitelistValue={formData.tagWhitelist}
                  blacklistValue={formData.tagBlacklist}
                  onWhitelistChange={(tags) => setFormData({ ...formData, tagWhitelist: tags })}
                  onBlacklistChange={(tags) => setFormData({ ...formData, tagBlacklist: tags })}
                />

                {/* Protocol Type Filter */}
                <NodeProtocolFilter
                  protocolOptions={formData.protocolOptions || []}
                  whitelistValue={formData.protocolWhitelist}
                  blacklistValue={formData.protocolBlacklist}
                  onWhitelistChange={(protocols) => setFormData({ ...formData, protocolWhitelist: protocols })}
                  onBlacklistChange={(protocols) => setFormData({ ...formData, protocolBlacklist: protocols })}
                />

                {/* Node Name Filter */}
                <NodeNameFilter
                  whitelistValue={formData.nodeNameWhitelist}
                  blacklistValue={formData.nodeNameBlacklist}
                  onWhitelistChange={(rules) => setFormData({ ...formData, nodeNameWhitelist: rules })}
                  onBlacklistChange={(rules) => setFormData({ ...formData, nodeNameBlacklist: rules })}
                />
              </Stack>
            </AccordionDetails>
          </Accordion>

          {/* ========== {t('subscriptions.form.sections.nodeDeduplication')} ========== */}
          <Accordion expanded={expandedPanels.dedup} onChange={handlePanelChange('dedup')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <FilterAltIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.nodeDeduplication')}
                <Chip size="small" label="Beta" color="error" variant="outlined" sx={{ ml: 1 }} />
              </Typography>
              {!expandedPanels.dedup && formData.deduplicationRule && (
                <Chip
                  size="small"
                  label={t('subscriptions.form.nodeDeduplication.configured')}
                  color="success"
                  variant="outlined"
                  sx={{ ml: 1 }}
                />
              )}
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <DeduplicationConfig
                value={formData.deduplicationRule || ''}
                onChange={(rule) => setFormData({ ...formData, deduplicationRule: rule })}
              />
            </AccordionDetails>
          </Accordion>

          {/* ========== Name Processing ========== */}
          <Accordion expanded={expandedPanels.naming} onChange={handlePanelChange('naming')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <TextFieldsIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.nameProcessing')}
              </Typography>
              {!expandedPanels.naming && (hasPreprocessRules(formData.nodeNamePreprocess) || formData.nodeNameRule) && (
                <Chip
                  size="small"
                  label={t('subscriptions.form.nodeDeduplication.configured')}
                  color="info"
                  variant="outlined"
                  sx={{ ml: 1 }}
                />
              )}
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <Stack spacing={2.5}>
                {/* Original Name Preprocessing */}
                <NodeNamePreprocessor
                  value={formData.nodeNamePreprocess}
                  onChange={(rules) => setFormData({ ...formData, nodeNamePreprocess: rules })}
                />

                {/* {t('subscriptions.form.nameProcessing.namingRule')} */}
                <Box>
                  <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 2 }}>
                    <Typography variant="subtitle1" fontWeight="bold">
                      {t('subscriptions.form.nameProcessing.namingRule')}
                    </Typography>
                    <ButtonGroup size="small" variant="outlined">
                      <Tooltip title={t('subscriptions.form.nameProcessing.builderTooltip')}>
                        <Button
                          onClick={() => setNamingMode('builder')}
                          variant={namingMode === 'builder' ? 'contained' : 'outlined'}
                          startIcon={<BuildIcon />}
                        >
                          {matchDownMd ? '' : t('subscriptions.form.nameProcessing.builder')}
                        </Button>
                      </Tooltip>
                      <Tooltip title={t('subscriptions.form.nameProcessing.manualTooltip')}>
                        <Button
                          onClick={() => setNamingMode('manual')}
                          variant={namingMode === 'manual' ? 'contained' : 'outlined'}
                          startIcon={<EditNoteIcon />}
                        >
                          {matchDownMd ? '' : t('subscriptions.form.nameProcessing.manual')}
                        </Button>
                      </Tooltip>
                    </ButtonGroup>
                  </Stack>

                  {namingMode === 'builder' ? (
                    <NodeRenameBuilder
                      value={formData.nodeNameRule}
                      onChange={(rule) => setFormData({ ...formData, nodeNameRule: rule })}
                    />
                  ) : (
                    <>
                      <TextField
                        fullWidth
                        label={t('subscriptions.form.nameProcessing.templateLabel')}
                        value={formData.nodeNameRule}
                        onChange={(e) => setFormData({ ...formData, nodeNameRule: e.target.value })}
                        placeholder={t('subscriptions.form.nameProcessing.templatePlaceholder')}
                        helperText={t('subscriptions.form.nameProcessing.templateHelper')}
                      />
                      <Box sx={helperPanelSx}>
                        <Typography variant="caption" sx={{ color: tertiaryText }} component="div">
                          <strong>{t('subscriptions.form.nameProcessing.variables')}</strong>
                          <br />• <code>$Name</code> - {t('subscriptions.form.nameProcessing.varName')} &nbsp;&nbsp; •{' '}
                          <code>$LinkName</code> - {t('subscriptions.form.nameProcessing.varLinkName')}
                          <br />• <code>$LinkCountry</code> - {t('subscriptions.form.nameProcessing.varLinkCountry')} &nbsp;&nbsp; •{' '}
                          <code>$Speed</code> - {t('subscriptions.form.nameProcessing.varSpeed')}
                          <br />• <code>$SpeedIcon</code> - {t('subscriptions.form.nameProcessing.varSpeedIcon')} &nbsp;&nbsp; •{' '}
                          <code>$Delay</code> - {t('subscriptions.form.nameProcessing.varDelay')}
                          <br />• <code>$DelayIcon</code> - {t('subscriptions.form.nameProcessing.varDelayIcon')} &nbsp;&nbsp; •{' '}
                          <code>$Group</code> - {t('subscriptions.form.nameProcessing.varGroup')}
                          <br />• <code>$Source</code> - {t('subscriptions.form.nameProcessing.varSource')} &nbsp;&nbsp; •{' '}
                          <code>$Index</code> - {t('subscriptions.form.nameProcessing.varIndex')}
                          <br />• <code>$DuplicateIndex</code> - {t('subscriptions.form.nameProcessing.varDuplicateIndex')} &nbsp;&nbsp; •{' '}
                          <code>$Protocol</code> - {t('subscriptions.form.nameProcessing.varProtocol')}
                          <br />• <code>$IpType</code> - {t('subscriptions.form.nameProcessing.varIpType')} &nbsp;&nbsp; •{' '}
                          <code>$Residential</code> -{t('subscriptions.form.nameProcessing.varResidential')}
                          <br />• <code>$FraudScore</code> - {t('subscriptions.form.nameProcessing.varFraudScore')} &nbsp;&nbsp; •{' '}
                          <code>$FraudScoreIcon</code> -{t('subscriptions.form.nameProcessing.varFraudScoreIcon')}
                          {unlockRenameVariables.length > 0 && (
                            <>
                              <br />•{' '}
                              {unlockRenameVariables.map((item, index) => (
                                <span key={item.key}>
                                  <code>{item.key}</code> - {item.label}
                                  {index < unlockRenameVariables.length - 1 ? '; ' : ''}
                                </span>
                              ))}
                            </>
                          )}
                          <br />• <code>$Tags</code> - {t('subscriptions.form.nameProcessing.varTags')} &nbsp;&nbsp; •{' '}
                          <code>$TagGroup(name)</code> - {t('subscriptions.form.nameProcessing.varTagGroup')}
                        </Typography>
                      </Box>
                      {formData.nodeNameRule && (
                        <Alert variant={'standard'} severity="info" sx={{ mt: 1 }}>
                          <Typography variant="body2">
                            <strong>{t('subscriptions.form.nameProcessing.preview')}</strong>{' '}
                            {previewSubscriptionNodeName(formData.nodeNameRule, {
                              speedIcon: getSpeedIcon(1.5, 'success'),
                              delayIcon: getDelayIcon(125, 'success'),
                              fraudScoreIcon: getFraudScoreIcon(12, 'success')
                            })}
                          </Typography>
                        </Alert>
                      )}
                    </>
                  )}
                </Box>
              </Stack>
            </AccordionDetails>
          </Accordion>

          {/* ========== {t('subscriptions.form.sections.advanced')} ========== */}
          <Accordion expanded={expandedPanels.advanced} onChange={handlePanelChange('advanced')} sx={accordionSx}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={accordionSummarySx}>
              <SecurityIcon color="primary" />
              <Typography variant="subtitle1" fontWeight={600}>
                {t('subscriptions.form.sections.advanced')}
              </Typography>
              {!expandedPanels.advanced && advancedSettingsCount > 0 && (
                <Chip
                  size="small"
                  label={t('subscriptions.form.advanced.configured', { count: advancedSettingsCount })}
                  color="secondary"
                  variant="outlined"
                  sx={{ ml: 1 }}
                />
              )}
            </AccordionSummary>
            <AccordionDetails sx={accordionDetailsSx}>
              <Stack spacing={2.5}>
                {/* Script Selection */}
                <Autocomplete
                  multiple
                  options={scripts}
                  getOptionLabel={(option) => `${option.name} (${option.version})`}
                  value={scripts.filter((s) => formData.selectedScripts.includes(s.id))}
                  onChange={(e, newValue) => setFormData({ ...formData, selectedScripts: newValue.map((s) => s.id) })}
                  sx={autocompleteChipSx}
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      label={t('subscriptions.form.advanced.scriptLabel')}
                      helperText={t('subscriptions.form.advanced.scriptHelper')}
                    />
                  )}
                  renderOption={(props, option) => (
                    <li {...props}>
                      <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Typography variant="body1">{option.name}</Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                          {t('subscriptions.form.advanced.scriptVersion', { version: option.version })}
                        </Typography>
                      </Box>
                    </li>
                  )}
                />

                {/* IP Whitelist/Blacklist */}
                <TextField
                  fullWidth
                  label={t('subscriptions.form.advanced.ipBlacklist')}
                  multiline
                  rows={2}
                  value={formData.IPBlacklist}
                  onChange={(e) => setFormData({ ...formData, IPBlacklist: e.target.value })}
                  helperText={t('subscriptions.form.advanced.ipBlacklistHelper')}
                />
                <TextField
                  fullWidth
                  label={t('subscriptions.form.advanced.ipWhitelist')}
                  multiline
                  rows={2}
                  value={formData.IPWhitelist}
                  onChange={(e) => setFormData({ ...formData, IPWhitelist: e.target.value })}
                  helperText={t('subscriptions.form.advanced.ipBlacklistHelper')}
                />
              </Stack>
            </AccordionDetails>
          </Accordion>
        </Box>
      </DialogContent>
      <DialogActions
        sx={{
          px: matchDownMd ? 2 : 3,
          py: 1.5,
          bgcolor: mutedPanelSurface,
          borderTop: '1px solid',
          borderColor: panelBorder
        }}
      >
        <Stack direction="row" spacing={2} sx={{ width: '100%', justifyContent: 'space-between' }}>
          <Button
            variant="outlined"
            startIcon={<VisibilityIcon />}
            onClick={onPreview}
            disabled={
              previewLoading ||
              (formData.selectedNodes.length === 0 && formData.selectedGroups.length === 0 && formData.selectedAirports.length === 0)
            }
          >
            {previewLoading ? t('subscriptions.form.actions.previewLoading') : t('subscriptions.form.actions.previewNode')}
            <Chip size="small" label="Beta" color="error" variant="outlined" sx={{ ml: 1 }} />
          </Button>
          <Stack direction="row" spacing={1}>
            <Button onClick={onClose}>{t('subscriptions.form.actions.close')}</Button>
            <Button variant="contained" onClick={onSubmit}>
              {t('subscriptions.form.actions.confirm')}
            </Button>
          </Stack>
        </Stack>
      </DialogActions>
    </Dialog>
  );
}
