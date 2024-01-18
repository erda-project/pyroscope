package cache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/storage/types"
	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu"
)

type Cache struct {
	ch      types.ClickHouseDB
	lfu     *lfu.Cache
	metrics *Metrics
	codec   Codec

	prefix string
	ttl    time.Duration

	evictionsDone chan struct{}
	writeBackDone chan struct{}
	flushOnce     sync.Once
}

type ClickHouseConfig struct {
	types.ClickHouseDB
	*Metrics
	Codec

	// Prefix for badger DB keys.
	Prefix string
	// TTL specifies number of seconds an item can reside in cache after
	// the last access. An obsolete item is evicted. Setting TTL to less
	// than a second disables time-based eviction.
	TTL time.Duration
}

// ClickHouseCodec is a shorthand of coder-decoder. A Codec implementation
// is responsible for type conversions and binary representation.
type ClickHouseCodec interface {
	Serialize(w io.Writer, key string, value interface{}) error
	Deserialize(r io.Reader, key string) (interface{}, error)
	// New returns a new instance of the type. The method is
	// called by GetOrCreate when an item can not be found by
	// the given key.
	New(key string) interface{}
}

// Codec is a shorthand of coder-decoder. A Codec implementation
// is responsible for type conversions and binary representation.
type Codec interface {
	Serialize(w io.Writer, key string, value interface{}) error
	Deserialize(r io.Reader, key string) (interface{}, error)
	DeserializeWithTime(r io.Reader, _ string, _ time.Time) (interface{}, error)
	SerializeWithTime(w io.Writer, _ string, v interface{}, t time.Time) error
	// New returns a new instance of the type. The method is
	// called by GetOrCreate when an item can not be found by
	// the given key.
	New(key string) interface{}
}

type Metrics struct {
	MissesCounter     prometheus.Counter
	ReadsCounter      prometheus.Counter
	DBWrites          prometheus.Observer
	DBReads           prometheus.Observer
	WriteBackDuration prometheus.Observer
	EvictionsDuration prometheus.Observer
}

func NewClickHouse(c ClickHouseConfig) *Cache {
	cache := &Cache{
		lfu:           lfu.New(),
		ch:            c.ClickHouseDB,
		codec:         c.Codec,
		metrics:       c.Metrics,
		prefix:        c.Prefix,
		ttl:           c.TTL,
		evictionsDone: make(chan struct{}),
		writeBackDone: make(chan struct{}),
	}

	evictionChannel := make(chan lfu.Eviction)
	writeBackChannel := make(chan lfu.Eviction)

	// eviction channel for saving cache items to disk
	cache.lfu.EvictionChannel = evictionChannel
	cache.lfu.WriteBackChannel = writeBackChannel
	cache.lfu.TTL = int64(c.TTL.Seconds())

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for e := range evictionChannel {
			// TODO(kolesnikovae): these errors should be at least logged.
			//  Perhaps, it will be better if we move it outside of the cache.
			//  Taking into account that writes almost always happen in batches,
			//  We should definitely take advantage of BadgerDB write batch API.
			//  Also, WriteBack and Evict could be combined. We also could
			//  consider moving caching to storage/db.
			//if err := cache.saveToDisk(e.Key, e.Value); err != nil {
			//	log.Println("error saving to disk:", err)
			//}
			log.Println("evicting", e.Key)
		}
		close(cache.evictionsDone)
	}()

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for e := range writeBackChannel {
			if err := cache.saveToDisk(e.Key, e.Value); err != nil {
				log.Println("error write back to disk:", err)
			}
		}
		close(cache.writeBackDone)
	}()

	return cache
}

func (cache *Cache) Put(key string, val interface{}) {
	if err := cache.saveToDisk(key, val); err != nil {
		logrus.Errorf("error saving to disk, key: %s, err: %v", key, err)
	}
	cache.lfu.Set(key, val)
}

func (cache *Cache) PutWithTime(key string, val interface{}, t time.Time) {
	if err := cache.saveToDiskWithTime(key, val, t); err != nil {
		logrus.Errorf("error saving to disk, key: %s, err: %v", key, err)
	}
	cache.lfu.Set(key+fmt.Sprintf(":%d", t.Unix()), val)
}

func (cache *Cache) saveToDisk(key string, val interface{}) error {
	b := bytebufferpool.Get()
	defer bytebufferpool.Put(b)
	if err := cache.codec.Serialize(b, key, val); err != nil {
		return fmt.Errorf("serialization: %w", err)
	}
	cache.metrics.DBWrites.Observe(float64(b.Len()))
	return cache.ch.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "insert into "+cache.ch.FQDN()+" values (?, ?, ?)", cache.prefix+key, b.String(), time.Now())
	})
}

