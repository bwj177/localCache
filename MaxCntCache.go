package personLocalCache

import (
	"context"
	"github.com/gotomicro/ekit/list"
	"sync"
	"sync/atomic"
	"time"
)

type MaxCntCache struct {
	*MapCache
	mutex  sync.Mutex
	MaxCnt int32
	Cnt    int32
	keys   *list.LinkedList[string] //LinkedList实现LRU
}

func NewMaxCntCache(c *MapCache, maxCnt int32) *MaxCntCache {
	res := &MaxCntCache{
		MapCache: c,
		MaxCnt:   maxCnt,
	}
	origin := c.onEvicted
	c.onEvicted = func(key string, val []byte) { //回调函数，在缓存删除中执行让cnt计数减一并让linklist删除该key
		atomic.AddInt32(&res.Cnt, -1)
		res.deleteKey(key)
		if origin != nil {
			origin(key, val)
		}
	}
	return res
}

func (c *MaxCntCache) Set(ctx context.Context,
	key string, val []byte, expiration time.Duration) error {

	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, err := c.MapCache.Get(ctx, key)
	if err != nil && err != MissCacheErr {
		// 这个错误比较棘手
		return err
	}
	if err == MissCacheErr {
		// 判断有没有超过最大值
		cnt := atomic.AddInt32(&c.Cnt, 1)
		// 满了
		if cnt > c.MaxCnt {
			atomic.AddInt32(&c.Cnt, -1) //因为次数超限制，因此将先前的+1回滚
			// 开始lur淘汰策略
			c.keys.Delete(0) //删除最前端的key
			atomic.AddInt32(&c.Cnt, -1)
			return errOverCapacity
		}
	}
	_ = c.keys.Append(key) //将新添加的key放入lru队列最末尾
	return c.MapCache.Set(ctx, key, val, expiration)
}

func (c *MaxCntCache) Get(ctx context.Context, key string) ([]byte, error) {

	val, err := c.MapCache.Get(ctx, key)
	if err == nil {
		// 把原本的删掉
		// 然后将 key 加到末尾
		c.deleteKey(key)
		_ = c.keys.Append(key)
	}
	return val, err
}

func (m *MaxCntCache) deleteKey(key string) { // 删除linklist中指定key
	for i := 0; i < m.keys.Len(); i++ {
		ele, _ := m.keys.Get(i)
		if ele == key {
			_, _ = m.keys.Delete(i)
			return
		}
	}
}
