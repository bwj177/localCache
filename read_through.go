package personLocalCache

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type ReadThroughCache struct {
	Cache
	mutex sync.RWMutex

	Expiration time.Duration
	// 我们把最常见的”捞DB”这种说法抽象为”加载数据”
	LoadFunc func(ctx context.Context, key string) ([]byte, error)
	// Loader

}

func (c *ReadThroughCache) Get(ctx context.Context, key string) (any, error) {
	// 逻辑：
	// 先捞缓存
	// 再捞 DB
	c.mutex.RLock()
	val, err := c.Cache.Get(ctx, key)
	c.mutex.RUnlock()
	if err != nil && err != MissCacheErr {
		// 这个代表出问题了，但是不知道哪里出问题了
		return nil, err
	}

	// 缓存没有数据
	if err == MissCacheErr {

		c.mutex.Lock()
		defer c.mutex.Unlock()

		val, err = c.LoadFunc(ctx, key)

		if err != nil {

			// 这里会暴露 LoadFunc 底层
			// 例如如果 LoadFunc 是数据库查询，这里就会暴露数据库的错误信息（或者 ORM 框架的）
			return nil, fmt.Errorf("cache: 无法加载数据, %w", err)
		}

		// 这里开 goroutine 就是半异步

		// 这里 err 可以考虑忽略掉，或者输出 warn 日志
		err = c.Cache.Set(ctx, key, val, c.Expiration)

		// 可能的结果: goroutine1 先，毫无问题，数据库和缓存是一致的
		// goroutine2 先，那就有问题了, DB 是 value2，但是 cache 是 value1
		if err != nil {
			log.Fatalln(err)
		}
		return val, nil
	}
	return val, nil
}

func (c *ReadThroughCache) Set(ctx context.Context, key string, val []byte, expiration time.Duration) error {
	// 你加不加锁，数据都可能不一致
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.Cache.Set(ctx, key, val, expiration)
}