func (cache *Cache) saveToDiskWithTime(key string, val interface{}, t time.Time) error {
	b := bytebufferpool.Get()
	defer bytebufferpool.Put(b)
	if err := cache.codec.SerializeWithTime(b, key, val, t); err != nil {
		return fmt.Errorf("serialization: %w", err)
	}
	cache.metrics.DBWrites.Observe(float64(b.Len()))
	return cache.ch.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "insert into "+cache.ch.FQDN()+" values (?, ?, ?)", cache.prefix+key, b.String(), t)
	})
}

func (cache *Cache) Sync() error {
	return cache.lfu.Iterate(func(k string, v interface{}) error {
		return cache.saveToDisk(k, v)
	})
}

func (cache *Cache) Flush() {
	cache.flushOnce.Do(func() {
		// Make sure there is no pending items.
		close(cache.lfu.WriteBackChannel)
		<-cache.writeBackDone
		// evict all the items in cache
		cache.lfu.Evict(cache.lfu.Len())
		close(cache.lfu.EvictionChannel)
		// wait until all evictions are done
		<-cache.evictionsDone
	})
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (cache *Cache) Evict(percent float64) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.EvictionsDuration.Observe))
	cache.lfu.Evict(int(float64(cache.lfu.Len()) * percent))
	timer.ObserveDuration()
}

func (cache *Cache) WriteBack() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.WriteBackDuration.Observe))
	cache.lfu.WriteBack()
	timer.ObserveDuration()
}

func (cache *Cache) Delete(key string) error {
	cache.lfu.Delete(key)
	return cache.ch.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "delete from "+cache.ch.FQDN()+" where key = ?", cache.prefix+key)
	})
}

func (cache *Cache) Discard(key string) {
	cache.lfu.Delete(key)
}

// DiscardPrefix deletes all data that matches a certain prefix
// In both cache and database
func (cache *Cache) DiscardPrefix(prefix string) error {
	cache.lfu.DeletePrefix(prefix)
	return cache.dropPrefixCH(cache.prefix + prefix)
}

func (cache *Cache) dropPrefixCH(prefix string) error {
	var err error
	for more := true; more; {
		if more, err = cache.dropPrefixBatchCH(prefix); err != nil {
			return err
		}
	}
	return nil
}

func (cache *Cache) dropPrefixBatchCH(prefix string) (bool, error) {
	err := cache.ch.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "delete from "+cache.ch.FQDN()+" where key like '?%'", prefix)
	})
	return false, err
}

func (cache *Cache) GetOrCreate(key string) (interface{}, error) {
	return cache.get(key, true)
}

func (cache *Cache) GetOrCreateWithTime(key string, t time.Time) (interface{}, error) {
	return cache.getWithTime(key, t, true)
}

func (cache *Cache) Lookup(key string) (interface{}, bool) {
	v, err := cache.get(key, false)
	return v, v != nil && err == nil
}

func (cache *Cache) LookupWithTime(key string, t time.Time) (interface{}, bool) {
	v, err := cache.getWithTime(key, t, false)
	return v, v != nil && err == nil
}

func (cache *Cache) LookupWithTimeLimit(key string, st, et time.Time, limit int) ([]interface{}, error) {
	var rows driver.Rows
	var err error
	//interval := (et.Unix() - st.Unix()) / int64(limit)
	err = cache.ch.View(func(conn clickhouse.Conn) error {
		rows, err = conn.Query(context.Background(), "select k, v, timestamp from "+cache.ch.FQDN()+"_all where k = ? and timestamp >= ? and timestamp <= ? order by timestamp asc", cache.prefix+key, st, et)
		return err
	})
	if err != nil {
		return nil, err
	}
	res := make([]interface{}, 0)
	for rows.Next() {
		var row TableRow
		if err := rows.ScanStruct(&row); err != nil {
			return nil, err
		}
		var val interface{}
		val, err = cache.codec.Deserialize(bytes.NewBufferString(row.V), strings.TrimPrefix(row.K, cache.prefix)+":"+fmt.Sprintf("%d", row.Timestamp.Unix()))
		if err != nil {
			return nil, err
		}
		res = append(res, val)
		cache.lfu.Set(strings.TrimPrefix(row.K, cache.prefix)+":"+fmt.Sprintf("%d", row.Timestamp.Unix()), val)
	}
	return res, nil
}

