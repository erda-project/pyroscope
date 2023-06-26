package flamebearer

import (
	"fmt"
	"strings"
)

// FormatValue formats a value based on the unit and max value
// Migrated from the front-end js chart: https://github.com/grafana/pyroscope/blob/b553e3a99152eb3da57f49dc6e44b047f0a56a5a/packages/pyroscope-flamegraph/src/ProfilerTable.tsx#L263-L445
func FormatValue(max, sampleRate int, unit string, samples int) string {
	switch unit {
	case "samples":
		return NewDurationFormatter(max/sampleRate, "", false).format(samples, sampleRate, true)
	case "objects":
		return NewObjectsFormatter(max).format(samples)
	case "goroutines":
		return NewObjectsFormatter(max).format(samples)
	case "bytes":
		return NewBytesFormatter(max).format(samples)
	case "lock_nanoseconds":
		return NewNanosecondsFormatter(max).format(samples)
	case "lock_samples":
		return NewObjectsFormatter(max).format(samples)
	case "trace_samples":
		return NewDurationFormatter(max/sampleRate, "", true).format(samples, sampleRate, true)
	case "exceptions":
		return NewObjectsFormatter(max).format(samples)
	default:
		return NewDurationFormatter(max/sampleRate, " ", true).format(samples, sampleRate, true)
	}
}

type DurationFormatter struct {
	Divider                  float64
	EnableSubsecondPrecision bool
	Suffix                   string
	Durations                [][2]int
	Units                    string
}

func NewDurationFormatter(maxDur int, units string, enableSubsecondPrecision bool) *DurationFormatter {
	df := &DurationFormatter{
		Divider: 1,
	}
	df.EnableSubsecondPrecision = enableSubsecondPrecision
	df.Durations = [][2]int{{60, 1}, {60, 2}, {24, 3}, {30, 4}, {12, 5}}
	df.Units = units

	if df.EnableSubsecondPrecision {
		df.Durations = append([][2]int{{1000, 6}, {1000, 2}}, df.Durations...)
		df.Suffix = "μs"
		maxDur *= 1e6 // Converting seconds to μs
	} else {
		df.Suffix = "second"
	}

	for i := 0; i < len(df.Durations); i++ {
		level := df.Durations[i]
		if maxDur < level[0] {
			break
		}
		df.Divider *= float64(level[0])
		maxDur /= level[0]
		df.Suffix = getLevelName(level[1])
	}

	return df
}

func (df *DurationFormatter) format(samples int, sampleRate int, withUnits bool) string {
	//sampleRateOrig := sampleRate
	if df.EnableSubsecondPrecision {
		sampleRate /= 1e6
	}
	n := float64(samples) / float64(sampleRate) / df.Divider
	var nStr string
	if n == 0 {
		nStr = "0.00"
	} else if n >= 0 && n < 0.01 || n <= 0 && n > -0.01 {
		nStr = "< 0.01"
	} else {
		nStr = fmt.Sprintf("%.2f", n)
	}
	if withUnits {
		units := df.Units
		if units == "" {
			suffix := df.Suffix
			if n == 1 || len(suffix) == 2 {
				units = suffix
			} else {
				units = suffix + "s"
			}
		}
		return fmt.Sprintf("%s %s", nStr, units)
	}
	return nStr
}

func (df *DurationFormatter) formatPrecise(samples int, sampleRate int) string {
	//sampleRateOrig := sampleRate
	if df.EnableSubsecondPrecision {
		sampleRate /= 1e6
	}
	n := float64(samples) / float64(sampleRate) / df.Divider

	units := df.Units
	if units == "" {
		suffix := df.Suffix
		if n == 1 || len(suffix) == 2 {
			units = suffix
		} else {
			units = suffix + "s"
		}
	}
	return fmt.Sprintf("%.5f %s", n, units)
}

func (df DurationFormatter) getLevelName(level int) string {
	switch level {
	case 1:
		return "minute"
	case 2:
		return "hour"
	case 3:
		return "day"
	case 4:
		return "month"
	case 5:
		return "year"
	case 6:
		return "ms"
	default:
		return ""
	}
}

type NanosecondsFormatter struct {
	Divider    float64
	Multiplier int
	Suffix     string
	Durations  [][2]int
}

