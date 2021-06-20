package worker

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
)

func transcode(sourceName string, in string, out string) {
	prepare(out)
	var cmd *exec.Cmd
	// create command fitting it's content with appropriate niceness:
	switch sourceName {
	case "CAM":
		// compress camera image slightly more
		cmd = exec.Command("nice", "10", "ffmpeg", "-nostats", "-i", in, "-vsync", "2", "-c:v", "libx264", "-c:a", "aac", "-b:a", "128k", "-crf", "26", out)
	case "PRES":
		cmd = exec.Command("nice", "9", "ffmpeg", "-nostats", "-i", in, "-vsync", "2", "-c:v", "libx264", "-tune", "stillimage", "-c:a", "aac", "-b:a", "128k", "-crf", "20", out)
	case "COMB":
		cmd = exec.Command("nice", "8", "ffmpeg", "-nostats", "-i", in, "-vsync", "2", "-c:v", "libx264", "-c:a", "aac", "-b:a", "128k", "-crf", "24", out)
	default:
		cmd = exec.Command("nice", "10", "ffmpeg", "-nostats", "-i", in, "-vsync", "2", "-c:v", "libx264", "-c:a", "aac", "-b:a", "128k", "-crf", "26", out)
	}
	log.WithFields(log.Fields{"input": in, "output": out, "command": cmd.String()}).Info("Transcoding")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.WithFields(log.Fields{"output": string(output)}).WithError(err).Error("Failed to process stream")
	}
}

// creates folder for output file if it doesn't exist
func prepare(out string) {
	dir := filepath.Dir(out)
	err := os.MkdirAll(dir, 0750)
	if err != nil {
		log.WithError(err).Error("Could not create target folder for transcoding")
	}
}
