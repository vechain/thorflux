import { PanelPlugin } from '@grafana/data';
import { SimpleOptions } from './types';
import { EpochSlotsPanel } from './components/SimplePanel';

export const plugin = new PanelPlugin<SimpleOptions>(EpochSlotsPanel).setPanelOptions((builder) => {
  return builder;
});
