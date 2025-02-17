package worker

import (
	"context"
	"sync"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/containerd/containerd"
)

const (
	VideoJobsQueueKey  = "video_jobs"
	TempDir            = "tmp_segments"
	MaxParallelJobs    = 4
	MinSegmentDuration = 15
	MaxSegments        = 8
	DefaultBaseBitrate = 400
	HDBaseBitrate      = 800
	FullHDBaseBitrate  = 1500
)

type Worker struct {
	logger    logger.Logger
	redisRepo videofiles.RedisRepository
	awsRepo   videofiles.AWSRepository
	cfg       *config.Config
	stopChan  chan struct{}
	jobChan   chan struct{}
	wg        sync.WaitGroup
	client    *containerd.Client
}

type VideoInfo struct {
	Width    int
	Height   int
	Duration float64
}

type VideoProcessor interface {
	ProcessVideo(ctx context.Context, inputPath, outputPath string) error
}
