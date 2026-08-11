import { type PropsWithChildren, useMemo, useState } from 'react';
import {
  type FabConfig,
  ScaffoldActionContext,
  ScaffoldStateContext,
  type TopBarConfig,
} from './scaffoldContext.ts';

const ScaffoldProvider = ({ children }: PropsWithChildren) => {
  const [fab, setFab] = useState<FabConfig | null>(null);
  const [topBar, setTopBar] = useState<TopBarConfig | null>(null);

  const actionValue = useMemo(() => ({ setFab, setTopBar }), []);

  const stateValue = useMemo(() => ({ fab, topBar }), [fab, topBar]);

  return (
    <ScaffoldActionContext.Provider value={actionValue}>
      <ScaffoldStateContext.Provider value={stateValue}>
        {children}
      </ScaffoldStateContext.Provider>
    </ScaffoldActionContext.Provider>
  );
};

export default ScaffoldProvider;
