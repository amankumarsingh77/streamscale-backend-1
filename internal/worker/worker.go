package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
)

var ErrNoJob = errors.New("no job available")

type Worker struct {
	logger    logger.Logger
	redisRepo videofiles.RedisRepository
	awsRepo   videofiles.AWSRepository
	cfg       *config.Config
	wg        sync.WaitGroup
	stopChan  chan struct{}
	jobs      chan *models.EncodeJob
	semaphore chan struct{} // For limiting concurrent tasks per worker
}

func NewWorker(cfg *config.Config, logger logger.Logger, redisRepo videofiles.RedisRepository, awsRepo videofiles.AWSRepository) *Worker {
	return &Worker{
		logger:    logger,
		redisRepo: redisRepo,
		awsRepo:   awsRepo,
		cfg:       cfg,
		stopChan:  make(chan struct{}),
		jobs:      make(chan *models.EncodeJob, 100),           // Buffer size for job channel
		semaphore: make(chan struct{}, cfg.Worker.WorkerCount), // Limit concurrent tasks
	}
}

func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Starting worker pool")

	// Start the Redis subscriber in a separate goroutine
	w.wg.Add(1)
	go w.subscribeToJobs(ctx)

	// Start worker goroutines
	for i := 0; i < w.cfg.Worker.WorkerCount; i++ {
		w.wg.Add(1)
		go func(id int) {
			w.runWorker(ctx, id)
		}(i)
	}

	return nil
}

func (w *Worker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	w.logger.Info("Worker pool stopped")
}

func (w *Worker) subscribeToJobs(ctx context.Context) {
	defer w.wg.Done()

	jobChan, err := w.redisRepo.SubscribeToJobs(ctx, VideoJobsQueueKey)
	if err != nil {
		w.logger.Errorf("Failed to subscribe to jobs: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case job := <-jobChan:
			if job != nil {
				select {
				case w.jobs <- job:
					// Job successfully queued
				case <-ctx.Done():
					return
				case <-w.stopChan:
					return
				}
			}
		}
	}
}

func (w *Worker) runWorker(ctx context.Context, workerID int) {
	defer w.wg.Done()
	w.logger.Infof("Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			w.logger.Infof("Worker %d received context cancellation", workerID)
			return
		case <-w.stopChan:
			w.logger.Infof("Worker %d received stop signal", workerID)
			return
		case job := <-w.jobs:
			// Acquire semaphore slot
			select {
			case w.semaphore <- struct{}{}:
				// Process the job in a separate goroutine
				go func() {
					defer func() { <-w.semaphore }() // Release semaphore slot
					if err := w.processJob(ctx, workerID, job); err != nil {
						w.logger.Errorf("Worker %d failed to process job %s: %v", workerID, job.JobID, err)
					}
				}()
			default:
				// If we can't acquire semaphore, put job back in channel
				select {
				case w.jobs <- job:
				default:
					w.logger.Warnf("Worker %d: Failed to requeue job %s, channel full", workerID, job.JobID)
				}
			}
		}
	}
}

func (w *Worker) processJob(ctx context.Context, workerID int, job *models.EncodeJob) error {
	w.logger.Infof("Worker %d processing job: %s", workerID, job.VideoID)

	// Check CPU usage before processing
	canAcceptJob, usage := utils.CheckCPUUsage(w.cfg.Worker.MaxCPUUsage)
	if !canAcceptJob {
		w.logger.Infof("Worker %d: CPU usage too high (%.2f%%), requeueing job", workerID, usage)
		select {
		case w.jobs <- job:
			return nil
		default:
			return fmt.Errorf("failed to requeue job, channel full")
		}
	}

	processor := NewVideoProcessor(w.cfg, w.awsRepo)
	if err := processor.ProcessVideo(ctx, job.InputS3Key, job.OutputS3Key); err != nil {
		return fmt.Errorf("failed to process video: %w", err)
	}

	return nil
}
