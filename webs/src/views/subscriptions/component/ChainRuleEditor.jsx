import { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';

import ChainFlowBuilder from './ChainFlowBuilder';
import MobileChainBuilder from './MobileChainBuilder';

export default function ChainRuleEditor({
  value,
  onChange,
  nodes = [],
  fields = [],
  operators = [],
  groupTypes = [],
  templateGroups = [],
  isMobile = false
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(value?.name || '');
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const [chainConfig, setChainConfig] = useState([]);
  const [targetConfig, setTargetConfig] = useState({ type: 'specified_nodes', conditions: null, nodeIds: [] });
  const [priority, setPriority] = useState(value?.sort ?? 0);

  const skipNextUpdate = useRef(false);

  useEffect(() => {
    if (value && !skipNextUpdate.current) {
      setName(value.name || '');
      setEnabled(value.enabled ?? true);
      setPriority(value.sort ?? 0);

      if (value.chainConfig) {
        try {
          const parsed = typeof value.chainConfig === 'string' ? JSON.parse(value.chainConfig) : value.chainConfig;
          setChainConfig(Array.isArray(parsed) ? parsed : []);
        } catch {
          setChainConfig([]);
        }
      } else {
        setChainConfig([]);
      }

      if (value.targetConfig) {
        try {
          const parsed = typeof value.targetConfig === 'string' ? JSON.parse(value.targetConfig) : value.targetConfig;
          setTargetConfig({
            type: parsed.type || 'specified_nodes',
            conditions: parsed.conditions || null,
            nodeId: parsed.nodeId,
            nodeIds: Array.isArray(parsed.nodeIds) ? parsed.nodeIds : parsed.nodeId ? [parsed.nodeId] : [],
            endPosition: parsed.endPosition
          });
        } catch {
          setTargetConfig({ type: 'specified_nodes', conditions: null, nodeIds: [] });
        }
      } else {
        setTargetConfig({ type: 'specified_nodes', conditions: null, nodeIds: [] });
      }
    }
    skipNextUpdate.current = false;
  }, [value]);

  const notifyChange = useCallback(
    (newName, newEnabled, newPriority, newChainConfig, newTargetConfig) => {
      skipNextUpdate.current = true;
      const output = {
        name: newName,
        enabled: newEnabled,
        sort: Number(newPriority) || 0,
        chainConfig: JSON.stringify(newChainConfig),
        targetConfig: JSON.stringify({
          type: newTargetConfig.type,
          conditions: newTargetConfig.type === 'conditions' ? newTargetConfig.conditions : undefined,
          nodeId: newTargetConfig.type === 'specified_node' ? newTargetConfig.nodeId : undefined,
          nodeIds: newTargetConfig.type === 'specified_nodes' ? newTargetConfig.nodeIds : undefined,
          endPosition: newTargetConfig.endPosition
        })
      };
      onChange?.(output);
    },
    [onChange]
  );

  const handleNameChange = useCallback(
    (e) => {
      const newName = e.target.value;
      setName(newName);
      notifyChange(newName, enabled, priority, chainConfig, targetConfig);
    },
    [enabled, priority, chainConfig, targetConfig, notifyChange]
  );

  const handleEnabledChange = useCallback(
    (e) => {
      const newEnabled = e.target.checked;
      setEnabled(newEnabled);
      notifyChange(name, newEnabled, priority, chainConfig, targetConfig);
    },
    [name, priority, chainConfig, targetConfig, notifyChange]
  );

  const handleChainConfigChange = useCallback(
    (newConfig) => {
      setChainConfig(newConfig);
      notifyChange(name, enabled, priority, newConfig, targetConfig);
    },
    [name, enabled, priority, targetConfig, notifyChange]
  );

  const handleTargetConfigChange = useCallback(
    (newConfig) => {
      setTargetConfig(newConfig);
      notifyChange(name, enabled, priority, chainConfig, newConfig);
    },
    [name, enabled, priority, chainConfig, notifyChange]
  );

  return (
    <Box sx={{ height: '100%' }}>
      <Stack spacing={2}>
        <Stack direction={isMobile ? 'column' : 'row'} spacing={2} alignItems={isMobile ? 'stretch' : 'center'} sx={{ pt: 1 }}>
          <TextField
            size="small"
            label={t('subscriptions.chain.ruleName')}
            value={name}
            onChange={handleNameChange}
            sx={{ flex: 1 }}
            placeholder={t('subscriptions.chain.ruleNamePlaceholder')}
            fullWidth={isMobile}
          />
          <FormControl
            size="small"
            sx={{ minWidth: 110 }}
          >
            <InputLabel>{t('subscriptions.chain.priority', '优先级')}</InputLabel>
            <Select
              value={priority}
              label={t('subscriptions.chain.priority', '优先级')}
              onChange={(event) => {
                const nextPriority = Number(event.target.value);
                setPriority(nextPriority);
                notifyChange(name, enabled, nextPriority, chainConfig, targetConfig);
              }}
            >
              {[0, 10, 20, 50, 100].map((item) => <MenuItem key={item} value={item}>{item}</MenuItem>)}
            </Select>
          </FormControl>
          <FormControlLabel
            control={<Switch checked={enabled} onChange={handleEnabledChange} />}
            label={t('common.enabled')}
            sx={isMobile ? { alignSelf: 'flex-start' } : {}}
          />
        </Stack>

        <Typography variant="body2" color="text.secondary">
          {isMobile ? t('subscriptions.chain.mobileDescription') : t('subscriptions.chain.desktopDescription')}
        </Typography>

        {isMobile ? (
          <MobileChainBuilder
            chainConfig={chainConfig}
            targetConfig={targetConfig}
            onChainConfigChange={handleChainConfigChange}
            onTargetConfigChange={handleTargetConfigChange}
            nodes={nodes}
            fields={fields}
            operators={operators}
            groupTypes={groupTypes}
            templateGroups={templateGroups}
          />
        ) : (
          <ChainFlowBuilder
            chainConfig={chainConfig}
            targetConfig={targetConfig}
            onChainConfigChange={handleChainConfigChange}
            onTargetConfigChange={handleTargetConfigChange}
            nodes={nodes}
            fields={fields}
            operators={operators}
            groupTypes={groupTypes}
            templateGroups={templateGroups}
          />
        )}
      </Stack>
    </Box>
  );
}
