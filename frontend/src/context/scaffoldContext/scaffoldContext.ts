import { createContext, type ReactNode } from 'react';

export interface FabConfig {
  onClick: () => void;
  icon: ReactNode;
}

interface ScaffoldActionsValue {
  setFab: (config: FabConfig | null) => void;
}

interface ScaffoldStateValue {
  fab: FabConfig | null;
}

export const ScaffoldActionContext = createContext<ScaffoldActionsValue | null>(
  null,
);
export const ScaffoldStateContext = createContext<ScaffoldStateValue | null>(
  null,
);
