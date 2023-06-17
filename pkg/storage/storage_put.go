package storage

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type PutInput struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
	SampleType      string
}

func (s *Storage) Put(ctx context.Context, pi *PutInput) error {
	if s.hc.IsOutOfDiskSpace() {
		return errOutOfSpace
	}
	if pi.StartTime.Before(s.retentionPolicy().LowerTimeBoundary()) {
		return errRetention
	}

	if err := segment.ValidateKey(pi.Key); err != nil {
		return err
	}

	appList := strings.Split(pi.Key.AppName(), ".")
	if err := s.appSvc.CreateOrUpdate(ctx, appmetadata.ApplicationMetadata{
		FQName:          pi.Key.AppName(),
		SpyName:         pi.SpyName,
		SampleRate:      pi.SampleRate,
		Units:           pi.Units,
		AggregationType: pi.AggregationType,
		SampleType:      appList[len(appList)-1],
		OrgID:           pi.Key.Labels()["DICE_ORG_ID"],
		OrgName:         pi.Key.Labels()["DICE_ORG_NAME"],
		Workspace:       pi.Key.Labels()["DICE_WORKSPACE"],
		ProjectID:       pi.Key.Labels()["DICE_PROJECT_ID"],
		ProjectName:     pi.Key.Labels()["DICE_PROJECT_NAME"],
		AppID:           pi.Key.Labels()["DICE_APPLICATION_ID"],
		AppName:         pi.Key.Labels()["DICE_APPLICATION_NAME"],
		ClusterName:     pi.Key.Labels()["DICE_CLUSTER_NAME"],
		ServiceName:     pi.Key.Labels()["DICE_SERVICE"],
		PodIP:           pi.Key.Labels()["POD_IP"],
	}); err != nil {
		s.logger.Error("error saving metadata", err)
	}

	s.putTotal.Inc()
	if pi.Key.HasProfileID() {
		if err := s.ensureAppSegmentExists(pi); err != nil {
			return err
		}
		return s.exemplars.insert(ctx, pi)
	}

	s.logger.WithFields(logrus.Fields{
		"startTime":       pi.StartTime.String(),
		"endTime":         pi.EndTime.String(),
		"key":             pi.Key.Normalized(),
		"samples":         pi.Val.Samples(),
		"units":           pi.Units,
		"aggregationType": pi.AggregationType,
	}).Debug("storage.Put")

	if err := s.labels.PutLabels(pi.Key.Labels()); err != nil {
		return fmt.Errorf("unable to write labels: %w", err)
	}

	sk := pi.Key.SegmentKey()
	//for k, v := range pi.Key.Labels() {
	//	key := k + ":" + v
	//	r, err := s.dimensions.GetOrCreate(key)
	//	if err != nil {
	//		s.logger.Errorf("dimensions cache for %v: %v", key, err)
	//		continue
	//	}
	//	r.(*dimension.Dimension).Insert([]byte(sk))
	//	s.dimensions.Put(key, r)
	//}

	skWithTime := fmt.Sprintf("%s:%d", sk, pi.EndTime.Unix())
	r := s.segments.New(skWithTime)

	st := r.(*segment.Segment)
	st.SetMetadata(metadata.Metadata{
		SpyName:         pi.SpyName,
		SampleRate:      pi.SampleRate,
		Units:           pi.Units,
		AggregationType: pi.AggregationType,
	})

	samples := pi.Val.Samples()
	err := st.Put(pi.StartTime, pi.EndTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := pi.Key.TreeKey(depth, t)
		res := s.trees.New(tk)
		cachedTree := res.(*tree.Tree)
		treeClone := pi.Val.Clone(r)
		for _, addon := range addons {
			if res, ok := s.trees.Lookup(pi.Key.TreeKey(addon.Depth, addon.T)); ok {
				ta := res.(*tree.Tree)
				ta.RLock()
				treeClone.Merge(ta)
				ta.RUnlock()
			}
		}
		cachedTree.Lock()
		cachedTree.Merge(treeClone)
		cachedTree.Unlock()
		s.trees.Put(tk, cachedTree)
	})
	if err != nil {
		return err
	}

	s.segments.Put(skWithTime, st)
	return nil
}