func NewNanosecondsFormatter(maxDur int) *NanosecondsFormatter {
	nf := &NanosecondsFormatter{
		Divider: 1,
	}
	nf.Durations = [][2]int{{60, 1}, {60, 2}, {24, 3}, {30, 4}, {12, 5}}

	maxDur /= 1e9

	for i := 0; i < len(nf.Durations); i++ {
		level := nf.Durations[i]
		if !levelExists(level) {
			break
		}

		if maxDur >= level[0] {
			nf.Divider *= float64(level[0])
			maxDur /= level[0]
			nf.Suffix = getLevelName(level[1])
		} else {
			break
		}
	}

	return nf
}

func (nf *NanosecondsFormatter) format(samples int) string {
	n := float64(samples) / nf.Divider / 1e9
	var nStr string
	if n >= 0 && n < 0.01 || n <= 0 && n > -0.01 {
		nStr = "< 0.01"
	} else {
		nStr = fmt.Sprintf("%.2f", n)
	}
	suffix := nf.Suffix
	if n == 1 {
		suffix = strings.TrimSuffix(suffix, "s")
	} else {
		suffix = suffix + "s"
	}
	return fmt.Sprintf("%s %s", nStr, suffix)
}

func (nf *NanosecondsFormatter) formatPrecise(samples int) string {
	n := float64(samples) / nf.Divider / 1e9

	suffix := nf.Suffix + getSuffixSuffix(nf.Suffix, n)
	return fmt.Sprintf("%.5f %s", n, suffix)
}

func getLevelName(level int) string {
	switch level {
	case 1:
		return "minute"
	case 2:
		return "hour"
	case 3:
		return "day"
	case 4:
		return "month"
	case 5:
		return "year"
	default:
		return ""
	}
}

func getSuffixSuffix(suffix string, n float64) string {
	if n != 1 {
		return "s"
	}
	return ""
}

var objects = []struct {
	Max    int
	Suffix string
}{
	{1000, "K"},
	{1000, "M"},
	{1000, "G"},
	{1000, "T"},
	{1000, "P"},
}

type ObjectsFormatter struct {
	Divider float64
	Suffix  string
}

func NewObjectsFormatter(maxObjects int) *ObjectsFormatter {
	of := &ObjectsFormatter{
		Divider: 1,
	}

	for i := 0; i < len(objects); i++ {
		level := objects[i]

		if maxObjects >= level.Max {
			of.Divider *= float64(level.Max)
			maxObjects /= level.Max
			of.Suffix = level.Suffix
		} else {
			break
		}
	}

	return of
}

func (of *ObjectsFormatter) format(samples int) string {
	n := float64(samples) / of.Divider
	var nStr string
	if n >= 0 && n < 0.01 || n <= 0 && n > -0.01 {
		nStr = "< 0.01"
	} else {
		nStr = fmt.Sprintf("%.2f", n)
	}
	return fmt.Sprintf("%s %s", nStr, of.Suffix)
}

func (of *ObjectsFormatter) formatPrecise(samples int) string {
	n := float64(samples) / of.Divider

	return fmt.Sprintf("%.5f %s", n, of.Suffix)
}

type BytesFormatter struct {
	Divider float64
	Suffix  string
	Bytes   [][2]int
}

func NewBytesFormatter(maxBytes int) *BytesFormatter {
	bf := &BytesFormatter{
		Divider: 1,
	}
	bf.Bytes = [][2]int{{1024, 1}, {1024, 2}, {1024, 3}, {1024, 4}, {1024, 5}}

	for i := 0; i < len(bf.Bytes); i++ {
		level := bf.Bytes[i]
		if !levelExists(level) {
			break
		}

		if maxBytes >= level[0] {
			bf.Divider *= float64(level[0])
			maxBytes /= level[0]
			bf.Suffix = bf.getLevelName(level[1])
		} else {
			break
		}
	}

	return bf
}

func (bf *BytesFormatter) format(samples int) string {
	n := float64(samples) / bf.Divider
	var nStr string
	if n >= 0 && n < 0.01 || n <= 0 && n > -0.01 {
		nStr = "< 0.01"
	} else {
		nStr = fmt.Sprintf("%.2f", n)
	}
	return fmt.Sprintf("%s %s", nStr, bf.Suffix)
}

func (bf *BytesFormatter) formatPrecise(samples int) string {
	n := float64(samples) / bf.Divider

	return fmt.Sprintf("%.5f %s", n, bf.Suffix)
}

func levelExists(level [2]int) bool {
	return level[0] > 0 && level[1] != 0
}

func (bf BytesFormatter) getLevelName(level int) string {
	switch level {
	case 1:
		return "KB"
	case 2:
		return "MB"
	case 3:
		return "GB"
	case 4:
		return "TB"
	case 5:
		return "PB"
	default:
		return ""
	}
}
