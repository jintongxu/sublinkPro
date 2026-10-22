const STATUSES = ['untested', 'success', 'partial', 'failed', 'disabled'];

// One safe display contract for table and mobile. Never render raw parser/network
// errors containing credentials. React escapes the returned plain text.
export function getValidationDisplay(node, translate) {
  const raw = String(node.ValidationStatus || node.validationStatus || 'untested').toLowerCase();
  const status = STATUSES.includes(raw) ? raw : 'untested';
  const color = { untested: 'default', success: 'success', partial: 'warning', failed: 'error', disabled: 'default' }[status];
  const reason = String(node.ValidationReason || node.validationReason || '')
    .replace(/[a-z][a-z0-9+.-]*:\/\/[^\s]+/gi, '[redacted link]')
    .replace(/(?:token|password|passwd|secret|credential)\s*[:=]\s*[^\s,;]+/gi, '[redacted credential]')
    .replace(/[\u0000-\u001f\u007f]/g, ' ')
    .trim()
    .slice(0, 160);
  const checkedAt = node.ValidationCheckAt || node.validationCheckAt;
  const date = checkedAt ? new Date(checkedAt) : null;
  const time = date && !Number.isNaN(date.getTime()) ? date.toLocaleString() : '';
  const detail = [reason, time && translate('nodes.table.validationCheckedAt', { time })].filter(Boolean).join(' · ');
  return { status, color, label: translate(`nodes.table.validationStatus.${status}`), detail };
}
