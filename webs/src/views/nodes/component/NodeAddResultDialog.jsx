import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getNodeDialogPaperSx, getNodePanelSx, getNodeThemeTokens } from '../nodeTheme';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import SwapHorizIcon from '@mui/icons-material/SwapHoriz';

const text = (value, fallback = '') => (typeof value === 'string' || typeof value === 'number' ? String(value) : fallback);

export default function NodeAddResultDialog({ open, result, onClose }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { isDark } = useResolvedColorScheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const tokens = getNodeThemeTokens(theme, isDark);
  if (!result) return null;

  const summary = result.summary || {};
  const items = Array.isArray(result.items) ? result.items : [];
  const skipped = Array.isArray(result.skipped) ? result.skipped : [];
  const failed = Array.isArray(result.failed) ? result.failed : [];
  const counts = {
    received: Number(summary.received ?? items.length ?? 0),
    validated: Number(summary.validated ?? 0),
    added: Number(summary.added ?? result.added ?? items.filter((i) => ['saved', 'added', 'output_ready'].includes(i.status)).length),
    skipped: Number(summary.skipped ?? skipped.length),
    invalid: Number(summary.invalid ?? result.invalid ?? items.filter((i) => i.status === 'invalid').length),
    failed: Number(summary.failed ?? result.failed ?? failed.length),
    warnings: Number(summary.warnings ?? result.warnings ?? 0)
  };
  const diagnostics = Array.isArray(result.diagnostics) ? result.diagnostics : [];
  const displayItems = items.length ? items : [...skipped, ...failed];
  const hasIssues = counts.skipped + counts.invalid + counts.failed + counts.warnings > 0;
  const mode = text(result.mode, '');

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth fullScreen={isMobile} PaperProps={{ sx: getNodeDialogPaperSx(theme, tokens) }}>
      <DialogTitle sx={{ pb: 1, display: 'flex', alignItems: 'center', gap: 1, color: tokens.primaryText, bgcolor: tokens.mutedPanelSurface, borderBottom: '1px solid', borderColor: tokens.panelBorder }}>
        {hasIssues ? <ErrorOutlineIcon sx={{ color: 'warning.main' }} /> : <CheckCircleOutlineIcon sx={{ color: 'success.main' }} />}
        {t('nodes.addResult.title')}
      </DialogTitle>
      <DialogContent sx={{ pt: 1, bgcolor: 'transparent' }}>
        {mode && <Typography variant="body2" sx={{ mb: 1 }}>{t('nodes.addResult.mode', { mode })}</Typography>}
        <Stack direction="row" spacing={1} sx={{ mb: 2, p: 1.5, borderRadius: 2, backgroundColor: tokens.nestedPanelSurface, border: '1px solid', borderColor: tokens.softBorder, flexWrap: 'wrap', gap: 1 }}>
          {Object.entries(counts).map(([key, value]) => <Chip key={key} label={t(`nodes.addResult.stats.${key}`, { count: value })} color={key === 'added' || key === 'validated' ? 'success' : key === 'failed' || key === 'invalid' ? 'error' : 'warning'} variant={value ? 'filled' : 'outlined'} size={isMobile ? 'small' : 'medium'} />)}
        </Stack>
        {result.tls && <Alert severity={result.tls.validated || result.tls.verified ? 'success' : 'warning'} sx={{ mb: 2, borderRadius: 2 }}>{result.tls.validated || result.tls.verified ? t('nodes.addResult.tlsValidated') : t('nodes.addResult.tlsSavedUnverified')}</Alert>}
        {!hasIssues && counts.added > 0 && <Alert severity="success" sx={{ borderRadius: 2 }}>{t('nodes.addResult.successAll', { count: counts.added })}</Alert>}
        {displayItems.length > 0 && <Box sx={{ mt: 2 }}><Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>{t('nodes.addResult.itemsTitle')}</Typography><List dense sx={{ ...getNodePanelSx(theme, tokens, hasIssues ? tokens.palette.warning.main : tokens.palette.success.main, { compact: true }), borderRadius: 2, overflow: 'hidden' }}>{displayItems.map((item, index) => {
          const name = text(item.name || item.safeName, t('nodes.addResult.unknownNode'));
          const protocol = text(item.protocol || item.protocolName, '');
          const status = text(item.status || (item.error ? 'failed' : item.skipped ? 'skipped' : 'saved'), 'unknown');
          const reasonCode = text(item.reasonCode, '');
          const reason = text(item.reason || item.error, '');
          return <ListItem key={index} sx={{ py: 1.25, px: 2 }}><ListItemIcon sx={{ minWidth: 32 }}><SwapHorizIcon fontSize="small" color="warning" /></ListItemIcon><ListItemText primary={<Typography variant="body2" sx={{ fontWeight: 500 }}>{name}</Typography>} secondary={<Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', gap: 0.5 }}><Chip label={status} size="small" variant="outlined" />{protocol && <Chip label={protocol} size="small" variant="outlined" color="info" />}{reasonCode && <Chip label={reasonCode} size="small" variant="outlined" color="secondary" />} {reason && <Typography variant="caption" color="text.secondary">{reason}</Typography>}</Stack>} /></ListItem>;
        })}</List></Box>}
        {diagnostics.length > 0 && <><Divider sx={{ my: 2 }} /><Typography variant="subtitle2" sx={{ mb: 1 }}>{t('nodes.addResult.diagnostics')}</Typography><Alert severity="info">{diagnostics.map((entry, index) => <Box key={index}>{text(entry.message || entry.code || entry, '')}</Box>)}</Alert></>}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2, bgcolor: tokens.mutedPanelSurface, borderTop: '1px solid', borderColor: tokens.panelBorder }}><Button variant="contained" onClick={onClose} fullWidth={isMobile}>{t('common.confirm')}</Button></DialogActions>
    </Dialog>
  );
}
NodeAddResultDialog.propTypes = { open: PropTypes.bool.isRequired, result: PropTypes.object, onClose: PropTypes.func.isRequired };
