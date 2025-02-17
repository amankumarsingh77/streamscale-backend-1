package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
)

const (
	VideoJobsQueue  = "video_jobs"
	JobChannel      = "new_video_jobs_channel" // Changed to be more specific
	DefaultCPULimit = 1.0                      // 1 CPU core
)

var ErrNoJob = errors.New("no job available")

func NewWorker(cfg *config.Config, logger logger.Logger, redisRepo videofiles.RedisRepository, awsRepo videofiles.AWSRepository) (*Worker, error) {
	if cfg == nil || logger == nil || redisRepo == nil || awsRepo == nil {
		return nil, errors.New("missing required dependencies")
	}

	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %v", err)
	}

	return &Worker{
		logger:    logger,
		redisRepo: redisRepo,
		awsRepo:   awsRepo,
		cfg:       cfg,
		stopChan:  make(chan struct{}),
		jobChan:   make(chan struct{}, 100),
		client:    client,
	}, nil
}

func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Starting worker pool")
	log.Println(w.cfg.Worker.WorkerCount)

	// Start the job notification subscriber
	w.wg.Add(1)
	go w.subscribeToJobs(ctx)

	// Start the workers
	for i := 0; i < w.cfg.Worker.WorkerCount; i++ {
		log.Println("Starting worker", i)
		w.wg.Add(1)
		go func(id int) {
			w.runWorker(ctx, id)
		}(i)
	}

	return nil
}

func (w *Worker) subscribeToJobs(ctx context.Context) {
	defer w.wg.Done()

	redisClient, ok := w.redisRepo.(interface{ GetRedisClient() *redis.Client })
	if !ok {
		w.logger.Error("Redis repository doesn't support getting client")
		return
	}
	client := redisClient.GetRedisClient()

	pubsub := client.Subscribe(ctx, JobChannel)
	defer pubsub.Close()

	// Wait for confirmation of subscription
	_, err := pubsub.Receive(ctx)
	if err != nil {
		w.logger.Errorf("Failed to subscribe to job channel: %v", err)
		return
	}

	w.logger.Info("Successfully subscribed to job notifications channel")
	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Job subscriber received context cancellation")
			return
		case <-w.stopChan:
			w.logger.Info("Job subscriber received stop signal")
			return
		case msg := <-ch:
			if msg != nil {
				w.logger.Infof("Received job notification: %s", msg.Payload)
				// Notify workers of new job
				select {
				case w.jobChan <- struct{}{}:
					w.logger.Info("Notified workers about new job")
				default:
					w.logger.Debug("Workers already notified about pending jobs")
				}
			}
		}
	}
}

func (w *Worker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	if w.client != nil {
		w.client.Close()
	}
	w.logger.Info("Worker stopped successfully")
}

func (w *Worker) runWorker(ctx context.Context, workerID int) {
	defer w.wg.Done()
	w.logger.Infof("Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			w.logger.Infof("Worker %d received shutdown signal", workerID)
			return
		case <-w.stopChan:
			w.logger.Infof("Worker %d received stop signal", workerID)
			return
		case <-w.jobChan:
			w.logger.Infof("Worker %d received job notification", workerID)
			if err := w.processNextJob(ctx, workerID); err != nil {
				if err == ErrNoJob {
					w.logger.Debugf("Worker %d: no job available", workerID)
					time.Sleep(time.Second)
				} else {
					w.logger.Errorf("Worker %d encountered error: %v", workerID, err)
					time.Sleep(time.Second)
				}
			}
		default:
			// Still check periodically for jobs that might have been missed
			if err := w.processNextJob(ctx, workerID); err != nil {
				if err == ErrNoJob {
					// Wait for job notification or periodic check
					select {
					case <-w.jobChan:
						continue
					case <-time.After(5 * time.Second):
						continue
					}
				}
				w.logger.Errorf("Worker %d encountered error: %v", workerID, err)
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

	// Create a context with timeout for dequeuing
	// dequeueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	// defer cancel()

	job, err := w.redisRepo.DequeueJob(ctx, VideoJobsQueue)
	if err != nil {
		if err == redis.Nil {
			return ErrNoJob
		}
		return fmt.Errorf("failed to dequeue job: %w", err)
	}
	if job == nil {
		return ErrNoJob
	}

	w.logger.Infof("Worker %d processing job: %s", workerID, job.VideoID)

	// Update job status to processing
	if err := w.redisRepo.UpdateStatus(ctx, job.VideoID, VideoJobsQueue, "processing"); err != nil {
		w.logger.Errorf("Failed to update job status: %v", err)
	}

	if err := w.processJobInContainer(ctx, workerID, job.InputS3Key); err != nil {
		// Update job status to failed
		if updateErr := w.redisRepo.UpdateStatus(ctx, job.VideoID, VideoJobsQueue, "failed"); updateErr != nil {
			w.logger.Errorf("Failed to update job status: %v", updateErr)
		}
		return fmt.Errorf("failed to process video: %w", err)
	}

	// Update job status to completed
	if err := w.redisRepo.UpdateStatus(ctx, job.VideoID, VideoJobsQueue, "completed"); err != nil {
		w.logger.Errorf("Failed to update job status: %v", err)
	}

	return nil
}

func (w *Worker) processJobInContainer(ctx context.Context, workerID int, inputS3Key string) error {
	// Pull the container image if not exists
	image, err := w.client.Pull(ctx, "krzemienski/ffmpeg-nvenc-bento4:latest", containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("failed to pull image: %v", err)
	}

	// Create container with CPU limits
	container, err := w.client.NewContainer(
		ctx,
		fmt.Sprintf("video-job-%d", workerID),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(fmt.Sprintf("video-job-snapshot-%d", workerID), image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithCPUs(fmt.Sprintf("%f", w.cfg.Worker.MaxCPUUsage)),
			oci.WithMemoryLimit(1024*1024*1024*3), // 3GB memory limit
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}
	defer container.Delete(ctx, containerd.WithSnapshotCleanup)

	// Create task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("failed to create task: %v", err)
	}
	defer task.Delete(ctx)

	// Start the task
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("failed to start task: %v", err)
	}

	status, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for task: %v", err)
	}

	// Get the exit status
	statusCode := <-status
	if statusCode.ExitCode() != 0 {
		return fmt.Errorf("task failed with exit code: %d", statusCode.ExitCode())
	}

	return nil
}
