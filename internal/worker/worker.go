package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
)

var ErrNoJob = errors.New("no job available")

func NewWorker(cfg *config.Config, logger logger.Logger, redisRepo videofiles.RedisRepository, awsRepo videofiles.AWSRepository) *Worker {
	return &Worker{
		logger:    logger,
		redisRepo: redisRepo,
		awsRepo:   awsRepo,
		cfg:       cfg,
		stopChan:  make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Starting worker pool")
	log.Println(w.cfg.Worker.WorkerCount)
	for i := 0; i < w.cfg.Worker.WorkerCount; i++ {
		log.Println("reached")
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

func (w *Worker) runWorker(ctx context.Context, workerID int) {
	defer func() {
		w.logger.Infof("Worker %d shutting down", workerID)
		w.wg.Done()
	}()

	// Log worker start
	w.logger.Infof("Worker %d started", workerID)

	// Add jitter to prevent all workers from polling simultaneously
	time.Sleep(time.Duration(workerID*100) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			w.logger.Infof("Worker %d received context cancellation", workerID)
			return
		case <-w.stopChan:
			w.logger.Infof("Worker %d received stop signal", workerID)
			return
		default:
			if err := w.processNextJob(ctx, workerID); err != nil {
				if err == ErrNoJob {
					time.Sleep(time.Second)
					continue
				}
				w.logger.Errorf("Worker %d encountered error: %v", workerID, err)
				// Add exponential backoff for errors
				time.Sleep(time.Second)
			}
		}
	}
}

func (w *Worker) processNextJob(ctx context.Context, workerID int) error {
	canAcceptJob, usage := utils.CheckCPUUsage(w.cfg.Worker.MaxCPUUsage)
	if !canAcceptJob {
		w.logger.Infof("Worker %d: CPU usage too high (%.2f%%), waiting...", workerID, usage)
		time.Sleep(5 * time.Second)
		return nil
	}

	job, _ := w.redisRepo.PeekJob(ctx, VideoJobsQueueKey)
	//if err != nil {
	//	return fmt.Errorf("failed to peek job: %w", err)
	//}
	if job == nil {
		return ErrNoJob
	}

	w.logger.Infof("Worker %d processing job: %s", workerID, job.VideoID)
	processor := NewVideoProcessor(w.cfg, w.awsRepo)
	if err := processor.ProcessVideo(ctx, job.InputS3Key, job.OutputS3Key); err != nil {
		return fmt.Errorf("failed to process video: %w", err)
	}

	return nil
}
