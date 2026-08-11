import { useContext, useLayoutEffect } from 'react';
import {
  type FabConfig,
  ScaffoldActionContext,
  ScaffoldStateContext,
  type TopBarConfig,
} from '../context/scaffoldContext/scaffoldContext.ts';

export function useScaffoldState() {
  const ctx = useContext(ScaffoldStateContext);
  if (!ctx)
    throw new Error(
      'useScaffoldState must be used within the ScaffoldStateProvider',
    );
  return ctx;
}

export function useScaffold(config: {
  fab?: FabConfig | null;
  topBar?: TopBarConfig | null;
}) {
  const actions = useContext(ScaffoldActionContext);

  if (!actions)
    throw new Error(
      'useScaffold must be used within the ScaffoldActionProvider',
    );

  const fab = config.fab;
  const topBar = config.topBar;
  useLayoutEffect(() => {
    if (fab !== undefined) {
      actions.setFab(fab);
    }

    if (topBar !== undefined) {
      actions.setTopBar(topBar);
    }

    return () => {
      actions.setFab(null);
      actions.setTopBar(null);
    };
  }, [actions, fab, topBar]);
}
