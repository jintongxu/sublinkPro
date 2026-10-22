import { useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Dialog, DialogTitle, DialogContent, DialogActions } from '@mui/material';
import { normalizeTask } from '../contexts/taskProgressState.mjs';

// Result is bounded by the producer; counters describe the full operation.
export default function TaskResultDetails({ task }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { result } = normalizeTask(task);
  if (!result) return null;
  return (
    <>
      <Button
        size="small"
        onClick={(event) => {
          event.stopPropagation();
          setOpen(true);
        }}
      >
        {t('tasks.actions.viewResult', '查看结果')}
      </Button>
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="md">
        <DialogTitle>{t('tasks.actions.viewResult', '查看结果')}</DialogTitle>
        <DialogContent dividers>
          {result.truncated && (
            <Alert severity="info" sx={{ mb: 2 }}>
              {t('tasks.result.truncated', '仅展示部分明细（最多 200 项、32 条诊断），汇总计数为完整结果。')}
            </Alert>
          )}
          <Box component="pre" sx={{ m: 0, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', color: 'text.primary', fontSize: '0.8rem' }}>
            {JSON.stringify(result, null, 2)}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>{t('common.close')}</Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
TaskResultDetails.propTypes = { task: PropTypes.object.isRequired };
