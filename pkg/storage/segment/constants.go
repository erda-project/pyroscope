package segment

import (
	"os"
	"strconv"
	"time"
)

// TODO: at some point we should change it so that segments can support different
// resolution and multiplier values. For now they are constants
const (
	multiplier = 10
)

var (
	resolution = 100 * time.Second
)

var durations = []time.Duration{}

func init() {
	resolutionDurationStr := os.Getenv("SEGMENT_RESOLUTION_DURATION_SECONDS")
	if resolutionDurationStr != "" {
		resolutionDuration, err := strconv.Atoi(resolutionDurationStr)
		if err != nil {
			panic(err)
		}
		resolution = time.Duration(resolutionDuration) * time.Second
	}
	d := resolution
	for i := 0; i < 50; i++ {
		durations = append(durations, d)
		newD := d * time.Duration(multiplier)
		if newD < d {
			return
		}
		d = newD
	}
}
