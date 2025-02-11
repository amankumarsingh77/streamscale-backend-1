package worker

import (
	"context"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
	"sync"
)

type Worker struct {
	logger    logger.Logger
	redisRepo videofiles.RedisRepository
	awsRepo   videofiles.AWSRepository
	cfg       *config.Config
	wg        sync.WaitGroup
}

func NewWorker(cfg *config.Config, logger logger.Logger, redisRepo videofiles.RedisRepository, awsRepo videofiles.AWSRepository) *Worker {
	return &Worker{
		logger:    logger,
		redisRepo: redisRepo,
		awsRepo:   awsRepo,
		cfg:       cfg,
	}
}

func (w *Worker) Start() {
	w.logger.Info("Starting worker")
	for range w.cfg.Worker.WorkerCount {
		w.wg.Add(1)
		go w.Worker()
	}
}

func (w *Worker) Worker() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if canAcceptJob, usage := utils.CheckCPUUsage(w.cfg.Worker.MaxCPUUsage); canAcceptJob {
				if err := ProcessVideo(ctx, w.cfg.S3.InputBucket, w.cfg.S3.OutputBucket); err != nil {
					w.logger.Errorf("error processing video: %v", err)
					return
				}
			} else {
				w.logger.Infof("CPU usage is high: %f", usage)
				return
			}

		}
	}
}
