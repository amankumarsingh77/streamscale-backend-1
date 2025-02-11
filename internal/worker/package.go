package worker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func PackageVideo(inputPath, outputPath string) error {
	files, err := os.ReadDir(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input directory: %w", err)
	}

	var fragmentedFiles []string

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".mp4") {
			continue
		}

		inputFile := filepath.Join(inputPath, file.Name())
		fragmentedFile := filepath.Join(inputPath, "frag_"+file.Name())

		cmdFragment := exec.Command("mp4fragment", inputFile, fragmentedFile)
		cmdFragment.Stdout = os.Stdout
		cmdFragment.Stderr = os.Stderr

		if err := cmdFragment.Run(); err != nil {
			return fmt.Errorf("failed to fragment %s: %w", file.Name(), err)
		}

		fragmentedFiles = append(fragmentedFiles, fragmentedFile)
	}

	if len(fragmentedFiles) == 0 {
		return fmt.Errorf("no MP4 files found to package")
	}

	args := append([]string{"--output-dir", outputPath, "--hls"}, fragmentedFiles...)
	cmdDash := exec.Command("mp4dash", args...)
	cmdDash.Stdout = os.Stdout
	cmdDash.Stderr = os.Stderr

	output, err := cmdDash.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to package video with mp4dash: %w, %v", err, string(output))
	}

	return nil
}
