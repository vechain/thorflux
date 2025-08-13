package pubsub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

// Task represents a handler task to be executed
type Task struct {
	EventType string
	Handler   Handler
	Event     *types.Event
}

// WorkerPool manages a pool of workers to handle tasks concurrently
type WorkerPool struct {
	workers    int
	taskQueue  chan Task
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	isShutdown bool
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workers int, queueSize int) *WorkerPool {
	if workers <= 0 {
		workers = config.DefaultWorkerPoolSize
	}
	if queueSize <= 0 {
		queueSize = config.DefaultTaskQueueSize
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workers:   workers,
		taskQueue: make(chan Task, queueSize),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start workers
	for i := 0; i < workers; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	slog.Info("Worker pool started", "workers", workers, "queue_size", queueSize)
	return pool
}

// worker is the main worker goroutine that processes tasks
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	slog.Debug("Worker started", "worker_id", id)

	for {
		select {
		case <-wp.ctx.Done():
			slog.Debug("Worker shutting down", "worker_id", id)
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				slog.Debug("Task queue closed, worker shutting down", "worker_id", id)
				return
			}

			wp.processTask(task, id)
		}
	}
}

// processTask executes a single task with error handling and metrics
func (wp *WorkerPool) processTask(task Task, workerID int) {
	start := time.Now()

	slog.Debug("Processing task",
		"worker_id", workerID,
		"event_type", task.EventType,
		"block_number", task.Event.Block.Number)

	if err := task.Handler(task.Event); err != nil {
		slog.Error("Failed to handle event",
			"worker_id", workerID,
			"event_type", task.EventType,
			"error", err,
			"block_number", task.Event.Block.Number)
	} else {
		slog.Debug("Task completed successfully",
			"worker_id", workerID,
			"event_type", task.EventType,
			"duration", time.Since(start),
			"block_number", task.Event.Block.Number)
	}
}

// Submit adds a task to the worker pool queue
func (wp *WorkerPool) Submit(task Task) error {
	wp.mu.RLock()
	if wp.isShutdown {
		wp.mu.RUnlock()
		return ErrWorkerPoolShutdown
	}
	wp.mu.RUnlock()

	select {
	case wp.taskQueue <- task:
		return nil
	case <-wp.ctx.Done():
		return ErrWorkerPoolShutdown
	default:
		return ErrWorkerPoolFull
	}
}

// SubmitBatch submits multiple tasks to the worker pool
func (wp *WorkerPool) SubmitBatch(tasks []Task) error {
	wp.mu.RLock()
	if wp.isShutdown {
		wp.mu.RUnlock()
		return ErrWorkerPoolShutdown
	}
	wp.mu.RUnlock()

	for _, task := range tasks {
		select {
		case wp.taskQueue <- task:
			// Task submitted successfully
		case <-wp.ctx.Done():
			return ErrWorkerPoolShutdown
		default:
			return ErrWorkerPoolFull
		}
	}
	return nil
}

// Wait waits for all workers to finish and all tasks to be completed
func (wp *WorkerPool) Wait() {
	wp.mu.Lock()
	if !wp.isShutdown {
		close(wp.taskQueue)
		wp.isShutdown = true
	}
	wp.mu.Unlock()

	wp.wg.Wait()
	slog.Info("Worker pool shutdown complete")
}

// Shutdown gracefully shuts down the worker pool
func (wp *WorkerPool) Shutdown() {
	wp.mu.Lock()
	if !wp.isShutdown {
		wp.cancel()
		close(wp.taskQueue)
		wp.isShutdown = true
	}
	wp.mu.Unlock()

	slog.Info("Worker pool shutdown initiated")
}

// IsShutdown returns true if the worker pool is shutdown
func (wp *WorkerPool) IsShutdown() bool {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return wp.isShutdown
}

// Stats returns current statistics about the worker pool
func (wp *WorkerPool) Stats() WorkerPoolStats {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	return WorkerPoolStats{
		Workers:    wp.workers,
		QueueSize:  len(wp.taskQueue),
		IsShutdown: wp.isShutdown,
	}
}

// WorkerPoolStats contains statistics about the worker pool
type WorkerPoolStats struct {
	Workers    int
	QueueSize  int
	IsShutdown bool
}

// Error types for worker pool
var (
	ErrWorkerPoolShutdown = &WorkerPoolError{msg: "worker pool is shutdown"}
	ErrWorkerPoolFull     = &WorkerPoolError{msg: "worker pool queue is full"}
)

// WorkerPoolError represents worker pool specific errors
type WorkerPoolError struct {
	msg string
}

func (e *WorkerPoolError) Error() string {
	return e.msg
}
