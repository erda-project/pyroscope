package labels

import (
	"context"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/types"
)

type Labels struct {
	db types.ClickHouseDB
}

func New(db types.ClickHouseDB) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func (ll *Labels) PutLabels(labels map[string]string) error {
	batch, err := ll.db.NewWriteBatch("insert into " + ll.db.FQDN() + " values (?, ?, ?)")
	if err != nil {
		return err
	}
	for k, v := range labels {
		if err := batch.Append("l:"+k, nil, time.Now()); err != nil {
			return err
		}
		if err := batch.Append("v:"+k+":"+v, nil, time.Now()); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (ll *Labels) Put(key, val string) {
	kk := "l:" + key
	kv := "v:" + key + ":" + val
	// ks := "h:" + key + ":" + val + ":" + stree
	err := ll.db.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "insert into ? values (?, ?, ?)", ll.db.FQDN(), kk, nil, time.Now())
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
	err = ll.db.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "insert into ? values (?, ?, ?)", ll.db.FQDN(), kv, nil, time.Now())
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
	// err = ll.db.Update(func(txn *badger.Txn) error {
	// 	return txn.SetEntry(badger.NewEntry([]byte(ks), []byte{}))
	// })
	// if err != nil {
	// 	// TODO: handle
	// 	panic(err)
	// }
}

type TableRow struct {
	K         string    `db:"k" ch:"k"`
	V         string    `db:"v" ch:"v"`
	Timestamp time.Time `db:"timestamp" ch:"timestamp"`
}

//revive:disable-next-line:get-return A callback is fine
func (ll *Labels) GetKeys(cb func(k string) bool) {
	err := ll.db.View(func(conn clickhouse.Conn) error {
		rows, err := conn.Query(context.Background(), "select k, v, max(timestamp) as timestamp from "+ll.db.FQDN()+"_all where k like '?%' group by (k,v)", "l:")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kv TableRow
			if err := rows.ScanStruct(&kv); err != nil {
				return err
			}
			shouldContinue := cb(string(kv.V[2:]))
			if !shouldContinue {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
}

// Delete removes key value label pair from the storage.
// If the pair can not be found, no error is returned.
func (ll *Labels) Delete(key, value string) error {
	return ll.db.Update(func(conn clickhouse.Conn) error {
		return conn.Exec(context.Background(), "delete from "+ll.db.FQDN()+" where k = ?", "v:"+key+":"+value)
	})
}

//revive:disable-next-line:get-return A callback is fine
func (ll *Labels) GetValues(key string, cb func(v string) bool) {
	err := ll.db.View(func(conn clickhouse.Conn) error {
		rows, err := conn.Query(context.Background(), "select k, v, max(timestamp) as timestamp from "+ll.db.FQDN()+"_all where k like ? group by (k,v)", "v:"+key+":"+"%")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kv TableRow
			if err := rows.ScanStruct(&kv); err != nil {
				return err
			}
			k := kv.K
			ks := string(k)
			li := strings.LastIndex(ks, ":") + 1
			shouldContinue := cb(ks[li:])
			if !shouldContinue {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
}
