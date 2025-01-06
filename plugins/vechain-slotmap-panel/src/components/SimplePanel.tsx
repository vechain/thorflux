import React, {useMemo} from 'react';
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

    const epochField = data.series[0].fields.find(f => f.name === '_value');
    const filledField = data.series[0].fields.find(f => f.name === 'filled');
    const proposerField = data.series[0].fields.find(f => f.name === 'proposer');

    if (!epochField || !filledField || !proposerField) {
      return { epochs: [], maxSlots: DEFAULT_SLOTS_PER_EPOCH };
    }

    const epochData: { [key: number]: Array<{ filled: number; proposer: string }> } = {};
    let maxSlotsInEpoch = DEFAULT_SLOTS_PER_EPOCH;

    // First pass: group by epoch and find max slots
    for (let i = 0; i < epochField.values.length; i++) {
      const epoch = epochField.values[i];
      const filled = parseInt(filledField.values[i]);
      const proposer = proposerField.values[i];

      if (!epochData[epoch]) {
        epochData[epoch] = [];
      }

      epochData[epoch].push({ filled, proposer });
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
          filledValues.push({ filled: -1, proposer: '' });
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

  const { epochs, maxSlots } = useMemo(() => {
    return processData();
  }, [data.series])

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

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch (err) {
      console.error('Failed to copy text: ', err);
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
    tooltipContent: css`
      display: flex;
      flex-direction: column;
      align-items: center;
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
    text: string[];
    style: React.CSSProperties;
  }>({
    visible: false,
    text: [],
    style: {},
  });

  const handleSlotHover = (
    event: React.MouseEvent,
    epoch: number,
    slotIndex: number,
    value: { filled: number; proposer: string }
  ) => {
    const element = event.currentTarget;
    const rect = element.getBoundingClientRect();
    const containerRect = element.closest(`.${styles.container}`)?.getBoundingClientRect();

    if (!containerRect) return;

    const status = value.filled === 1 ? 'filled' : value.filled === 0 ? 'missed' : 'pending';
    const slotNumber = getSlotNumber(epoch, slotIndex);

    const left = rect.left - containerRect.left + rect.width / 2;
    const top = rect.top - containerRect.top + (rect.height * 2);

    setTooltip({
      visible: true,
      text: [
        `Slot ${slotNumber}: ${status}`,
        value.proposer || 'No proposer'
      ],
      style: {
        left: `${left}px`,
        top: `${top}px`,
      },
    });
  };

  const handleSlotClick = (proposer: string) => {
    if (proposer) {
      copyToClipboard(proposer);
    }
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
    <>
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
                    backgroundColor: getSlotColor(value.filled),
                    opacity: value.filled === -1 ? 0.3 : 1,
                  }}
                  onClick={() => handleSlotClick(value.proposer)}
                  onMouseEnter={(e) => handleSlotHover(e, epoch, index, value)}
                  onMouseLeave={() => setTooltip(prev => ({ ...prev, visible: false }))}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
      {tooltip.visible && (
        <div
          className={styles.tooltip}
          style={tooltip.style}
        >
          <div className={styles.tooltipContent}>
            {tooltip.text.map((line, index) => (
              <div key={index}>{line}</div>
            ))}
          </div>
        </div>
      )}
    </>
  );
};
