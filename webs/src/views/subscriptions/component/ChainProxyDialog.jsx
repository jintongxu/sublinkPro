import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import InputAdornment from '@mui/material/InputAdornment';
import Paper from '@mui/material/Paper';
import Chip from '@mui/material/Chip';
import Switch from '@mui/material/Switch';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';

import CloseIcon from '@mui/icons-material/Close';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import DragIndicatorIcon from '@mui/icons-material/DragIndicator';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import TouchAppIcon from '@mui/icons-material/TouchApp';
import VisibilityIcon from '@mui/icons-material/Visibility';
import SearchIcon from '@mui/icons-material/Search';
import FilterListIcon from '@mui/icons-material/FilterList';

import { DragDropContext, Droppable, Draggable } from '@hello-pangea/dnd';
import {
  getChainRules,
  createChainRule,
  updateChainRule,
  deleteChainRule,
  toggleChainRule,
  sortChainRules,
  getChainOptions,
  previewChainLinks
} from '../../../api/subscriptions';
import ChainPreviewDialog from './ChainPreviewDialog';
import ChainRuleEditor from './ChainRuleEditor';
import { getChainProxyIconButtonSx, getChainProxyThemeTokens } from './chainProxyTheme';

export default function ChainProxyDialog({ open, onClose, subscription }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const { isDark } = useResolvedColorScheme();
  const tokens = getChainProxyThemeTokens(theme, isDark);
  const {
    dialogSurface,
    dialogSurfaceGradient,
    mutedPanelSurface,
    elevatedSurface,
    panelBorder,
    softBorder,
    selectedSurface,
    primaryStrongBorder,
    primaryText,
    secondaryText,
    tertiaryText,
    infoSurface,
    infoSoftBorder,
    cardShadow
  } = tokens;
  const iconButtonSx = getChainProxyIconButtonSx(tokens);
  const errorIconButtonSx = getChainProxyIconButtonSx(tokens, theme.palette.error.main);

  const [loading, setLoading] = useState(false);
  const [rules, setRules] = useState([]);
  const [options, setOptions] = useState({ nodes: [], conditionFields: [], operators: [], groupTypes: [], templateGroups: [] });
  const [editingRule, setEditingRule] = useState(null);
  const [editMode, setEditMode] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewData, setPreviewData] = useState(null);
  const [errorMessage, setErrorMessage] = useState('');
  const [ruleSearch, setRuleSearch] = useState('');
  const [ruleStatus, setRuleStatus] = useState('all');

  const getErrorMessage = (error, fallback) =>
    error?.response?.data?.error || error?.response?.data?.msg || error?.message || fallback;

  const loadData = useCallback(async () => {
    if (!subscription?.ID) return;
    setLoading(true);
    setErrorMessage('');
    try {
      const [rulesRes, optionsRes] = await Promise.all([getChainRules(subscription.ID), getChainOptions(subscription.ID)]);
      setRules(rulesRes?.data || []);
      setOptions(optionsRes?.data || { nodes: [], conditionFields: [], operators: [], groupTypes: [], templateGroups: [] });
    } catch (err) {
      console.error('Failed to load chain proxy data:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.loadFailed', '加载链式代理配置失败')));
    } finally {
      setLoading(false);
    }
  }, [subscription?.ID]);

  useEffect(() => {
    if (open && subscription?.ID) {
      loadData();
    }
  }, [open, subscription?.ID, loadData]);

  const handleAdd = () => {
    setEditingRule({
      name: '',
      enabled: true,
      chainConfig: '[]',
      targetConfig: '{"type":"specified_node"}'
    });
    setEditMode(true);
  };

  const handleEdit = (rule) => {
    setEditingRule(rule);
    setEditMode(true);
  };

  const handleSave = async () => {
    if (!editingRule) return;

    setErrorMessage('');
    setLoading(true);
    try {
      if (editingRule.id) {
        await updateChainRule(subscription.ID, editingRule.id, editingRule);
      } else {
        await createChainRule(subscription.ID, editingRule);
      }
      await loadData();
      setEditMode(false);
      setEditingRule(null);
    } catch (err) {
      console.error('Failed to save chain rule:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.saveFailed', '保存链式代理规则失败')));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (rule) => {
    if (!window.confirm(t('subscriptions.chain.confirmDeleteRule', { name: rule.name || t('subscriptions.chain.unnamedRule') }))) return;

    setLoading(true);
    try {
      await deleteChainRule(subscription.ID, rule.id);
      await loadData();
    } catch (err) {
      console.error('Failed to delete chain rule:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.deleteFailed', '删除链式代理规则失败')));
    } finally {
      setLoading(false);
    }
  };

  const handleToggle = async (rule) => {
    try {
      await toggleChainRule(subscription.ID, rule.id);
      await loadData();
    } catch (err) {
      console.error('Failed to toggle chain rule status:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.toggleFailed', '切换链式代理规则状态失败')));
    }
  };

  const handleDragEnd = async (result) => {
    if (!result.destination) return;

    const sourceRule = filteredRules[result.source.index];
    const destinationRule = filteredRules[result.destination.index];
    if (!sourceRule || !destinationRule || sourceRule.id === destinationRule.id) return;

    const items = Array.from(rules);
    const sourceIndex = items.findIndex((rule) => rule.id === sourceRule.id);
    const destinationIndex = items.findIndex((rule) => rule.id === destinationRule.id);
    const [reorderedItem] = items.splice(sourceIndex, 1);
    items.splice(destinationIndex, 0, reorderedItem);
    setRules(items);

    try {
      await sortChainRules(
        subscription.ID,
        items.map((r) => r.id)
      );
    } catch (err) {
      console.error('Failed to save chain rule order:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.sortFailed', '保存链式代理顺序失败')));
      await loadData();
    }
  };

  const handleRuleChange = (data) => {
    setEditingRule({ ...editingRule, ...data });
  };

  const handleBack = () => {
    setEditMode(false);
    setEditingRule(null);
  };

  const handlePreview = async () => {
    setPreviewOpen(true);
    setPreviewLoading(true);
    try {
      const res = await previewChainLinks(subscription.ID);
      setPreviewData(res?.data || null);
    } catch (err) {
      console.error('Failed to preview chain links:', err);
      setErrorMessage(getErrorMessage(err, t('subscriptions.chain.previewFailed', '预览链式代理失败')));
      setPreviewData(null);
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleClosePreview = () => {
    setPreviewOpen(false);
    setPreviewData(null);
  };

  const getTypeFriendlyName = (type) => {
    const labels = t('subscriptions.chain.proxyTypeShort', { returnObjects: true });
    return labels[type] || type;
  };

  const parseChainConfig = (configStr) => {
    try {
      const config = JSON.parse(configStr || '[]');
      return config
        .map((item) => {
          if (item.type === 'specified_node') {
            if (item.nodeId) {
              const node = (options.nodes || []).find((n) => n.id === item.nodeId);
              if (node) {
                return node.name || node.linkName || t('subscriptions.chain.nodeNumber', { id: item.nodeId });
              }
              return t('subscriptions.chain.nodeNumber', { id: item.nodeId });
            }
            return t('subscriptions.chain.proxyTypes.specified_node');
          }
          if (item.type === 'dynamic_node') {
            return t('subscriptions.chain.proxyTypeShort.dynamic_node');
          }
          if (item.type === 'custom_group' || item.type === 'template_group') {
            return item.groupName || getTypeFriendlyName(item.type);
          }
          return item.groupName || getTypeFriendlyName(item.type);
        })
        .filter(Boolean);
    } catch {
      return [];
    }
  };

  const parseTargetConfig = (configStr) => {
    try {
      const config = JSON.parse(configStr || '{}');
      if (config.type === 'all') return t('subscriptions.chain.targetTypes.all');
      if (config.type === 'specified_node') {
        if (config.nodeId) {
          const node = (options.nodes || []).find((n) => n.id === config.nodeId);
          if (node) {
            return node.name || node.linkName || t('subscriptions.chain.nodeNumber', { id: config.nodeId });
          }
          return t('subscriptions.chain.nodeNumber', { id: config.nodeId });
        }
        return t('subscriptions.chain.noNodeSelected');
      }
      if (config.type === 'conditions' && config.conditions?.conditions?.length > 0) {
        return t('subscriptions.chain.conditionCount', { count: config.conditions.conditions.length });
      }
      return t('subscriptions.chain.unconfigured');
    } catch {
      return t('subscriptions.chain.unconfigured');
    }
  };

  const enabledRuleCount = rules.filter((rule) => rule.enabled !== false).length;
  const filteredRules = rules.filter((rule) => {
    const isEnabled = rule.enabled !== false;
    if (ruleStatus === 'enabled' && !isEnabled) return false;
    if (ruleStatus === 'disabled' && isEnabled) return false;
    const keyword = ruleSearch.trim().toLowerCase();
    if (!keyword) return true;
    const chainNames = parseChainConfig(rule.chainConfig).join(' ');
    const targetName = parseTargetConfig(rule.targetConfig);
    return [rule.name, chainNames, targetName, rule.chainConfig, rule.targetConfig]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
      .includes(keyword);
  });

  return (
    <>
      <Dialog
        open={open}
        onClose={onClose}
        maxWidth="lg"
        fullWidth
        fullScreen={isMobile}
        slotProps={{
          paper: {
            sx: isMobile
              ? {
                  borderRadius: 0,
                  border: '1px solid',
                  borderColor: panelBorder,
                  bgcolor: dialogSurface,
                  backgroundImage: dialogSurfaceGradient
                }
              : {
                  minHeight: '80vh',
                  borderRadius: 2,
                  border: '1px solid',
                  borderColor: panelBorder,
                  bgcolor: dialogSurface,
                  backgroundImage: dialogSurfaceGradient
                }
          }
        }}
      >
        <DialogTitle sx={{ pb: 1.5, bgcolor: mutedPanelSurface, borderBottom: '1px solid', borderColor: panelBorder }}>
          <Stack direction="row" alignItems="center" justifyContent="space-between">
            <Stack direction="row" alignItems="center" spacing={1}>
              <AccountTreeIcon color="primary" />
              <Box>
                <Typography variant={isMobile ? 'subtitle1' : 'h6'} fontWeight={600} sx={{ color: primaryText }}>
                  {t('subscriptions.chain.dialogTitle')}
                  <Chip size="small" label="Beta" color="error" variant="outlined" sx={{ ml: 1 }} />
                </Typography>
                {isMobile && (
                  <Typography variant="caption" sx={{ color: secondaryText }}>
                    {subscription?.Name}
                  </Typography>
                )}
                {!isMobile && (
                  <Typography variant="body2" sx={{ mt: 0.25, color: secondaryText }}>
                    {subscription?.Name}
                  </Typography>
                )}
              </Box>
            </Stack>
            <IconButton onClick={onClose} size="small" sx={iconButtonSx}>
              <CloseIcon />
            </IconButton>
          </Stack>
        </DialogTitle>

        <DialogContent dividers sx={{ p: isMobile ? 2 : 3, bgcolor: dialogSurface, borderColor: panelBorder }}>
          {errorMessage && (
            <Alert severity="error" sx={{ mb: 2 }} onClose={() => setErrorMessage('')}>
              {errorMessage}
            </Alert>
          )}
          {loading && (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <CircularProgress />
            </Box>
          )}

          {!loading && !editMode && (
            <Box>
              <Paper
                variant="outlined"
                sx={{
                  mb: 2,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: selectedSurface,
                  borderColor: primaryStrongBorder,
                  boxShadow: cardShadow
                }}
              >
                <Stack direction={isMobile ? 'column' : 'row'} spacing={2} alignItems={isMobile ? 'stretch' : 'center'} justifyContent="space-between">
                  <Box>
                    <Typography variant="overline" sx={{ color: secondaryText, letterSpacing: 0.5 }}>
                      {t('subscriptions.chain.currentSubscription')}
                    </Typography>
                    <Typography variant="subtitle1" fontWeight={700} sx={{ color: primaryText }} noWrap>
                      {subscription?.Name || t('subscriptions.chain.unnamedSubscription')}
                    </Typography>
                  </Box>
                  <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                    <Chip
                      size="small"
                      color={enabledRuleCount > 0 ? 'success' : 'default'}
                      label={enabledRuleCount > 0 ? t('subscriptions.chain.enabled') : t('subscriptions.chain.notEnabled')}
                    />
                    <Chip size="small" variant="outlined" label={t('subscriptions.chain.ruleSummary', { total: rules.length, enabled: enabledRuleCount })} />
                  </Stack>
                </Stack>
              </Paper>

              <Box
                sx={{
                  mb: 2,
                  p: 1.5,
                  borderRadius: 2,
                  bgcolor: elevatedSurface,
                  border: '1px solid',
                  borderColor: softBorder,
                  boxShadow: cardShadow
                }}
              >
                <Typography variant="body2" sx={{ mb: 1, color: secondaryText }}>
                  {t('subscriptions.chain.description')}
                </Typography>
                <Typography
                  variant="caption"
                  sx={{
                    display: 'block',
                    p: 1.25,
                    bgcolor: infoSurface,
                    color: tertiaryText,
                    borderRadius: 1.5,
                    border: '1px solid',
                    borderColor: infoSoftBorder
                  }}
                >
                  {t('subscriptions.chain.overlapHint')}
                </Typography>
              </Box>

              <Stack direction={isMobile ? 'column' : 'row'} spacing={1} sx={{ mb: 2 }} alignItems={isMobile ? 'stretch' : 'center'}>
                <TextField
                  size="small"
                  fullWidth
                  value={ruleSearch}
                  onChange={(event) => setRuleSearch(event.target.value)}
                  placeholder={t('subscriptions.chain.searchRules')}
                  aria-label={t('subscriptions.chain.searchRules')}
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <SearchIcon fontSize="small" />
                      </InputAdornment>
                    )
                  }}
                />
                <Stack direction="row" spacing={0.5} alignItems="center" sx={{ flexShrink: 0 }}>
                  <FilterListIcon fontSize="small" sx={{ color: secondaryText }} />
                  {['all', 'enabled', 'disabled'].map((status) => (
                    <Chip
                      key={status}
                      size="small"
                      clickable
                      color={ruleStatus === status ? 'primary' : 'default'}
                      variant={ruleStatus === status ? 'filled' : 'outlined'}
                      label={t(`subscriptions.chain.filter.${status}`)}
                      onClick={() => setRuleStatus(status)}
                    />
                  ))}
                </Stack>
              </Stack>

              {isMobile ? (
                <Stack spacing={1.5}>
                  {filteredRules.map((rule) => (
                    <Card
                      key={rule.id}
                      variant="outlined"
                      sx={{
                        borderRadius: 2,
                        opacity: rule.enabled ? 1 : 0.6,
                        transition: 'all 0.2s ease',
                        bgcolor: elevatedSurface,
                        borderColor: softBorder,
                        boxShadow: cardShadow,
                        '&:active': { transform: 'scale(0.98)' }
                      }}
                    >
                      <CardContent sx={{ py: 1.5, px: 2, '&:last-child': { pb: 1.5 } }}>
                        <Stack spacing={1.5}>
                          <Stack direction="row" alignItems="center" justifyContent="space-between">
                            <Stack direction="row" alignItems="center" spacing={1}>
                              <Typography variant="subtitle1" fontWeight={600} sx={{ color: primaryText }}>
                                {rule.name || t('subscriptions.chain.unnamedRule')}
                              </Typography>
                              {!rule.enabled && <Chip label={t('common.disabled')} size="small" color="default" />}
                            </Stack>
                            <Switch checked={rule.enabled} onChange={() => handleToggle(rule)} size="small" />
                          </Stack>

                          <Stack direction="row" alignItems="center" spacing={0.5} flexWrap="wrap" sx={{ gap: 0.5 }}>
                            {parseChainConfig(rule.chainConfig).map((name, i) => (
                              <Chip key={i} label={name} size="small" color="primary" variant="outlined" sx={{ borderRadius: 1.5 }} />
                            ))}
                            <ArrowForwardIcon sx={{ fontSize: 16, color: secondaryText, mx: 0.5 }} />
                            <Typography variant="body2" sx={{ color: secondaryText }}>
                              {parseTargetConfig(rule.targetConfig)}
                            </Typography>
                          </Stack>

                          <Stack direction="row" spacing={1} justifyContent="flex-end">
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<EditIcon />}
                              onClick={() => handleEdit(rule)}
                              sx={{ minWidth: 80, borderColor: primaryStrongBorder }}
                            >
                              {t('common.edit')}
                            </Button>
                            <Button
                              size="small"
                              variant="outlined"
                              color="error"
                              startIcon={<DeleteIcon />}
                              onClick={() => handleDelete(rule)}
                              sx={{ minWidth: 80, borderColor: tokens.errorSoftBorder }}
                            >
                              {t('common.delete')}
                            </Button>
                          </Stack>
                        </Stack>
                      </CardContent>
                    </Card>
                  ))}
                </Stack>
              ) : (
                <DragDropContext onDragEnd={handleDragEnd}>
                  <Droppable droppableId="chain-rules">
                    {(provided) => (
                      <Stack spacing={1} {...provided.droppableProps} ref={provided.innerRef}>
                        {filteredRules.map((rule, index) => (
                          <Draggable key={rule.id} draggableId={String(rule.id)} index={index}>
                            {(provided, snapshot) => (
                              <Paper
                                ref={provided.innerRef}
                                {...provided.draggableProps}
                                variant="outlined"
                                sx={{
                                  p: 2,
                                  backgroundColor: snapshot.isDragging ? selectedSurface : elevatedSurface,
                                  opacity: rule.enabled ? 1 : 0.6,
                                  borderRadius: 2,
                                  boxShadow: cardShadow,
                                  borderColor: snapshot.isDragging ? primaryStrongBorder : softBorder
                                }}
                              >
                                <Stack direction="row" alignItems="center" spacing={2}>
                                  <Box {...provided.dragHandleProps} sx={{ cursor: 'grab', color: secondaryText }}>
                                    <DragIndicatorIcon />
                                  </Box>

                                  <Box sx={{ flex: 1 }}>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                      <Typography variant="subtitle2" sx={{ color: primaryText }}>
                                        {rule.name || t('subscriptions.chain.unnamedRule')}
                                      </Typography>
                                      {!rule.enabled && <Chip label={t('common.disabled')} size="small" color="default" />}
                                    </Stack>
                                    <Stack direction="row" spacing={1} sx={{ mt: 0.5 }}>
                                      {parseChainConfig(rule.chainConfig).map((name, i) => (
                                        <Chip key={i} label={name} size="small" color="primary" variant="outlined" />
                                      ))}
                                      <Typography variant="caption" sx={{ color: secondaryText }}>
                                        → {parseTargetConfig(rule.targetConfig)}
                                      </Typography>
                                    </Stack>
                                  </Box>

                                  <Switch checked={rule.enabled} onChange={() => handleToggle(rule)} size="small" />
                                  <IconButton size="small" onClick={() => handleEdit(rule)} sx={iconButtonSx}>
                                    <EditIcon fontSize="small" />
                                  </IconButton>
                                  <IconButton size="small" onClick={() => handleDelete(rule)} sx={errorIconButtonSx}>
                                    <DeleteIcon fontSize="small" />
                                  </IconButton>
                                </Stack>
                              </Paper>
                            )}
                          </Draggable>
                        ))}
                        {provided.placeholder}
                      </Stack>
                    )}
                  </Droppable>
                </DragDropContext>
              )}

              {rules.length > 0 && filteredRules.length === 0 && (
                <Paper
                  variant="outlined"
                  sx={{ p: 3, mb: 2, textAlign: 'center', borderRadius: 2, bgcolor: elevatedSurface, borderColor: softBorder }}
                >
                  <Typography sx={{ color: secondaryText }}>{t('subscriptions.chain.noMatchingRules')}</Typography>
                </Paper>
              )}

              {rules.length === 0 && (
                <Paper
                  variant="outlined"
                  sx={{
                    p: isMobile ? 3 : 4,
                    textAlign: 'center',
                    borderRadius: 2,
                    bgcolor: elevatedSurface,
                    backgroundImage: dialogSurfaceGradient,
                    borderColor: softBorder,
                    boxShadow: cardShadow
                  }}
                >
                  <TouchAppIcon sx={{ fontSize: 48, color: secondaryText, mb: 1 }} />
                  <Typography sx={{ color: primaryText }} gutterBottom>
                    {t('subscriptions.chain.emptyRules')}
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 2, color: secondaryText }}>
                    {t('subscriptions.chain.emptyRulesDescription')}
                  </Typography>
                  <Button variant="contained" startIcon={<AddIcon />} onClick={handleAdd} size={isMobile ? 'large' : 'medium'}>
                    {t('subscriptions.chain.addRule')}
                  </Button>
                </Paper>
              )}
            </Box>
          )}

          {!loading && editMode && editingRule && (
            <ChainRuleEditor
              value={editingRule}
              onChange={handleRuleChange}
              nodes={options.nodes || []}
              fields={options.conditionFields || []}
              operators={options.operators || []}
              groupTypes={options.groupTypes || []}
              templateGroups={options.templateGroups || []}
              isMobile={isMobile}
            />
          )}
        </DialogContent>

        <DialogActions sx={{ px: isMobile ? 2 : 3, py: 1.5, bgcolor: mutedPanelSurface, borderTop: '1px solid', borderColor: panelBorder }}>
          {!editMode ? (
            <>
              <Button onClick={onClose}>{t('common.close')}</Button>
              {rules.length > 0 && (
                <>
                  <Button variant="outlined" color="info" startIcon={<VisibilityIcon />} onClick={handlePreview}>
                    {t('subscriptions.chain.previewFullChain')}
                  </Button>
                  <Button variant="contained" startIcon={<AddIcon />} onClick={handleAdd}>
                    {t('subscriptions.chain.addRule')}
                  </Button>
                </>
              )}
            </>
          ) : (
            <>
              <Button onClick={handleBack}>{t('subscriptions.chain.backToList')}</Button>
              <Button variant="contained" onClick={handleSave} disabled={loading}>
                {t('subscriptions.chain.saveRule')}
              </Button>
            </>
          )}
        </DialogActions>
      </Dialog>

      <ChainPreviewDialog open={previewOpen} onClose={handleClosePreview} loading={previewLoading} data={previewData} />
    </>
  );
}
