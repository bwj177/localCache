package personLocalCache

import (
	"context"
	"log"
)

type WriteBackCache struct {
	*MapCache
}

func NewWriteBackCache(store func(ctx context.Context, key string, val []byte) error, cache *MapCache) *WriteBackCache {
	c := &WriteBackCache{
		MapCache: cache,
	}
	origin := c.onEvicted
	c.onEvicted = func(key string, val []byte) { //重写回调函数，在cache过期后将其载入数据库
		err := store(context.Background(), key, val)
		if err != nil {
			log.Fatalln(err)
		}
		origin(key, val) //调用原来的
	}
	return c
}

func (w *WriteBackCache) Close() error {
	// 遍历所有的 key，将值刷新到数据库
	for k, v := range w.data {
		w.onEvicted(k, v.val) //将所有key刷新入db
	}
	return nil
}
