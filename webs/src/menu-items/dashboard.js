// assets
import { IconDashboard } from '@tabler/icons-react';

// ==============================|| DASHBOARD MENU ITEMS ||============================== //

const dashboard = {
  id: 'dashboard',
  title: 'Dashboard',
  titleKey: 'navigation.groups.dashboard',
  type: 'group',
  children: [
    {
      id: 'default',
      title: 'Dashboard',
      titleKey: 'navigation.items.dashboard',
      type: 'item',
      url: '/dashboard/default',
      icon: IconDashboard,
      breadcrumbs: false
    },
  ]
};

export default dashboard;
