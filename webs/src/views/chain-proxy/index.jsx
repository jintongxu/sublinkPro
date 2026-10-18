import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import CircularProgress from '@mui/material/CircularProgress';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import SettingsEthernetIcon from '@mui/icons-material/SettingsEthernet';

import MainCard from 'ui-component/cards/MainCard';
import { getSubscriptions } from 'api/subscriptions';
import { ChainProxyDialog } from 'views/subscriptions/component';

export default function ChainProxyManagement() {
  const { t } = useTranslation();
  const [subscriptions, setSubscriptions] = useState([]);
  const [selectedSubscriptionId, setSelectedSubscriptionId] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const selectedSubscription = subscriptions.find((subscription) => subscription.ID === selectedSubscriptionId) || null;

  const loadSubscriptions = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getSubscriptions({ page: 1, pageSize: 1000 });
      const items = response.data?.items || response.data || [];
      setSubscriptions(items);
      setSelectedSubscriptionId((current) => current || items[0]?.ID || '');
    } catch (error) {
      console.error(error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadSubscriptions();
  }, [loadSubscriptions]);

  return (
    <MainCard title={t('navigation.items.chainProxy')} secondary={t('chainProxy.page.subtitle')}>
      <Stack spacing={2}>
        <Card variant="outlined">
          <CardContent>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems={{ xs: 'stretch', sm: 'center' }}>
              <AccountTreeIcon color="primary" />
              <Box sx={{ flex: 1 }}>
                <Typography variant="h6">{t('chainProxy.page.title')}</Typography>
                <Typography variant="body2" color="text.secondary">
                  {t('chainProxy.page.description')}
                </Typography>
              </Box>
              <FormControl size="small" sx={{ minWidth: { xs: '100%', sm: 260 } }} disabled={loading || subscriptions.length === 0}>
                <InputLabel>{t('chainProxy.page.subscription')}</InputLabel>
                <Select
                  value={selectedSubscriptionId}
                  label={t('chainProxy.page.subscription')}
                  onChange={(event) => setSelectedSubscriptionId(event.target.value)}
                >
                  {subscriptions.map((subscription) => (
                    <MenuItem key={subscription.ID} value={subscription.ID}>
                      {subscription.Name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              <Button
                variant="contained"
                startIcon={<SettingsEthernetIcon />}
                disabled={!selectedSubscription}
                onClick={() => setDialogOpen(true)}
              >
                {t('chainProxy.page.manage')}
              </Button>
            </Stack>
          </CardContent>
        </Card>

        {loading && <CircularProgress size={24} />}
        {!loading && subscriptions.length === 0 && (
          <Typography color="text.secondary">{t('chainProxy.page.empty')}</Typography>
        )}
        {selectedSubscription && (
          <Typography variant="body2" color="text.secondary">
            {t('chainProxy.page.currentSubscription', { name: selectedSubscription.Name })}
          </Typography>
        )}
      </Stack>

      <ChainProxyDialog
        open={dialogOpen}
        subscription={selectedSubscription}
        onClose={() => setDialogOpen(false)}
      />
    </MainCard>
  );
}
