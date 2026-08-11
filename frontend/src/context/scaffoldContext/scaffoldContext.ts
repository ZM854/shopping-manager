import { createContext, type ReactNode } from 'react';

export interface FabConfig {
  onClick: () => void;
  icon: ReactNode;
}

export interface TopBarConfig {
  onActionClick?: () => void;
  actionIcon?: ReactNode;
  title?: string;
}

interface ScaffoldActionsValue {
  setFab: (config: FabConfig | null) => void;
  setTopBar: (config: TopBarConfig | null) => void;
}

interface ScaffoldStateValue {
  fab: FabConfig | null;
  topBar: TopBarConfig | null;
}

export const ScaffoldActionContext = createContext<ScaffoldActionsValue | null>(
  null,
);
export const ScaffoldStateContext = createContext<ScaffoldStateValue | null>(
  null,
);
