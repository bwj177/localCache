package personLocalCache

import (
	"context"
	"errors"
	"sync"
	"time"
)

type item struct { //缓存数据结构体  添加过期时间字段
	val        []byte
	expireTime time.Time
}

var (
	MissCacheErr  = errors.New("map cache:Get:未找到指定缓存")
	CloseCacheErr = errors.New("map cache:缓存已经关闭，请勿继续操作！")
)

type MapCache struct { // mapCache 实现Cache接口
	lock          sync.RWMutex                 //  避免并发读写
	data          map[string]*item             // 缓存数据 map
	onEvicted     func(key string, val []byte) //回调函数
	close         chan struct{}                // close信号，使用者可以主动关闭缓存
	closed        bool                         //close 标识
	cycleInterval time.Duration                //定期轮询
}

var _ Cache = (*MapCache)(nil)

// 初始化mapCache, Evicted与cycleInterval为可选参数，在此使用函数选项模式传参
type MapCacheOption func(m *MapCache)

func WithOnEvicatedCache(onEvicated func(key string, val []byte)) MapCacheOption {
	return func(m *MapCache) {
		m.onEvicted = onEvicated
	}
}

func WithCycleIntervalCache(cycleInterval time.Duration) MapCacheOption {
	return func(m *MapCache) {
		m.cycleInterval = cycleInterval
	}
}

func NewMapCache(opts ...MapCacheOption) *MapCache {
	cache := &MapCache{
		data:          make(map[string]*item),
		onEvicted:     nil,
		close:         make(chan struct{}),
		closed:        false,
		cycleInterval: 0,
	}
	for _, opt := range opts { //function option model
		opt(cache)
	}

	return cache
}

func (m *MapCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.lock.RLock() // 读锁避免map冲突
	val, ok := m.data[key]
	m.lock.Unlock()
	if !ok {
		return nil, MissCacheErr // miss cache
	}
	now := time.Now()
	if val.Expired(now) { // 处理 expire cache
		m.lock.Lock()
		defer m.lock.Unlock()
		val, ok = m.data[key] // double check 避免在没锁的空隙状态别人对该key又设置了Cache导致误删
		if !ok {
			return nil, MissCacheErr
		}
		if val.Expired(now) {
			m.delete(key)
			return nil, MissCacheErr
		}
	}
	return val.val, nil
}

func (m *MapCache) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	if m.closed {
		return CloseCacheErr
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	var dl time.Time
	if expiration > 0 {
		dl = time.Now().Add(expiration)
	}
	m.data[key] = &item{
		val:        value,
		expireTime: dl,
	}
	return nil
}

func (m *MapCache) Delete(ctx context.Context, key string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.closed { //如果已经关闭了就不让执行
		return CloseCacheErr
	}
	m.delete(key)
	return nil
}

func (m *MapCache) delete(key string) {
	val, ok := m.data[key]
	if ok {
		delete(m.data, key)
		//回调函数CDC
		if m.onEvicted != nil {
			m.onEvicted(key, val.val)
		}
	}
}

func (m *MapCache) LoadAndDelete(ctx context.Context, key string) ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	item, ok := m.data[key]
	if !ok {
		return nil, MissCacheErr
	}
	m.delete(key)
	return item.val, nil
}

func (i *item) Expired(now time.Time) bool {
	return i.expireTime.Before(now)
}

func (m *MapCache) CloseCache() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.close <- struct{}{} //发送关闭信号
	if m.onEvicted != nil {
		for k, v := range m.data {
			m.onEvicted(k, v.val) // 以此便利k，v执行回调函数
		}
	}
	m.data = nil
	return nil
}

func (m *MapCache) checkCycle(cnt int) { //定期轮询缓存k，v删除过期键值，避免大量过期kv占用内存,让用户自己去决定每次轮询多少的kv，如果kv过多 遍历一遍是非常浪费资源的
	go func() {
		ticker := time.NewTicker(m.cycleInterval)
		c := 0
		for {
			select {
			case now := <-ticker.C:
				m.lock.Lock()
				for k, v := range m.data {
					if !v.expireTime.IsZero() && v.Expired(now) { //如果设置了过期时间并且已经过期
						m.delete(k)
					}
					c += 1
					if c > cnt {
						break //遍历超次数退出
					}
				}
			case <-m.close:
				close(m.close)
				m.closed = true
				return
			}
		}
	}()
}
