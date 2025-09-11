package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
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
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1024)
			for {
				n := runtime.Stack(buf, false)
				if n < len(buf) {
					buf = buf[:n]
					break
				}
				buf = make([]byte, 2*len(buf))
			}
			slog.Error("Worker panic recovered",
				"worker_id", workerID,
				"event_type", task.EventType,
				"block_number", task.Event.Block.Number,
				"panic", r)

			// fmt so \n and \t are interpreted correctly
			fmt.Printf("Stack trace:\n%s\n", string(buf))
		}
	}()

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

// SubmitBatch submits multiple tasks to the worker pool
func (wp *WorkerPool) SubmitBatch(tasks []Task) error {
	wp.mu.RLock()
	if wp.isShutdown {
		wp.mu.RUnlock()
		return errors.New(config.ErrWorkerPoolShutdown)
	}
	wp.mu.RUnlock()

	for _, task := range tasks {
		select {
		case wp.taskQueue <- task:
			// Task submitted successfully
		case <-wp.ctx.Done():
			return errors.New(config.ErrWorkerPoolShutdown)
		}
	}
	return nil
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

	slog.Info("Worker pool shutdown initiated, waiting for workers to complete")

	// Wait for all workers to finish processing their current tasks
	wp.wg.Wait()

	slog.Info("Worker pool shutdown completed")
}
