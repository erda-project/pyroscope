package selfprofiling

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/grafana/pyroscope-go/upstream"

	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func NewSession(logger pyroscope.Logger, ingester ingestion.Ingester, appName string, tags map[string]string) *pyroscope.Session {
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	session, _ := pyroscope.NewSession(pyroscope.SessionConfig{
		Upstream: NewUpstream(logger, ingester),
		AppName:  appName,
		ProfilingTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
		SampleRate: 100,
		UploadRate: 10 * time.Second,
		Logger:     logger,
		Tags:       tags,
	})
	return session
}

type Upstream struct {
	logger   pyroscope.Logger
	ingester ingestion.Ingester
}

func NewUpstream(logger pyroscope.Logger, ingester ingestion.Ingester) *Upstream {
	return &Upstream{
		logger:   logger,
		ingester: ingester,
	}
}

func (u *Upstream) Upload(j *upstream.UploadJob) {
	defer func() {
		if r := recover(); r != nil {
			u.logger.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()

	key, err := segment.ParseKey(j.Name)
	if err != nil {
		u.logger.Errorf("invalid key %q: %v", j.Name, err)
		return
	}

	if len(j.Profile) == 0 {
		return
	}

	profile := &pprof.RawProfile{
		Profile:          j.Profile,
		SampleTypeConfig: tree.DefaultSampleTypeMapping,
	}
	if j.SampleTypeConfig != nil {
		profile.SampleTypeConfig = sampleTypeConfigFromUpstream(j.SampleTypeConfig)
	}
	if len(j.PrevProfile) > 0 {
		profile.PreviousProfile = j.PrevProfile
	}

	err = u.ingester.Ingest(context.TODO(), &ingestion.IngestInput{
		Profile: profile,
		Metadata: ingestion.Metadata{
			SpyName:   j.SpyName,
			StartTime: j.StartTime,
			EndTime:   j.EndTime,
			Key:       key,
		},
	})

	if err != nil {
		u.logger.Errorf("failed to store a local profile: %v", err)
	}
}

func (*Upstream) Flush() {

}

func sampleTypeConfigFromUpstream(config map[string]*upstream.SampleType) map[string]*tree.SampleTypeConfig {
	res := make(map[string]*tree.SampleTypeConfig)
	for k, v := range config {
		vv := &tree.SampleTypeConfig{
			Units:       metadata.Units(v.Units),
			DisplayName: v.DisplayName,
			Aggregation: metadata.AggregationType(v.Aggregation),
			Cumulative:  v.Cumulative,
			Sampled:     v.Sampled,
		}
		res[k] = vv
	}
	return res
}
