package worker

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	MaxParallelJobs    = 4
	MinSegmentDuration = 15
	MaxSegments        = 8
	TempDir            = "tmp_segments"
)

type VideoInfo struct {
	Width    int
	Height   int
	Duration float64
}

func SplitVideo(inputPath, tempDir string, videoInfo *VideoInfo) ([]string, error) {
	segmentDir := filepath.Join(tempDir, "segments")
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		return nil, err
	}

	segmentCount := math.Min(math.Ceil(videoInfo.Duration/MinSegmentDuration), MaxSegments)
	segmentDuration := math.Ceil(videoInfo.Duration / segmentCount)

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c", "copy",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%.0f", segmentDuration),
		"-reset_timestamps", "1",
		"-segment_format_options", "movflags=+faststart",
		filepath.Join(segmentDir, "segment_%03d.mp4"),
	)

	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("splitting failed: %v", err)
	}

	segments, err := filepath.Glob(filepath.Join(segmentDir, "segment_*.mp4"))
	if err != nil {
		return nil, err
	}

	return segments, nil
}

func EncodeSegment(inputPath, outputPath string, bitrate int) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "libsvtav1",
		"-preset", "9",
		"-crf", "32",
		"-g", "240",
		"-svtav1-params",
		fmt.Sprintf("tune=0:"+
			"film-grain=0:"+
			"fast-decode=1:"+
			"mbr=%d", bitrate),
		"-movflags", "+faststart",
		"-c:a", "aac",
		"-b:a", "128k",
		"-y", outputPath,
	)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func StitchSegments(segments []string, outputPath string) error {
	listFile := "concat_list.txt"
	file, err := os.Create(listFile)
	if err != nil {
		return err
	}
	defer os.Remove(listFile)

	for _, seg := range segments {

		absPath, err := filepath.Abs(seg)
		if err != nil {
			return err
		}
		file.WriteString(fmt.Sprintf("file '%s'\n", absPath))
	}
	file.Close()

	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y", outputPath,
	)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func ParallelEncodeSegments(segments []string, bitrate int) ([]string, error) {
	sem := make(chan struct{}, MaxParallelJobs)
	var wg sync.WaitGroup
	encodedSegments := make([]string, len(segments))
	errChan := make(chan error, 1)

	for i, seg := range segments {
		sem <- struct{}{}
		wg.Add(1)

		go func(idx int, inputPath string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			outputPath := filepath.Join("temp/segments", fmt.Sprintf("encoded_%03d.mp4", idx))
			err := EncodeSegment(inputPath, outputPath, bitrate)
			if err != nil {
				select {
				case errChan <- fmt.Errorf("segment %d failed: %v", idx, err):
				default:
				}
				return
			}
			encodedSegments[idx] = outputPath
		}(i, seg)
	}

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return nil, err
	}

	return encodedSegments, nil
}

func GetVideoInfo(inputPath string) (*VideoInfo, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	finalPath := filepath.Join(dir, inputPath)
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", finalPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %v output: %v", err, string(output))
	}

	trimmedOutput := strings.TrimSpace(string(output))
	trimmedOutput = strings.TrimRight(trimmedOutput, ",")
	parts := strings.Split(trimmedOutput, ",")

	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected ffprobe output: %s", trimmedOutput)
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid width: %v", err)
	}

	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid height: %v", err)
	}

	cmd = exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "csv=p=0", finalPath)
	durationOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe duration error: %v", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(durationOutput)), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid duration: %v", err)
	}

	return &VideoInfo{
		Width:    width,
		Height:   height,
		Duration: duration,
	}, nil
}

func parseLogFile(filename, key string) (float64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	sum := 0.0
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, key) {
			parts := strings.Split(line, "=")
			if len(parts) < 2 {
				continue
			}
			val, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				continue
			}
			sum += val
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, fmt.Errorf("no valid entries found for key %s", key)
	}

	return sum / float64(count), nil
}

func AnalyzeComplexity(inputPath string) (spatial, temporal float64, err error) {

	spatialLog := "spatial.log"
	defer os.Remove(spatialLog)

	cmdSpatial := exec.Command("ffmpeg",
		"-i", inputPath,
		"-vf", "signalstats=stat=tout,metadata=print:key=lavfi.signalstats.YAVG:file="+spatialLog,
		"-f", "null", "-",
	)
	if err := cmdSpatial.Run(); err != nil {
		return 0, 0, fmt.Errorf("spatial analysis failed: %v", err)
	}

	yavg, err := parseLogFile(spatialLog, "lavfi.signalstats.YAVG=")
	if err != nil {
		return 0, 0, fmt.Errorf("parsing spatial log failed: %v", err)
	}
	spatial = math.Pow(yavg, 2)

	tempLog := "temp.log"
	defer os.Remove(tempLog)

	cmdTemp := exec.Command("ffmpeg",
		"-i", inputPath,
		"-vf", "signalstats=stat=tout,metadata=print:key=lavfi.signalstats.YDIF:file="+tempLog,
		"-f", "null", "-",
	)
	if err := cmdTemp.Run(); err != nil {
		return 0, 0, fmt.Errorf("temporal analysis failed: %v", err)
	}

	temporal, err = parseLogFile(tempLog, "lavfi.signalstats.YDIF=")
	if err != nil {
		return 0, 0, fmt.Errorf("parsing temporal log failed: %v", err)
	}

	return spatial, temporal, nil
}

func ComputeBitrate(videoInfo *VideoInfo, spatial, temporal float64) int {

	pixels := videoInfo.Width * videoInfo.Height
	baseBitrate := 0
	switch {
	case pixels >= 1920*1080:
		baseBitrate = 1500
	case pixels >= 1280*720:
		baseBitrate = 800
	default:
		baseBitrate = 400
	}

	spatialComplexity := math.Min(spatial/800.0, 1.0)
	temporalComplexity := math.Min(temporal/40.0, 1.0)
	score := (spatialComplexity*0.7 + temporalComplexity*0.3)

	adjusted := int(float64(baseBitrate) * (0.3 + 0.7*score))
	return adjusted
}
