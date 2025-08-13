package config

import (
	"time"

	"github.com/vechain/thor/v2/thor"
)

// Blockchain constants
const (
	// Block intervals and timing
	DefaultBlockInterval = 10 * time.Second
	BlockIntervalSeconds = thor.BlockInterval

	// Epoch and period constants
	EpochLength        = 180
	CheckpointInterval = thor.CheckpointInterval

	// Timeout constants
	DefaultTimeout    = 10 * time.Second
	DefaultRetryDelay = 5 * time.Second
	LongRetryDelay    = 30 * time.Second

	// Sync constants
	DefaultQuerySize     = 200
	DefaultChannelBuffer = 2000
	MaxBlocksBehind      = 300

	// Cache constants
	DefaultCacheSize = 100

	// Concurrency constants
	DefaultWorkerPoolSize = 10
	DefaultTaskQueueSize  = 100

	// Logging intervals
	LogIntervalBlocks           = 250
	RecentBlockThreshold        = 10 * time.Minute
	RecentBlockThresholdMinutes = 5 * time.Minute

	// Fork detection
	ForkDetectionTimeout = 3 * time.Minute

	// Gas constants
	GasDivisor  = 100.0
	VETDecimals = 18

	// Database query constants
	DefaultQueryStartDate = "2015-01-01T00:00:00Z"
	DefaultQueryEndDate   = "2100-01-01T00:00:00Z"

	// GetValidators contract address
	GetValidatorsContractAddress = "0x841a6556c524d47030762eb14dc4af897e605d9b"

	// Storage keys
	EpochLengthStorageKey = "epoch-length"
)

// Default configuration values
const (
	DefaultThorURL      = "http://localhost:8569"
	DefaultInfluxDB     = "http://localhost:8086"
	DefaultInfluxToken  = "admin-token"
	DefaultInfluxOrg    = "vechain"
	DefaultInfluxBucket = "vechain"
	DefaultThorBlocks   = 360 * 24 * 7 // 1 week of blocks
)

// Error messages
const (
	ErrInfluxTokenRequired               = "--influx-token or INFLUX_DB_TOKEN is required"
	ErrUnexpectedResults                 = "unexpected number of results: %d, expected %d"
	ErrCallReverted                      = "call %d reverted or had VM error: %s"
	ErrFailedToCreateCache               = "failed to create LRU cache: %w"
	ErrFailedToInitializeABI             = "failed to initialize helper ABI: %w"
	ErrFailedToFetchStakerInfo           = "failed to fetch staker info: %w"
	ErrFailedToUnpackStakerInfo          = "failed to unpack staker info: %w"
	ErrFailedToFetchStakerInfoFromDB     = "failed to fetch staker info from DB: %w"
	ErrFailedToDecodeTotalStake          = "failed to decode total stake data: %w"
	ErrFailedToDecodeQueuedStake         = "failed to decode queued stake data: %w"
	ErrFailedToDecodeStakerBalance       = "failed to decode staker balance data: %w"
	ErrFailedToDecodeTotalSupply         = "failed to decode total supply data: %w"
	ErrFailedToDecodeTotalBurned         = "failed to decode total burned data: %w"
	ErrFailedToUnpackValidators          = "failed to unpack validators: %w"
	ErrFailedToDecodeResultData          = "failed to decode result data: %w"
	ErrFailedToFetchPreviousTotals       = "failed to fetch previous totals: %w"
	ErrFailedToDecodePreviousTotalSupply = "failed to decode total supply data: %w"
	ErrFailedToDecodePreviousTotalBurned = "failed to decode total burned data: %w"

	// Worker pool error messages
	ErrWorkerPoolShutdown = "worker pool is shutdown"
	ErrWorkerPoolFull     = "worker pool queue is full"
	ErrWorkerPoolTimeout  = "worker pool operation timed out"
)

// ABI method names
const (
	StakerBalanceMethod = "stakerBalance"
	GetValidatorsMethod = "getValidators"
	TotalStakeMethod    = "totalStake"
	QueuedStakeMethod   = "queuedStake"
	TotalSupplyMethod   = "totalSupply"
	TotalBurnedMethod   = "totalBurned"
)

// Event names
const (
	ValidationQueuedEvent       = "ValidationQueued"
	ValidationWithdrawnEvent    = "ValidationWithdrawn"
	ValidationSignaledExitEvent = "ValidationSignaledExit"
	StakeIncreasedEvent         = "StakeIncreased"
	StakeDecreasedEvent         = "StakeDecreased"
	DelegationAddedEvent        = "DelegationAdded"
	DelegationWithdrawnEvent    = "DelegationWithdrawn"
	DelegationSignaledExitEvent = "DelegationSignaledExit"
)

// Measurement names for InfluxDB
const (
	BlockStatsMeasurement            = "block_stats"
	TransactionsMeasurement          = "transactions"
	LivenessMeasurement              = "liveness"
	BlockspaceUtilizationMeasurement = "blockspace_utilization"
	StakerEventsMeasurement          = "staker_events"
	IndividualValidatorsMeasurement  = "individual_validators"
	DelegationAddedMeasurement       = "delegation_added"
)

// Field names for InfluxDB
const (
	BestBlockNumberField  = "best_block_number"
	BlockNumberField      = "block_number"
	ValidatorStakedField  = "validator_staked"
	CompletedPeriodsField = "completed_periods"
)
