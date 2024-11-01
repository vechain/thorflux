import React from 'react';
import { PanelProps } from '@grafana/data';
import { useTheme2 } from '@grafana/ui';
import { css } from '@emotion/css';

export const EpochBlocksPanel: React.FC<PanelProps> = ({ data, width, height }) => {
  const theme = useTheme2();

  const processData = () => {
    if (!data.series.length || !data.series[0].fields.length) {
      return [];
    }

    const epochField = data.series[0].fields.find(f => f.name === 'epoch');
    const valueField = data.series[0].fields.find(f => f.name === '_value');

    if (!epochField || !valueField) {
      return [];
    }

    const epochData: { [key: number]: number[] } = {};
    for (let i = 0; i < epochField.values.length; i++) {
      const epoch = epochField.values[i];
      const value = valueField.values[i];

      if (!epochData[epoch]) {
        epochData[epoch] = [];
      }

      epochData[epoch].push(value);
    }

    const epochs = Object.entries(epochData)
      .map(([epoch, values]) => ({
        epoch: parseInt(epoch),
        values
      }))
      .sort((a, b) => a.epoch - b.epoch);

    return epochs.map((epochData, index) => {
      const isFirstEpoch = index === 0;
      const isOnlyEpoch = epochs.length === 1;

      if (isFirstEpoch && !isOnlyEpoch) {
        return {
          epoch: epochData.epoch,
          values: epochData.values
        };
      } else {
        const filledValues = [...epochData.values];
        while (filledValues.length < 180) {
          filledValues.push(-1); // -1 represents pending
        }
        return {
          epoch: epochData.epoch,
          values: filledValues
        };
      }
    });
  };

  const epochs = processData();

  const getBlockNumber = (epoch: number, blockIndex: number) => {
    return (epoch * 180) + blockIndex;
  };

  // Function to interpolate between colors based on percentage
  const interpolateColor = (percent: number): string => {
    if (percent === -1) return theme.colors.secondary.main; // Pending block

    // Convert percentage to ensure 0 is green and 100 is red
    const normalizedPercent = percent / 100;

    // RGB values for green (0%) and red (100%)
    const green = { r: 50, g: 205, b: 50 }; // Lighter green
    const red = { r: 220, g: 20, b: 60 }; // Crimson red

    // Interpolate between the colors
    const r = Math.round(green.r + (red.r - green.r) * normalizedPercent);
    const g = Math.round(green.g + (red.g - green.g) * normalizedPercent);
    const b = Math.round(green.b + (red.b - green.b) * normalizedPercent);

    return `rgb(${r}, ${g}, ${b})`;
  };

  const BLOCK_SIZE = 16;
  const BLOCK_GAP = 2;
  const BLOCKS_PER_MARKER = 12;
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
    blocksContainer: css`
      display: flex;
      flex-wrap: wrap;
      gap: ${BLOCK_GAP}px;
      position: relative;
    `,
    block: css`
      width: ${BLOCK_SIZE}px;
      height: ${BLOCK_SIZE}px;
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

  const handleBlockHover = (
    event: React.MouseEvent,
    epoch: number,
    blockIndex: number,
    value: number
  ) => {
    const element = event.currentTarget;
    const rect = element.getBoundingClientRect();
    const containerRect = element.closest(`.${styles.container}`)?.getBoundingClientRect();

    if (!containerRect) return;

    let status: string;
    if (value === -1) {
      status = 'pending';
    } else {
      status = `${value}%`;
    }

    const blockNumber = getBlockNumber(epoch, blockIndex);

    const left = rect.left - containerRect.left + rect.width / 2;
    const top = rect.top - containerRect.top + rect.height;

    setTooltip({
      visible: true,
      text: `Block ${blockNumber}: ${status}`,
      style: {
        left: `${left}px`,
        top: `${top}px`,
      },
    });
  };

  const getMarkerPosition = (blockIndex: number) => {
    const blockTotalWidth = BLOCK_SIZE + BLOCK_GAP;
    return blockIndex * blockTotalWidth + (BLOCK_SIZE / 2);
  };

  const markers = Array.from({ length: Math.ceil(180 / BLOCKS_PER_MARKER) }, (_, i) => {
    const blockIndex = i * BLOCKS_PER_MARKER;
    return {
      blockIndex,
      position: getMarkerPosition(blockIndex)
    };
  });

  return (
    <div className={styles.container}>
      <div className={styles.headerContainer}>
        <div className={styles.headerContent}>
          {markers.map(({ blockIndex, position }) => (
            <div
              key={blockIndex}
              className={styles.headerMarker}
              style={{ left: position }}
            >
              {blockIndex}
            </div>
          ))}
        </div>
      </div>

      {epochs.map(({ epoch, values }) => (
        <div key={epoch} className={styles.row}>
          <div className={styles.epochNumber}>{epoch}</div>
          <div className={styles.blocksContainer}>
            {values.map((value, index) => (
              <div
                key={index}
                className={styles.block}
                style={{
                  backgroundColor: interpolateColor(value),
                  opacity: value === -1 ? 0.3 : 1,
                }}
                onMouseEnter={(e) => handleBlockHover(e, epoch, index, value)}
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