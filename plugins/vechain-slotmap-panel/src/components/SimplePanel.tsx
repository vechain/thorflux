import React from 'react';
import { PanelProps } from '@grafana/data';
import { useTheme2 } from '@grafana/ui';
import { css } from '@emotion/css';

export const EpochSlotsPanel: React.FC<PanelProps> = ({ data, width, height }) => {
  const theme = useTheme2();

  const DEFAULT_SLOTS_PER_EPOCH = 180;

  const processData = () => {
    if (!data.series.length || !data.series[0].fields.length) {
      return { epochs: [], maxSlots: DEFAULT_SLOTS_PER_EPOCH };
    }

    const epochField = data.series[0].fields.find(f => f.name === 'epoch');
    const valueField = data.series[0].fields.find(f => f.name === '_value');

    if (!epochField || !valueField) {
      return { epochs: [], maxSlots: DEFAULT_SLOTS_PER_EPOCH };
    }

    const epochData: { [key: number]: number[] } = {};
    let maxSlotsInEpoch = DEFAULT_SLOTS_PER_EPOCH;

    // First pass: group by epoch and find max slots
    for (let i = 0; i < epochField.values.length; i++) {
      const epoch = epochField.values[i];
      const value = valueField.values[i];

      if (!epochData[epoch]) {
        epochData[epoch] = [];
      }

      epochData[epoch].push(value);
      maxSlotsInEpoch = Math.max(maxSlotsInEpoch, epochData[epoch].length);
    }

    const epochs = Object.entries(epochData)
      .map(([epoch, values]) => ({
        epoch: parseInt(epoch),
        values
      }))
      .sort((a, b) => a.epoch - b.epoch);

    const processedEpochs = epochs.map((epochData, index) => {
      const isFirstEpoch = index === 0;
      const isOnlyEpoch = epochs.length === 1;

      if (isFirstEpoch && !isOnlyEpoch) {
        return {
          epoch: epochData.epoch,
          values: epochData.values
        };
      } else {
        const filledValues = [...epochData.values];
        while (filledValues.length < maxSlotsInEpoch) {
          filledValues.push(-1);
        }
        return {
          epoch: epochData.epoch,
          values: filledValues
        };
      }
    });

    return {
      epochs: processedEpochs,
      maxSlots: maxSlotsInEpoch
    };
  };

  const { epochs, maxSlots } = processData();

  const getSlotNumber = (epoch: number, slotIndex: number) => {
    return (epoch * maxSlots) + slotIndex;
  };

  const getSlotColor = (value: number) => {
    switch (value) {
      case 1:
        return theme.colors.success.main;
      case 0:
        return theme.colors.error.main;
      default:
        return theme.colors.secondary.main;
    }
  };

  const SLOT_SIZE = 16;
  const SLOT_GAP = 2;
  const SLOTS_PER_MARKER = 12;
  const HEADER_BOTTOM_MARGIN = 24;
  const EPOCH_NUMBER_WIDTH = 60;
  const EPOCH_NUMBER_MARGIN = 8;

  const styles = {
    container: css`
      padding: ${theme.spacing(1)};
      width: 100%;
      height: 100%;
      overflow: auto;
      position: relative;
      isolation: isolate;
    `,
    row: css`
      display: flex;
      align-items: center;
      margin-bottom: ${theme.spacing(0.5)};
      position: relative;
    `,
    epochNumber: css`
      width: ${EPOCH_NUMBER_WIDTH}px;
      margin-right: ${EPOCH_NUMBER_MARGIN}px;
      font-size: ${theme.typography.size.sm};
    `,
    slotsContainer: css`
      display: flex;
      flex-wrap: wrap;
      gap: ${SLOT_GAP}px;
      position: relative;
    `,
    slot: css`
      width: ${SLOT_SIZE}px;
      height: ${SLOT_SIZE}px;
      border-radius: 2px;
      cursor: pointer;
      transition: opacity 0.2s;
      position: relative;
      &:hover {
        opacity: 0.8;
      }
    `,
    tooltip: css`
      position: absolute;
      background: ${theme.colors.background.secondary};
      padding: ${theme.spacing(0.5)} ${theme.spacing(1)};
      border-radius: ${theme.shape.radius.default};
      font-size: ${theme.typography.size.xs};
      z-index: 9999;
      transform: translate(-50%, 0);
      white-space: nowrap;
      pointer-events: none;
      box-shadow: 0 2px 4px rgba(0, 0, 0, 0.15);
      top: 100%;
      margin-top: 1px;
    `,
    headerContainer: css`
      margin-bottom: ${HEADER_BOTTOM_MARGIN}px;
      position: relative;
      height: 20px;
      z-index: 1;
    `,
    headerContent: css`
      position: relative;
      margin-left: ${EPOCH_NUMBER_WIDTH + EPOCH_NUMBER_MARGIN}px;
    `,
    headerMarker: css`
      position: absolute;
      text-align: center;
      transform: translateX(-50%);
      color: ${theme.colors.text.secondary};
      font-size: ${theme.typography.size.sm};
    `
  };

  const [tooltip, setTooltip] = React.useState<{
    visible: boolean;
    text: string;
    style: React.CSSProperties;
  }>({
    visible: false,
    text: '',
    style: {},
  });

  const handleSlotHover = (
    event: React.MouseEvent,
    epoch: number,
    slotIndex: number,
    value: number
  ) => {
    const element = event.currentTarget;
    const rect = element.getBoundingClientRect();
    const containerRect = element.closest(`.${styles.container}`)?.getBoundingClientRect();

    if (!containerRect) return;

    const status = value === 1 ? 'filled' : value === 0 ? 'missed' : 'pending';
    const slotNumber = getSlotNumber(epoch, slotIndex);

    const left = rect.left - containerRect.left + rect.width / 2;
    const top = rect.top - containerRect.top + rect.height;

    setTooltip({
      visible: true,
      text: `Slot ${slotNumber}: ${status}`,
      style: {
        left: `${left}px`,
        top: `${top}px`,
      },
    });
  };

  const getMarkerPosition = (slotIndex: number) => {
    const slotTotalWidth = SLOT_SIZE + SLOT_GAP;
    return slotIndex * slotTotalWidth + (SLOT_SIZE / 2);
  };

  const markers = Array.from({ length: Math.ceil(maxSlots / SLOTS_PER_MARKER) }, (_, i) => {
    const slotIndex = i * SLOTS_PER_MARKER;
    return {
      slotIndex,
      position: getMarkerPosition(slotIndex)
    };
  });

  return (
    <div className={styles.container}>
      <div className={styles.headerContainer}>
        <div className={styles.headerContent}>
          {markers.map(({ slotIndex, position }) => (
            <div
              key={slotIndex}
              className={styles.headerMarker}
              style={{ left: position }}
            >
              {slotIndex}
            </div>
          ))}
        </div>
      </div>

      {epochs.map(({ epoch, values }) => (
        <div key={epoch} className={styles.row}>
          <div className={styles.epochNumber}>{epoch}</div>
          <div className={styles.slotsContainer}>
            {values.map((value, index) => (
              <div
                key={index}
                className={styles.slot}
                style={{
                  backgroundColor: getSlotColor(value),
                  opacity: value === -1 ? 0.3 : 1,
                }}
                onMouseEnter={(e) => handleSlotHover(e, epoch, index, value)}
                onMouseLeave={() => setTooltip(prev => ({ ...prev, visible: false }))}
              />
            ))}
          </div>
        </div>
      ))}

      {tooltip.visible && (
        <div
          className={styles.tooltip}
          style={tooltip.style}
        >
          {tooltip.text}
        </div>
      )}
    </div>
  );
};