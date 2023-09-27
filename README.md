# 简介：
该项目基于Go实现，目前已实现本地Cache主要API，Get，Set，过期删除，内存淘汰，缓存模式，避免缓存异常...
项目主要设计风格为装饰器模式，无侵入式的去装饰实现新的API

顶级接口：
Cache

           -MapCache
            -MaxMemoryCache
            -MaxCntCache
                -read_through
                -write_through
                -write_back
                    -bloomFilterCache
                    ...


未实现：分布式节点，一致性哈希+singlefilght缓解一致性问题；分布式锁实现（关于过期时间，续约，加锁重试...)
