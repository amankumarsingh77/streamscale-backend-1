package worker

import (
	"context"
	"sync"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
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
	wg        sync.WaitGroup
	stopChan  chan struct{}
	isRunning bool
}

type VideoInfo struct {
	Width    int
	Height   int
	Duration float64
}

type VideoProcessor interface {
	ProcessVideo(ctx context.Context, inputPath, outputPath string) error
}
