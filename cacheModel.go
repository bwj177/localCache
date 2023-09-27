package personLocalCache

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type StrategyCache interface { //缓存模式策略接口，，策略不同只会影响get set
	Get(ctx context.Context, key string, cache Cache) ([]byte, error)
	Set(ctx context.Context, key string, val []byte, expiration time.Duration, cache Cache) error
}

type CacheModel struct { //策略模式选择不同缓存模式
	Cache
	StrategyCache //缓存模式策略  根据需求不同可更改不同的策略
}

func (m *CacheModel) Get(ctx context.Context, key string) ([]byte, error) { //在外层封装Get，屏蔽内部的细节
	res, err := m.StrategyCache.Get(ctx, key, m.Cache)
	return res, err
}

func (m *CacheModel) Set(ctx context.Context, key string, val []byte, expiration time.Duration) error {
	return m.StrategyCache.Set(ctx, key, val, expiration, m.Cache)
}

func NewCacheModel(cache Cache, strategyCache StrategyCache) *CacheModel {
	return &CacheModel{
		Cache:         cache,
		StrategyCache: strategyCache}
}

func NewReadThroughStrategyCache(Expiration time.Duration, LoadFunc func(ctx context.Context, key string) ([]byte, error)) StrategyCache {
	return &ReadThroughStrategyCache{
		Expiration: Expiration,
		LoadFunc:   LoadFunc,
	}
}

type ReadThroughStrategyCache struct {
	mutex sync.RWMutex

	Expiration time.Duration
	// 我们把最常见的”捞DB”这种说法抽象为”加载数据”
	LoadFunc func(ctx context.Context, key string) ([]byte, error)
}

func (c *ReadThroughStrategyCache) Get(ctx context.Context, key string, cache Cache) ([]byte, error) {
	// 逻辑：
	// 先捞缓存
	// 再捞 DB
	c.mutex.RLock()
	val, err := cache.Get(ctx, key)
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
		err = cache.Set(ctx, key, val, c.Expiration)

		// 可能的结果: goroutine1 先，毫无问题，数据库和缓存是一致的
		// goroutine2 先，那就有问题了, DB 是 value2，但是 cache 是 value1
		if err != nil {
			log.Fatalln(err)
		}
		return val, nil
	}
	return val, nil
}

func (c *ReadThroughStrategyCache) Set(ctx context.Context, key string, val []byte, expiration time.Duration, cache Cache) error {
	// 你加不加锁，数据都可能不一致
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return cache.Set(ctx, key, val, expiration)
}
