import { type PropsWithChildren, useMemo, useState } from 'react';
import {
  type FabConfig,
  ScaffoldActionContext,
  ScaffoldStateContext,
} from './scaffoldContext.ts';

const ScaffoldProvider = ({ children }: PropsWithChildren) => {
  const [fab, setFab] = useState<FabConfig | null>(null);

  const actionValue = useMemo(() => ({ setFab }), []);

  const stateValue = useMemo(() => ({ fab }), [fab]);

  return (
    <ScaffoldActionContext.Provider value={actionValue}>
      <ScaffoldStateContext.Provider value={stateValue}>
        {children}
      </ScaffoldStateContext.Provider>
    </ScaffoldActionContext.Provider>
  );
};

export default ScaffoldProvider;
