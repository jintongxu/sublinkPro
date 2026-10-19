import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import CircularProgress from '@mui/material/CircularProgress';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import SettingsEthernetIcon from '@mui/icons-material/SettingsEthernet';

import MainCard from 'ui-component/cards/MainCard';
import { getSubscriptions, getChainRules, getChainOptions } from 'api/subscriptions';
import { ChainProxyDialog } from 'views/subscriptions/component';

export default function ChainProxyManagement() {
  const { t } = useTranslation();
  const [subscriptions, setSubscriptions] = useState([]);
  const [selectedSubscriptionId, setSelectedSubscriptionId] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const selectedSubscription = subscriptions.find((subscription) => String(subscription.ID) === String(selectedSubscriptionId)) || null;

  const loadSubscriptions = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getSubscriptions({ page: 1, pageSize: 1000 });
      const items = response.data?.items || response.data || [];
      const enrichedItems = await Promise.all(
        items.map(async (subscription) => {
          try {
            const [rulesResponse, optionsResponse] = await Promise.all([
              getChainRules(subscription.ID),
              getChainOptions(subscription.ID)
            ]);
            return {
              ...subscription,
              ChainRules: rulesResponse?.data || [],
              ChainOptions: optionsResponse?.data || { nodes: [] }
            };
          } catch (error) {
            console.warn('加载订阅链式代理规则失败:', subscription.ID, error);
            return { ...subscription, ChainRules: [], ChainOptions: { nodes: [] } };
          }
        })
      );
      setSubscriptions(enrichedItems);
      setSelectedSubscriptionId((current) => {
        const savedId = localStorage.getItem('chainProxy_selectedSubscriptionId');
        const nextId = current || savedId;
        return enrichedItems.some((subscription) => String(subscription.ID) === String(nextId)) ? nextId : enrichedItems[0]?.ID || '';
      });
    } catch (error) {
      console.error(error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadSubscriptions();
  }, [loadSubscriptions]);

  const refreshSubscriptionRules = useCallback(async (subscriptionId) => {
    if (!subscriptionId) return;
    try {
      const response = await getChainRules(subscriptionId);
      setSubscriptions((current) =>
        current.map((subscription) =>
          String(subscription.ID) === String(subscriptionId) ? { ...subscription, ChainRules: response?.data || [] } : subscription
        )
      );
    } catch (error) {
      console.warn('加载订阅链式代理规则失败:', subscriptionId, error);
    }
  }, []);

  const handleDialogClose = () => {
    setDialogOpen(false);
    refreshSubscriptionRules(selectedSubscriptionId);
  };

  const handleManageSubscription = (subscription) => {
    setSelectedSubscriptionId(subscription.ID);
    localStorage.setItem('chainProxy_selectedSubscriptionId', String(subscription.ID));
    setDialogOpen(true);
  };

  const getNodeName = (nodes, nodeId) => {
    const node = (nodes || []).find((item) => String(item.id ?? item.ID) === String(nodeId));
    return node?.name || node?.Name || node?.linkName || node?.LinkName || t('subscriptions.chain.nodeNumber', { id: nodeId });
  };

  const getChainReferences = (rule, subscription) => {
    let chain = [];
    try {
      chain = JSON.parse(rule.chainConfig || '[]');
    } catch {
      return [t('subscriptions.chain.invalidConfig')];
    }
    return chain.map((item) => {
      if (item.type === 'specified_node' && item.nodeId) return getNodeName(subscription.ChainOptions?.nodes, item.nodeId);
      if (item.type === 'specified_node') return t('subscriptions.chain.proxyTypes.specified_node');
      if (item.groupName) return item.groupName;
      if (item.type === 'dynamic_node') {
        const count = item.nodeConditions?.conditions?.length;
        return count ? t('subscriptions.chain.dynamicNodeWithConditions', { count }) : t('subscriptions.chain.proxyTypeShort.dynamic_node');
      }
      return t(`subscriptions.chain.proxyTypeShort.${item.type}`, { defaultValue: item.type });
    });
  };

  const getTargetReference = (rule, subscription) => {
    try {
      const target = JSON.parse(rule.targetConfig || '{}');
      if (target.type === 'specified_node' && target.nodeId) return getNodeName(subscription.ChainOptions?.nodes, target.nodeId);
      if (target.type === 'specified_nodes' && target.nodeIds?.length) {
        return target.nodeIds.map((id) => getNodeName(subscription.ChainOptions?.nodes, id)).join(', ');
      }
      if (target.type === 'conditions' && target.conditions?.conditions?.length) {
        return t('subscriptions.chain.conditionCount', { count: target.conditions.conditions.length });
      }
      return t(`subscriptions.chain.targetTypes.${target.type || 'all'}`, { defaultValue: t('subscriptions.chain.unconfigured') });
    } catch {
      return t('subscriptions.chain.unconfigured');
    }
  };

  return (
    <MainCard title={t('navigation.items.chainProxy')} secondary={t('chainProxy.page.subtitle')}>
      <Stack spacing={2}>
        <Card variant="outlined">
          <CardContent>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems={{ xs: 'flex-start', sm: 'center' }}>
              <AccountTreeIcon color="primary" />
              <Box sx={{ flex: 1 }}>
                <Typography variant="h6">{t('chainProxy.page.title')}</Typography>
                <Typography variant="body2" color="text.secondary">
                  {t('chainProxy.page.description')}
                </Typography>
              </Box>
              <Chip size="small" variant="outlined" label={t('chainProxy.page.subscriptionCount', { count: subscriptions.length })} />
            </Stack>
          </CardContent>
        </Card>

        {loading && <CircularProgress size={24} />}
        {!loading && subscriptions.length === 0 && <Typography color="text.secondary">{t('chainProxy.page.empty')}</Typography>}
        {!loading && subscriptions.length > 0 && (
          <Stack spacing={1.5}>
            {subscriptions.map((subscription) => {
              const rules = subscription.ChainRules || [];
              const enabledRules = rules.filter((rule) => rule.enabled !== false);
              const isSelected = String(subscription.ID) === String(selectedSubscriptionId);
              return (
                <Card
                  key={subscription.ID}
                  variant="outlined"
                  sx={{
                    borderColor: isSelected ? 'primary.main' : 'divider',
                    bgcolor: isSelected ? 'action.hover' : 'background.paper'
                  }}
                >
                  <CardContent>
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 1.5, sm: 3 }} alignItems={{ xs: 'stretch', sm: 'center' }}>
                      <Box sx={{ width: { sm: '28%' }, minWidth: 0 }}>
                        <Typography variant="overline" color="text.secondary">
                          {t('chainProxy.page.currentSubscriptionLabel')}
                        </Typography>
                        <Typography variant="h6" noWrap title={subscription.Name}>
                          {subscription.Name}
                        </Typography>
                      </Box>

                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 0.75 }}>
                          {rules.length ? t('chainProxy.page.previewHint') : t('chainProxy.page.notConfigured')}
                        </Typography>
                        {rules.length > 0 && (
                          <Stack spacing={0.75}>
                            {rules.map((rule, index) => {
                              const chainReferences = getChainReferences(rule, subscription);
                              const targetReference = getTargetReference(rule, subscription);
                              return (
                                <Stack key={rule.id} direction={{ xs: 'column', md: 'row' }} spacing={0.75} alignItems={{ xs: 'flex-start', md: 'center' }}>
                                  <Chip
                                    size="small"
                                    color={rule.enabled === false ? 'default' : 'primary'}
                                    variant={rule.enabled === false ? 'outlined' : 'filled'}
                                    label={`${index + 1}. ${rule.name || t('subscriptions.chain.unnamedRule')}`}
                                  />
                                  <Typography variant="body2" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                                    {chainReferences.length ? chainReferences.join(' → ') : t('subscriptions.chain.unconfigured')}
                                    {' → '}
                                    <strong>{targetReference}</strong>
                                  </Typography>
                                </Stack>
                              );
                            })}
                          </Stack>
                        )}
                      </Box>

                      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap sx={{ flexShrink: 0 }}>
                        <Chip size="small" color={enabledRules.length ? 'success' : 'default'} label={enabledRules.length ? t('chainProxy.page.enabled') : t('chainProxy.page.notEnabled')} />
                        <Chip size="small" variant="outlined" label={t('chainProxy.page.ruleSummary', { total: rules.length, enabled: enabledRules.length })} />
                        <Button variant="contained" startIcon={<SettingsEthernetIcon />} onClick={() => handleManageSubscription(subscription)}>
                          {t('chainProxy.page.manage')}
                        </Button>
                      </Stack>
                    </Stack>
                  </CardContent>
                </Card>
              );
            })}
          </Stack>
        )}
      </Stack>

      <ChainProxyDialog
        open={dialogOpen}
        subscription={selectedSubscription}
        onClose={handleDialogClose}
      />
    </MainCard>
  );
}