func (cache *Cache) LookupByKeys(keys []string, st, et time.Time, limit int) (map[string][]interface{}, error) {
	var rows driver.Rows
	var err error
	keysWithPrefix := make([]string, 0, len(keys))
	for _, k := range keys {
		keysWithPrefix = append(keysWithPrefix, cache.prefix+k)
	}
	interval := (et.Unix() - st.Unix()) / int64(limit)
	err = cache.ch.View(func(conn clickhouse.Conn) error {
		rows, err = conn.Query(context.Background(), "select first_value(k) as firstk, first_value(v) as v, toStartOfInterval(timestamp, INTERVAL "+fmt.Sprintf("%d", interval)+" second) as timestamp from "+cache.ch.FQDN()+"_all where k in ? and timestamp >= ? and timestamp <= ? group by k, timestamp order by timestamp asc", keysWithPrefix, st, et)
		return err
	})
	if err != nil {
		return nil, err
	}
	res := make(map[string][]interface{}, 0)
	for rows.Next() {
		var row TableRow
		if err := rows.ScanStruct(&row); err != nil {
			return nil, err
		}
		var val interface{}
		val, err = cache.codec.Deserialize(bytes.NewBufferString(row.V), strings.TrimPrefix(row.FirstK, cache.prefix)+":"+fmt.Sprintf("%d", row.Timestamp.Unix()))
		if err != nil {
			return nil, err
		}
		k := strings.TrimPrefix(row.FirstK, cache.prefix)
		if _, ok := res[k]; !ok {
			res[k] = make([]interface{}, 0)
		}
		res[k] = append(res[k], val)
		cache.lfu.Set(k+":"+fmt.Sprintf("%d", row.Timestamp.Unix()), val)
	}
	return res, nil
}

type TableRow struct {
	FirstK    string    `db:"firstk" ch:"firstk"`
	K         string    `db:"k" ch:"k"`
	V         string    `db:"v" ch:"v"`
	Timestamp time.Time `db:"timestamp" ch:"timestamp"`
}

func (cache *Cache) iterate(key string, createNotFound bool) (interface{}, error) {
	cache.metrics.ReadsCounter.Inc()
	return cache.lfu.GetOrSet(key, func() (interface{}, error) {
		cache.metrics.MissesCounter.Inc()
		var rows driver.Rows
		var err error
		err = cache.ch.View(func(conn clickhouse.Conn) error {
			rows, err = conn.Query(context.Background(), "select k, v, max(timestamp) as timestamp from "+cache.ch.FQDN()+"_all where k = ? group by (k,v) limit 1", cache.prefix+key)
			return err
		})
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		recordNotFound := true
		var row TableRow
		if rows.Next() {
			recordNotFound = false
			if err := rows.ScanStruct(&row); err != nil {
				return nil, err
			}
		}
		if recordNotFound && createNotFound {
			return cache.codec.New(key), nil
		}

		cache.metrics.DBReads.Observe(float64(len(row.V)))
		return cache.codec.Deserialize(bytes.NewBufferString(row.V), key)
	})
}

func (cache *Cache) New(key string) interface{} {
	return cache.codec.New(key)
}

func (cache *Cache) get(key string, createNotFound bool) (interface{}, error) {
	cache.metrics.ReadsCounter.Inc()
	return cache.lfu.GetOrSet(key, func() (interface{}, error) {
		cache.metrics.MissesCounter.Inc()
		var row TableRow
		var buf []byte
		err := cache.ch.View(func(conn clickhouse.Conn) error {
			if err := conn.QueryRow(context.Background(), "select k, v, max(timestamp) as timestamp from "+cache.ch.FQDN()+"_all where k = ? group by (k,v) limit 1", cache.prefix+key).ScanStruct(&row); err != nil {
				return err
			}
			buf = []byte(row.V)
			return nil
		})
		switch {
		default:
			return nil, err
		case err == nil:
		case err != nil:
			if createNotFound {
				return cache.codec.New(key), nil
			}
			return nil, nil
		}
		cache.metrics.DBReads.Observe(float64(len(buf)))
		return cache.codec.Deserialize(bytes.NewReader(buf), key)
	})
}

func (cache *Cache) getWithTime(key string, t time.Time, createNotFound bool) (interface{}, error) {
	return cache.lfu.GetOrSet(key+fmt.Sprintf(":%d", t.Unix()), func() (interface{}, error) {
		cache.metrics.MissesCounter.Inc()
		var row TableRow
		var buf []byte
		err := cache.ch.View(func(conn clickhouse.Conn) error {
			if err := conn.QueryRow(context.Background(), "select k, v, timestamp from "+cache.ch.FQDN()+"_all where k = ? and timestamp= ? limit 1", cache.prefix+key, t).ScanStruct(&row); err != nil {
				return err
			}
			buf = []byte(row.V)
			return nil
		})
		switch {
		default:
			return nil, err
		case err == nil:
		case err != nil:
			if createNotFound {
				return cache.codec.New(key), nil
			}
			return nil, nil
		}
		cache.metrics.DBReads.Observe(float64(len(buf)))
		return cache.codec.DeserializeWithTime(bytes.NewReader(buf), key, t)
	})
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}
