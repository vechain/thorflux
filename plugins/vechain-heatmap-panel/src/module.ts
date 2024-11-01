import { PanelPlugin } from '@grafana/data';
import { SimpleOptions } from './types';
import { EpochBlocksPanel } from './components/SimplePanel';

export const plugin = new PanelPlugin<SimpleOptions>(EpochBlocksPanel).setPanelOptions((builder) => {
  return builder;
});
