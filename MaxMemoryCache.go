package personLocalCache

import (
	"context"
	"errors"
	"github.com/gotomicro/ekit/list"
	"sync"
	"time"
)

var (
	errOverCapacity     = errors.New("cache: 超过缓存最大容量")
	errFailedToSetCache = errors.New("cache: 设置键值对失败")
)

type MaxMemoryCache struct {
	*MapCache
	max   int64 // cache最大容量
	used  int64 // cache已使用容量
	mutex *sync.Mutex
	keys  *list.LinkedList[string] //LinkedList实现LRU
}

func NewMaxMemoryCache(c *MapCache, max int64) *MaxMemoryCache {
	res := &MaxMemoryCache{
		MapCache: c,
		max:      max,
	}
	origin := c.onEvicted
	c.onEvicted = func(key string, val []byte) { //回调函数，在缓存删除中执行让cnt计数减一并让linklist删除该key
		res.evicted(key, val)
		if origin != nil {
			origin(key, val)
		}
	}
	return res
}

func (m *MaxMemoryCache) Set(ctx context.Context, key string, val []byte,
	expiration time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 也可以用 Get，但是要记得调整 keys 和计算容量变化差值
	_, _ = m.MapCache.LoadAndDelete(ctx, key)
	for m.used+int64(len(val)) > m.max {
		k, err := m.keys.Get(0)
		if err != nil {
			return err
		}
		_ = m.MapCache.Delete(ctx, k)
	}
	err := m.MapCache.Set(ctx, key, val, expiration)
	if err == nil {
		m.used = m.used + int64(len(val))
		_ = m.keys.Append(key)
	}

	return nil
}

func (c *MaxMemoryCache) Get(ctx context.Context, key string) ([]byte, error) {

	val, err := c.MapCache.Get(ctx, key)
	if err == nil {
		// 把原本的删掉
		// 然后将 key 加到末尾
		c.deleteKey(key)
		_ = c.keys.Append(key)
	}
	return val, err
}

func (m *MaxMemoryCache) evicted(key string, val []byte) { //重写回调函数
	m.used = m.used - int64(len(val))
	m.deleteKey(key)
}

func (m *MaxMemoryCache) deleteKey(key string) { //删除linklist指定的key
	for i := 0; i < m.keys.Len(); i++ {
		ele, _ := m.keys.Get(i)
		if ele == key {
			_, _ = m.keys.Delete(i)
			return
		}
	}
}
