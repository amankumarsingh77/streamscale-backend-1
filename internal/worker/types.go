package worker

import (
	"context"
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

type VideoInfo struct {
	Width    int
	Height   int
	Duration float64
}

type VideoProcessor interface {
	ProcessVideo(ctx context.Context, inputPath, outputPath string) error
}
