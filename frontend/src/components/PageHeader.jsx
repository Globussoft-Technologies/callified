import React from 'react';
import {
  Activity,
  BarChart3,
  Bot,
  CalendarClock,
  ContactRound,
  CreditCard,
  FileClock,
  FileText,
  Flag,
  Gauge,
  Headphones,
  History,
  LayoutDashboard,
  ListChecks,
  Megaphone,
  MessageCircle,
  Package,
  Phone,
  Plug,
  Radio,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  UserCog,
  Users,
  Workflow,
} from 'lucide-react';

const ICONS = {
  activity: Activity,
  analytics: BarChart3,
  billing: CreditCard,
  campaigns: Megaphone,
  checkIn: ShieldCheck,
  dashboard: LayoutDashboard,
  dnd: ContactRound,
  executives: Users,
  featureFlags: Flag,
  history: History,
  integrations: Plug,
  knowledge: FileText,
  logs: FileClock,
  manualDial: Phone,
  monitor: Headphones,
  ops: ListChecks,
  products: Package,
  progress: Gauge,
  providerAccounts: Radio,
  receptionist: Bot,
  sandbox: Sparkles,
  scheduled: CalendarClock,
  settings: Settings,
  subscriptions: SlidersHorizontal,
  team: Users,
  userManagement: UserCog,
  whatsapp: MessageCircle,
  workflow: Workflow,
};

export default function PageHeader({ icon = 'dashboard', title, subtitle, style }) {
  const Icon = ICONS[icon] || LayoutDashboard;

  return (
    <div className="page-heading" style={style}>
      <div className="page-heading-icon" aria-hidden="true">
        <Icon size={20} strokeWidth={2.2} />
      </div>
      <div className="page-heading-copy">
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
    </div>
  );
}
