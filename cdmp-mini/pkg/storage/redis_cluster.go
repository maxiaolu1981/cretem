package storage

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	redis "github.com/go-redis/redis/v8"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/spf13/viper"
)

// ErrRedisIsDown is returned when we can't communicate with redis.
var ErrRedisIsDown = errors.New("storage: Redis is either down or not configured")

var (
	singlePool      atomic.Value
	singleCachePool atomic.Value
	redisUp         atomic.Value
)

var disableRedis atomic.Value

// DisableRedis allows dynamically enabling/disabling redis communication (useful for testing)
func DisableRedis(ok bool) {
	if ok {
		redisUp.Store(false)
		disableRedis.Store(true)
		return
	}
	redisUp.Store(true)
	disableRedis.Store(false)
}

func shouldConnect() bool {
	if v := disableRedis.Load(); v != nil {
		return !v.(bool)
	}
	return true
}

// Connected returns true if we are connected to redis.
func Connected() bool {
	if v := redisUp.Load(); v != nil {
		return v.(bool)
	}
	return false
}

func singleton(cache bool) redis.UniversalClient {
	if cache {
		v := singleCachePool.Load()
		if v != nil {
			return v.(redis.UniversalClient)
		}
		return nil
	}
	if v := singlePool.Load(); v != nil {
		return v.(redis.UniversalClient)
	}
	return nil
}

// nolint: unparam
func connectSingleton(cache bool, config *options.RedisOptions) bool {
	if existing := singleton(cache); existing != nil {
		return true
	}

	log.Debug("Connecting to redis cluster")

	client := NewRedisClusterPool(cache, config)
	if client == nil {
		log.Error("Redis cluster client creation returned nil, will retry")
		return false
	}

	if cache {
		singleCachePool.Store(client)
	} else {
		singlePool.Store(client)
	}
	return true
}

// RedisCluster is a storage manager that uses the redis database.
type RedisCluster struct {
	KeyPrefix string
	HashKeys  bool
	IsCache   bool
}

func clusterConnectionIsOpen(cluster RedisCluster) bool {

	c := singleton(cluster.IsCache)
	if c == nil {
		log.Warn("clusterConnectionIsOpen: client instance is nil")
		return false
	}

	const maxAttempts = 5
	const attemptInterval = 300 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.Ping(pingCtx).Err()
		pingCancel()
		if err == nil {
			if attempt > 1 {
				log.Debugf("Redis cluster ping成功（第%d次尝试）", attempt)
			}
			return true
		}

		lastErr = err
		if errors.Is(err, context.DeadlineExceeded) {
			log.Debugf("Redis cluster ping第%d次尝试超时，继续重试", attempt)
		} else {
			log.Debugf("Redis cluster ping第%d次尝试失败: %v", attempt, err)
		}

		time.Sleep(attemptInterval)
	}

	if lastErr != nil {
		log.Warnf("Redis cluster health check failed after %d attempts: %v", maxAttempts, lastErr)
	} else {
		log.Warnf("Redis cluster health check failed after %d attempts", maxAttempts)
	}
	return false
}

// ConnectToRedis starts a go routine that periodically tries to connect to redis.
func ConnectToRedis(ctx context.Context, config *options.RedisOptions) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	c := []RedisCluster{
		{}, {IsCache: true},
	}
	var ok bool
	for _, v := range c {
		if !connectSingleton(v.IsCache, config) {
			break
		}
		if !clusterConnectionIsOpen(v) {
			redisUp.Store(false)
			break
		}
		ok = true
	}
	redisUp.Store(ok)
again:
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !shouldConnect() {
				continue
			}
			for _, v := range c {
				if !connectSingleton(v.IsCache, config) {
					redisUp.Store(false)
					goto again
				}
				if !clusterConnectionIsOpen(v) {
					redisUp.Store(false)
					goto again
				}
			}
			redisUp.Store(true)
		}
	}
}

// NewRedisClusterPool creates a redis cluster pool.
func NewRedisClusterPool(isCache bool, config *options.RedisOptions) redis.UniversalClient {
	log.Debug("Creating new Redis connection pool")

	poolSize := 500
	if config.MaxActive > 0 {
		poolSize = config.MaxActive
	}

	timeout := 5 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	var tlsConfig *tls.Config
	if config.UseSSL {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: config.SSLInsecureSkipVerify,
		}
	}

	var client redis.UniversalClient
	opts := &RedisOpts{
		Addrs:        getRedisAddrs(config),
		MasterName:   config.MasterName,
		Password:     config.Password,
		DB:           config.Database,
		DialTimeout:  timeout,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		IdleTimeout:  240 * timeout,
		PoolSize:     poolSize,
		TLSConfig:    tlsConfig,
	}

	if opts.MasterName != "" {
		log.Debug("--> [REDIS] Creating sentinel-backed failover client")
		client = redis.NewFailoverClient(opts.failover())
	} else if config.EnableCluster {
		log.Debug("--> [REDIS] Creating cluster client")
		client = redis.NewClusterClient(opts.cluster())
	} else {
		log.Debug("--> [REDIS] Creating single-node client")
		client = redis.NewClient(opts.simple())
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Errorf("新创建的Redis客户端验证失败: %v", err)
		_ = client.Close()
		return nil
	}
	//log.Info("新创建的Redis客户端验证成功")
	return client
}

func getRedisAddrs(config *options.RedisOptions) (addrs []string) {
	if len(config.Addrs) != 0 {
		addrs = config.Addrs
	}
	if len(addrs) == 0 && config.Port != 0 {
		addr := config.Host + ":" + strconv.Itoa(config.Port)
		addrs = append(addrs, addr)
	}
	return addrs
}

// RedisOpts wraps redis.UniversalOptions
type RedisOpts redis.UniversalOptions

func (o *RedisOpts) cluster() *redis.ClusterOptions {
	if len(o.Addrs) == 0 {
		o.Addrs = []string{"192.168.10.14:6379",
			"192.168.10.14:6380",
			"192.168.10.14:6381"}
	}
	return &redis.ClusterOptions{
		Addrs:     o.Addrs,
		OnConnect: o.OnConnect,

		Password: o.Password,

		MaxRedirects:   o.MaxRedirects,
		ReadOnly:       o.ReadOnly,
		RouteByLatency: o.RouteByLatency,
		RouteRandomly:  o.RouteRandomly,

		MaxRetries:      o.MaxRetries,
		MinRetryBackoff: o.MinRetryBackoff,
		MaxRetryBackoff: o.MaxRetryBackoff,

		DialTimeout:        o.DialTimeout,
		ReadTimeout:        o.ReadTimeout,
		WriteTimeout:       o.WriteTimeout,
		PoolSize:           o.PoolSize,
		MinIdleConns:       o.MinIdleConns,
		MaxConnAge:         o.MaxConnAge,
		PoolTimeout:        o.PoolTimeout,
		IdleTimeout:        o.IdleTimeout,
		IdleCheckFrequency: o.IdleCheckFrequency,

		TLSConfig: o.TLSConfig,
	}
}

func (o *RedisOpts) simple() *redis.Options {
	addr := "192.168.10.14:6379"
	if len(o.Addrs) > 0 {
		addr = o.Addrs[0]
	}
	return &redis.Options{
		Addr:      addr,
		OnConnect: o.OnConnect,

		DB:       o.DB,
		Password: o.Password,

		MaxRetries:      o.MaxRetries,
		MinRetryBackoff: o.MinRetryBackoff,
		MaxRetryBackoff: o.MaxRetryBackoff,

		DialTimeout:  o.DialTimeout,
		ReadTimeout:  o.ReadTimeout,
		WriteTimeout: o.WriteTimeout,

		PoolSize:           o.PoolSize,
		MinIdleConns:       o.MinIdleConns,
		MaxConnAge:         o.MaxConnAge,
		PoolTimeout:        o.PoolTimeout,
		IdleTimeout:        o.IdleTimeout,
		IdleCheckFrequency: o.IdleCheckFrequency,

		TLSConfig: o.TLSConfig,
	}
}

func (o *RedisOpts) failover() *redis.FailoverOptions {
	if len(o.Addrs) == 0 {
		o.Addrs = []string{"192.168.10.14:6379",
			"192.168.10.14:6380",
			"192.168.10.14:6381"}
	}
	return &redis.FailoverOptions{
		SentinelAddrs: o.Addrs,
		MasterName:    o.MasterName,
		OnConnect:     o.OnConnect,

		DB:       o.DB,
		Password: o.Password,

		MaxRetries:      o.MaxRetries,
		MinRetryBackoff: o.MinRetryBackoff,
		MaxRetryBackoff: o.MaxRetryBackoff,

		DialTimeout:  o.DialTimeout,
		ReadTimeout:  o.ReadTimeout,
		WriteTimeout: o.WriteTimeout,

		PoolSize:           o.PoolSize,
		MinIdleConns:       o.MinIdleConns,
		MaxConnAge:         o.MaxConnAge,
		PoolTimeout:        o.PoolTimeout,
		IdleTimeout:        o.IdleTimeout,
		IdleCheckFrequency: o.IdleCheckFrequency,

		TLSConfig: o.TLSConfig,
	}
}

// Connect will establish a connection (always true for dynamic redis)
func (r *RedisCluster) Connect() bool {
	return true
}

func (r *RedisCluster) singleton() redis.UniversalClient {
	return singleton(r.IsCache)
}

// GetClient returns the redis client instance
func (r *RedisCluster) GetClient() redis.UniversalClient {
	client := r.singleton()
	if client == nil {
		log.Warnf("RedisCluster.GetClient() 获取客户端为空，IsCache=%v", r.IsCache)
		return nil
	}
	//log.Debugf("RedisCluster.GetClient() 获取客户端成功，类型=%T，IsCache=%v", client, r.IsCache)
	return client
}

func (r *RedisCluster) hashKey(in string) string {
	if !r.HashKeys {
		return in
	}
	return HashStr(in)
}

func (r *RedisCluster) fixKey(keyName string) string {
	return r.KeyPrefix + r.hashKey(keyName)
}

func (r *RedisCluster) cleanKey(keyName string) string {
	return strings.Replace(keyName, r.KeyPrefix, "", 1)
}

func (r *RedisCluster) Up() error {
	if !Connected() {
		return ErrRedisIsDown
	}
	return nil
}

// GetKey retrieves a key from the database
func (r *RedisCluster) GetKey(ctx context.Context, keyName string) (string, error) {
	if err := r.Up(); err != nil {
		return "", err
	}
	cluster := r.singleton()
	fixedKey := r.fixKey(keyName)

	value, err := cluster.Get(ctx, fixedKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", redis.Nil // ✅ 明确返回redis.Nil表示key不存在
		}
		return "", err
	}

	return value, nil

}

// GetMultiKey gets multiple keys from the database
func (r *RedisCluster) GetMultiKey(ctx context.Context, keys []string) ([]string, error) {
	if err := r.Up(); err != nil {
		return nil, err
	}
	cluster := r.singleton()
	keyNames := make([]string, len(keys))
	copy(keyNames, keys)
	for index, val := range keyNames {
		keyNames[index] = r.fixKey(val)
	}

	result := make([]string, 0)

	switch v := cluster.(type) {
	case *redis.ClusterClient:
		{
			getCmds := make([]*redis.StringCmd, 0)
			pipe := v.Pipeline()
			for _, key := range keyNames {
				getCmds = append(getCmds, pipe.Get(ctx, key))
			}
			_, err := pipe.Exec(ctx)
			if err != nil && !errors.Is(err, redis.Nil) {
				log.Debugf("Error trying to get value: %s", err.Error())
				return nil, ErrKeyNotFound
			}
			for _, cmd := range getCmds {
				result = append(result, cmd.Val())
			}
		}
	case *redis.Client:
		{
			values, err := cluster.MGet(ctx, keyNames...).Result()
			if err != nil {
				log.Debugf("Error trying to get value: %s", err.Error())
				return nil, ErrKeyNotFound
			}
			for _, val := range values {
				strVal := fmt.Sprint(val)
				if strVal == "<nil>" {
					strVal = ""
				}
				result = append(result, strVal)
			}
		}
	}

	for _, val := range result {
		if val != "" {
			return result, nil
		}
	}

	return nil, ErrKeyNotFound
}

// GetKeyTTL returns TTL of the given key
func (r *RedisCluster) GetKeyTTL(ctx context.Context, keyName string) (ttl int64, err error) {
	if err = r.Up(); err != nil {
		return 0, err
	}
	duration, err := r.singleton().TTL(ctx, r.fixKey(keyName)).Result()
	return int64(duration.Seconds()), err
}

// GetRawKey returns the value of the given key (without prefix)
func (r *RedisCluster) GetRawKey(ctx context.Context, keyName string) (string, error) {
	if err := r.Up(); err != nil {
		return "", err
	}
	value, err := r.singleton().Get(ctx, keyName).Result()
	if err != nil {
		log.Debugf("Error trying to get value: %s", err.Error())
		return "", ErrKeyNotFound
	}
	return value, nil
}

// GetExp returns the expiry of the given key
func (r *RedisCluster) GetExp(ctx context.Context, keyName string) (int64, error) {
	log.Debugf("Getting exp for key: %s", r.fixKey(keyName))
	if err := r.Up(); err != nil {
		return 0, err
	}

	value, err := r.singleton().TTL(ctx, r.fixKey(keyName)).Result()
	if err != nil {
		log.Errorf("Error trying to get TTL: ", err.Error())
		return 0, ErrKeyNotFound
	}
	return int64(value.Seconds()), nil
}

// SetExp sets expiry of the given key
func (r *RedisCluster) SetExp(ctx context.Context, keyName string, timeout time.Duration) error {
	if err := r.Up(); err != nil {
		return err
	}
	err := r.singleton().Expire(ctx, r.fixKey(keyName), timeout).Err()
	if err != nil {
		log.Errorf("Could not EXPIRE key: %s", err.Error())
	}
	return err
}

// SetKey creates or updates a key-value pair
func (r *RedisCluster) SetKey(ctx context.Context, keyName, session string, timeout time.Duration) error {
	//log.Debugf("[STORE] SET Raw key is: %s", keyName)
	//log.Debugf("[STORE] Setting key: %s", r.fixKey(keyName))

	if err := r.Up(); err != nil {
		return err
	}
	err := r.singleton().Set(ctx, r.fixKey(keyName), session, timeout).Err()
	if err != nil {
		log.Errorf("Error trying to set value: %s", err.Error())
		return err
	}
	//log.Debugf("存储成功:key=%v", r.fixKey(keyName))
	return nil
}

// SetRawKey sets the value of the given key (without prefix)
func (r *RedisCluster) SetRawKey(ctx context.Context, keyName, session string, timeout time.Duration) error {
	if err := r.Up(); err != nil {
		return err
	}
	err := r.singleton().Set(ctx, keyName, session, timeout).Err()
	if err != nil {
		log.Errorf("Error trying to set value: %s", err.Error())
		return err
	}
	return nil
}

// Decrement decrements a key in redis
func (r *RedisCluster) Decrement(ctx context.Context, keyName string) {
	keyName = r.fixKey(keyName)
	log.Debugf("Decrementing key: %s", keyName)
	if err := r.Up(); err != nil {
		return
	}
	err := r.singleton().Decr(ctx, keyName).Err()
	if err != nil {
		log.Errorf("Error trying to decrement value: %s", err.Error())
	}
}

// IncrememntWithExpire increments a key with expiration
func (r *RedisCluster) IncrememntWithExpire(ctx context.Context, keyName string, expire int64) int64 {
	log.Debugf("Incrementing raw key: %s", keyName)
	if err := r.Up(); err != nil {
		return 0
	}
	fixedKey := keyName
	val, err := r.singleton().Incr(ctx, fixedKey).Result()

	if err != nil {
		log.Errorf("Error trying to increment value: %s", err.Error())
	} else {
		log.Debugf("Incremented key: %s, val is: %d", fixedKey, val)
	}

	if val == 1 && expire > 0 {
		log.Debug("--> Setting Expire")
		r.singleton().Expire(ctx, fixedKey, time.Duration(expire)*time.Second)
	}

	return val
}

// GetKeys returns all keys matching the filter
func (r *RedisCluster) GetKeys(ctx context.Context, filter string) []string {
	if err := r.Up(); err != nil {
		return nil
	}
	client := r.singleton()
	if client == nil {
		log.Warn("Redis client is nil in GetKeys")
		return nil
	}

	// 构建搜索模式
	filterHash := ""
	if filter != "" {
		filterHash = r.hashKey(filter)
	}
	searchStr := r.KeyPrefix + filterHash + "*"
	//	log.Debugf("[STORE] Search pattern: %s", searchStr)

	// 内部函数：扫描单个节点的键
	fnFetchKeys := func(nodeClient *redis.Client, nodeCtx context.Context) ([]string, error) {
		var keys []string
		iter := nodeClient.Scan(nodeCtx, 0, searchStr, 0).Iterator()
		for iter.Next(nodeCtx) {
			keys = append(keys, iter.Val())
		}
		return keys, iter.Err()
	}

	sessions := make([]string, 0)

	// 类型断言：区分集群和单机模式
	switch clusterCli := client.(type) {
	case *redis.ClusterClient:
		// 集群模式：遍历所有主节点
		resultCh := make(chan []string, 5) // 缓冲避免阻塞
		errCh := make(chan error, 1)

		go func() {
			defer close(resultCh)
			defer close(errCh)

			// 回调函数符合v8要求：(context.Context, *redis.Client) error
			foreachErr := clusterCli.ForEachMaster(ctx, func(nodeCtx context.Context, nodeCli *redis.Client) error {
				if nodeCtx.Err() != nil {
					return nodeCtx.Err()
				}

				nodeKeys, err := fnFetchKeys(nodeCli, nodeCtx)
				if err != nil {
					log.Warnf("Node %s scan failed: %v", nodeCli.Options().Addr, err)
					return nil
				}

				select {
				case resultCh <- nodeKeys:
					return nil
				case <-nodeCtx.Done():
					return nodeCtx.Err()
				}
			})

			if foreachErr != nil && !errors.Is(foreachErr, context.Canceled) {
				errCh <- fmt.Errorf("ForEachMaster failed: %w", foreachErr)
			}
		}()

		// 收集结果
		for {
			select {
			case keys := <-resultCh:
				sessions = append(sessions, keys...)
			case err, ok := <-errCh:
				if ok && err != nil {
					log.Errorf("Cluster scan failed: %v", err)
				}
				return nil
			case <-ctx.Done():
				log.Errorf("GetKeys canceled: %v", ctx.Err())
				return nil
			}
		}

	case *redis.Client:
		// 单机模式：直接扫描
		nodeKeys, err := fnFetchKeys(clusterCli, ctx)
		if err != nil {
			log.Errorf("Single node scan failed: %v", err)
			return nil
		}
		sessions = nodeKeys

	default:
		log.Errorf("Unsupported client type: %T", client)
		return nil
	}

	// 清理键前缀
	for i, key := range sessions {
		sessions[i] = r.cleanKey(key)
	}

	return sessions
}

// GetKeysAndValuesWithFilter returns all keys and values matching the filter
func (r *RedisCluster) GetKeysAndValuesWithFilter(ctx context.Context, filter string) map[string]string {
	if err := r.Up(); err != nil {
		return nil
	}
	keys := r.GetKeys(ctx, filter)
	if keys == nil {
		log.Error("Error trying to get filtered client keys")
		return nil
	}

	if len(keys) == 0 {
		return nil
	}

	for i, v := range keys {
		keys[i] = r.KeyPrefix + v
	}

	client := r.singleton()
	values := make([]string, 0)

	switch v := client.(type) {
	case *redis.ClusterClient:
		{
			getCmds := make([]*redis.StringCmd, 0)
			pipe := v.Pipeline()
			for _, key := range keys {
				getCmds = append(getCmds, pipe.Get(ctx, key))
			}
			_, err := pipe.Exec(ctx)
			if err != nil && !errors.Is(err, redis.Nil) {
				log.Errorf("Error trying to get client keys: %s", err.Error())
				return nil
			}

			for _, cmd := range getCmds {
				values = append(values, cmd.Val())
			}
		}
	case *redis.Client:
		{
			result, err := v.MGet(ctx, keys...).Result()
			if err != nil {
				log.Errorf("Error trying to get client keys: %s", err.Error())
				return nil
			}

			for _, val := range result {
				strVal := fmt.Sprint(val)
				if strVal == "<nil>" {
					strVal = ""
				}
				values = append(values, strVal)
			}
		}
	}

	m := make(map[string]string)
	for i, v := range keys {
		m[r.cleanKey(v)] = values[i]
	}

	return m
}

// GetKeysAndValues returns all keys and their values
func (r *RedisCluster) GetKeysAndValues(ctx context.Context) map[string]string {
	return r.GetKeysAndValuesWithFilter(ctx, "")
}

// DeleteKey removes a key from the database
func (r *RedisCluster) DeleteKey(ctx context.Context, keyName string) (bool, error) {
	if err := r.Up(); err != nil {
		return false, err
	}
	//log.Debugf("DEL Key was: %s", keyName)
	//log.Debugf("DEL Key became: %s", r.fixKey(keyName))
	n, err := r.singleton().Del(ctx, r.fixKey(keyName)).Result()
	if err != nil {
		// 根据错误类型提供更具体的错误信息
		if errors.Is(err, context.DeadlineExceeded) {
			return false, fmt.Errorf("delete operation timeout for key %s: %w", keyName, err)
		}
		if errors.Is(err, context.Canceled) {
			return false, fmt.Errorf("delete operation canceled for key %s: %w", keyName, err)
		}

		log.Errorf("Error trying to delete key %s: %s", keyName, err.Error())
		return false, fmt.Errorf("redis delete failed for key %s: %w", keyName, err)
	}

	return n > 0, nil
}

// DeleteAllKeys removes all keys from the database
func (r *RedisCluster) DeleteAllKeys(ctx context.Context) bool {
	if err := r.Up(); err != nil {
		return false
	}
	n, err := r.singleton().FlushAll(ctx).Result()
	if err != nil {
		log.Errorf("Error trying to delete keys: %s", err.Error())
	}
	return n == "OK"
}

// DeleteRawKey removes a key without prefix
func (r *RedisCluster) DeleteRawKey(ctx context.Context, keyName string) (bool, error) {
	if err := r.Up(); err != nil {
		return false, err
	}
	n, err := r.singleton().Del(ctx, keyName).Result()
	if err != nil {
		log.Errorf("Error trying to delete key: %s", err.Error())
		return false, err
	}
	return n > 0, nil
}

// DeleteScanMatch removes keys matching pattern in bulk
func (r *RedisCluster) DeleteScanMatch(ctx context.Context, pattern string) bool {
	if err := r.Up(); err != nil {
		return false
	}
	client := r.singleton()
	log.Debugf("Deleting: %s", pattern)

	// 修正：fnScan增加context参数，适配v8的Scan命令
	fnScan := func(client *redis.Client, nodeCtx context.Context) ([]string, error) {
		values := make([]string, 0)
		iter := client.Scan(nodeCtx, 0, pattern, 0).Iterator()
		for iter.Next(nodeCtx) { // 迭代器需传入context
			values = append(values, iter.Val())
		}

		if err := iter.Err(); err != nil {
			return nil, err
		}
		return values, nil
	}

	var err error
	var keys []string
	var values []string

	switch v := client.(type) {
	case *redis.ClusterClient:
		ch := make(chan []string)
		go func() {
			// 核心修复：回调函数添加context.Context参数，符合v8签名要求
			err = v.ForEachMaster(ctx, func(nodeCtx context.Context, client *redis.Client) error {
				values, err = fnScan(client, nodeCtx) // 传入nodeCtx
				if err != nil {
					return err
				}
				ch <- values
				return nil
			})
			close(ch)
		}()

		for vals := range ch {
			keys = append(keys, vals...)
		}
	case *redis.Client:
		keys, err = fnScan(v, ctx) // 传入ctx
	}

	if err != nil {
		log.Errorf("SCAN command failed with err: %s", err.Error())
		return false
	}

	if len(keys) > 0 {
		for _, name := range keys {
			log.Debugf("Deleting: %s", name)
			err := client.Del(ctx, name).Err()
			if err != nil {
				log.Errorf("Error trying to delete key: %s - %s", name, err.Error())
			}
		}
		log.Debugf("Deleted: %d records", len(keys))
	} else {
		log.Debug("RedisCluster called DEL - Nothing to delete")
	}

	return true
}

// DeleteKeys removes a group of keys in bulk
func (r *RedisCluster) DeleteKeys(ctx context.Context, keys []string) bool {
	if err := r.Up(); err != nil {
		return false
	}
	if len(keys) > 0 {
		for i, v := range keys {
			keys[i] = r.fixKey(v)
		}

		log.Debugf("Deleting: %v", keys)
		client := r.singleton()
		switch v := client.(type) {
		case *redis.ClusterClient:
			{
				pipe := v.Pipeline()
				for _, k := range keys {
					pipe.Del(ctx, k)
				}

				if _, err := pipe.Exec(ctx); err != nil {
					log.Errorf("Error trying to delete keys: %s", err.Error())
				}
			}
		case *redis.Client:
			{
				_, err := v.Del(ctx, keys...).Result()
				if err != nil {
					log.Errorf("Error trying to delete keys: %s", err.Error())
				}
			}
		}
	} else {
		log.Debug("RedisCluster called DEL - Nothing to delete")
	}

	return true
}

// StartPubSubHandler listens for pubsub messages
func (r *RedisCluster) StartPubSubHandler(ctx context.Context, channel string, callback func(interface{})) error {
	if err := r.Up(); err != nil {
		return err
	}
	client := r.singleton()
	if client == nil {
		return errors.New("redis connection failed")
	}

	pubsub := client.Subscribe(ctx, channel)
	defer pubsub.Close()

	if _, err := pubsub.Receive(ctx); err != nil {
		log.Errorf("Error while receiving pubsub message: %s", err.Error())
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-pubsub.Channel():
			callback(msg)
		}
	}
}

// Publish sends a message to a channel
func (r *RedisCluster) Publish(ctx context.Context, channel, message string) error {
	if err := r.Up(); err != nil {
		return err
	}
	err := r.singleton().Publish(ctx, channel, message).Err()
	if err != nil {
		log.Errorf("Error trying to publish message: %s", err.Error())
		return err
	}
	return nil
}

// GetAndDeleteSet gets and deletes a set
func (r *RedisCluster) GetAndDeleteSet(ctx context.Context, keyName string) []interface{} {
	log.Debugf("Getting raw key set: %s", keyName)
	if err := r.Up(); err != nil {
		return nil
	}
	log.Debugf("keyName is: %s", keyName)
	fixedKey := r.fixKey(keyName)
	log.Debugf("Fixed keyname is: %s", fixedKey)

	client := r.singleton()

	var lrange *redis.StringSliceCmd
	_, err := client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		lrange = pipe.LRange(ctx, fixedKey, 0, -1)
		pipe.Del(ctx, fixedKey)
		return nil
	})
	if err != nil {
		log.Errorf("Multi command failed: %s", err.Error())
		return nil
	}

	vals := lrange.Val()
	log.Debugf("Analytics returned: %d", len(vals))
	if len(vals) == 0 {
		return nil
	}

	log.Debugf("Unpacked vals: %d", len(vals))
	result := make([]interface{}, len(vals))
	for i, v := range vals {
		result[i] = v
	}

	return result
}

// AppendToSet appends a value to a set
func (r *RedisCluster) AppendToSet(ctx context.Context, keyName, value string) {
	fixedKey := r.fixKey(keyName)
	log.Debug("Pushing to raw key list", log.String("keyName", keyName))
	log.Debug("Appending to fixed key list", log.String("fixedKey", fixedKey))
	if err := r.Up(); err != nil {
		return
	}
	if err := r.singleton().RPush(ctx, fixedKey, value).Err(); err != nil {
		log.Errorf("Error trying to append to set keys: %s", err.Error())
	}
}

// Exists checks if a key exists
func (r *RedisCluster) Exists(ctx context.Context, keyName string) (bool, error) {
	fixedKey := r.fixKey(keyName)
	//log.Debug("Checking if exists", log.String("keyName", fixedKey))

	exists, err := r.singleton().Exists(ctx, fixedKey).Result()
	if err != nil {
		log.Errorf("Error trying to check if key exists: %s", err.Error())
		return false, err
	}
	return exists == 1, nil
}

// RemoveFromList deletes a value from a list
func (r *RedisCluster) RemoveFromList(ctx context.Context, keyName, value string) error {
	fixedKey := r.fixKey(keyName)

	log.Debug(
		"Removing value from list",
		log.String("keyName", keyName),
		log.String("fixedKey", fixedKey),
		log.String("value", value),
	)

	if err := r.singleton().LRem(ctx, fixedKey, 0, value).Err(); err != nil {
		log.Error(
			"LREM command failed",
			log.String("keyName", keyName),
			log.String("fixedKey", fixedKey),
			log.String("value", value),
			log.String("error", err.Error()),
		)
		return err
	}
	return nil
}

// GetListRange gets range of elements from a list
func (r *RedisCluster) GetListRange(ctx context.Context, keyName string, from, to int64) ([]string, error) {
	fixedKey := r.fixKey(keyName)

	elements, err := r.singleton().LRange(ctx, fixedKey, from, to).Result()
	if err != nil {
		log.Error(
			"LRANGE command failed",
			log.String("keyName", keyName),
			log.String("fixedKey", fixedKey),
			log.Int64("from", from),
			log.Int64("to", to),
			log.String("error", err.Error()),
		)
		return nil, err
	}
	return elements, nil
}

// AppendToSetPipelined appends values using pipeline
func (r *RedisCluster) AppendToSetPipelined(ctx context.Context, key string, values [][]byte) {
	if len(values) == 0 {
		return
	}

	fixedKey := r.fixKey(key)
	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return
	}
	client := r.singleton()

	pipe := client.Pipeline()
	for _, val := range values {
		pipe.RPush(ctx, fixedKey, val)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		log.Errorf("Error trying to append to set keys: %s", err.Error())
	}

	if storageExpTime := int64(viper.GetDuration("analytics.storage-expiration-time")); storageExpTime != int64(-1) {
		exp, _ := r.GetExp(ctx, key)
		if exp == -1 {
			_ = r.SetExp(ctx, key, time.Duration(storageExpTime)*time.Second)
		}
	}
}

// GetSet returns set values
func (r *RedisCluster) GetSet(ctx context.Context, keyName string) (map[string]string, error) {
	log.Debugf("Getting from key set: %s", keyName)
	log.Debugf("Getting from fixed key set: %s", r.fixKey(keyName))
	if err := r.Up(); err != nil {
		return nil, err
	}
	val, err := r.singleton().SMembers(ctx, r.fixKey(keyName)).Result()
	if err != nil {
		log.Errorf("Error trying to get key set: %s", err.Error())
		return nil, err
	}

	result := make(map[string]string)
	for i, value := range val {
		result[strconv.Itoa(i)] = value
	}
	return result, nil
}

// AddToSet adds a value to a set
func (r *RedisCluster) AddToSet(ctx context.Context, keyName, value string) error {
	log.Debugf("Pushing to raw key set: %s", keyName)
	log.Debugf("Pushing to fixed key set: %s", r.fixKey(keyName))
	if err := r.Up(); err != nil {
		return err
	}
	err := r.singleton().SAdd(ctx, r.fixKey(keyName), value).Err()
	if err != nil {
		log.Errorf("Error trying to append keys: %s", err.Error())
		return err
	}
	return nil
}

// RemoveFromSet removes a value from a set
func (r *RedisCluster) RemoveFromSet(ctx context.Context, keyName, value string) error {
	log.Debugf("Removing from raw key set: %s", keyName)
	log.Debugf("Removing from fixed key set: %s", r.fixKey(keyName))
	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return err
	}
	err := r.singleton().SRem(ctx, r.fixKey(keyName), value).Err()
	if err != nil {
		log.Errorf("Error trying to remove keys: %s", err.Error())
		return err
	}
	return nil
}

// IsMemberOfSet checks if a value is in a set
func (r *RedisCluster) IsMemberOfSet(ctx context.Context, keyName, value string) (bool, error) {
	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return false, err
	}
	val, err := r.singleton().SIsMember(ctx, r.fixKey(keyName), value).Result()
	if err != nil {
		log.Errorf("Error trying to check set member: %s", err.Error())
		return false, err
	}
	log.Debugf("SISMEMBER %s %s %v %v", keyName, value, val, err)
	return val, nil
}

// SetRollingWindow manages a time-based rolling window
func (r *RedisCluster) SetRollingWindow(
	ctx context.Context,
	keyName string,
	per int64,
	valueOverride string,
	pipeline bool,
) (int, []interface{}) {
	log.Debugf("Incrementing raw key: %s", keyName)
	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return 0, nil
	}
	log.Debugf("keyName is: %s", keyName)
	now := time.Now()
	log.Debugf("Now is: %v", now)
	onePeriodAgo := now.Add(time.Duration(-1*per) * time.Second)
	log.Debugf("Then is: %v", onePeriodAgo)

	client := r.singleton()
	var zrange *redis.StringSliceCmd

	pipeFn := func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, keyName, "-inf", strconv.Itoa(int(onePeriodAgo.UnixNano())))
		zrange = pipe.ZRange(ctx, keyName, 0, -1)

		element := redis.Z{
			Score: float64(now.UnixNano()),
		}

		if valueOverride != "-1" {
			element.Member = valueOverride
		} else {
			element.Member = strconv.Itoa(int(now.UnixNano()))
		}

		pipe.ZAdd(ctx, keyName, &element)
		pipe.Expire(ctx, keyName, time.Duration(per)*time.Second)
		return nil
	}

	var err error
	if pipeline {
		_, err = client.Pipelined(ctx, pipeFn)
	} else {
		_, err = client.TxPipelined(ctx, pipeFn)
	}

	if err != nil {
		log.Errorf("Multi command failed: %s", err.Error())
		return 0, nil
	}

	values := zrange.Val()
	if values == nil {
		return 0, nil
	}

	intVal := len(values)
	result := make([]interface{}, len(values))
	for i, v := range values {
		result[i] = v
	}

	log.Debugf("Returned: %d", intVal)
	return intVal, result
}

// GetRollingWindow returns rolling window data
func (r RedisCluster) GetRollingWindow(ctx context.Context, keyName string, per int64, pipeline bool) (int, []interface{}) {
	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return 0, nil
	}
	now := time.Now()
	onePeriodAgo := now.Add(time.Duration(-1*per) * time.Second)

	client := r.singleton()
	var zrange *redis.StringSliceCmd

	pipeFn := func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, keyName, "-inf", strconv.Itoa(int(onePeriodAgo.UnixNano())))
		zrange = pipe.ZRange(ctx, keyName, 0, -1)
		return nil
	}

	var err error
	if pipeline {
		_, err = client.Pipelined(ctx, pipeFn)
	} else {
		_, err = client.TxPipelined(ctx, pipeFn)
	}
	if err != nil {
		log.Errorf("Multi command failed: %s", err.Error())
		return 0, nil
	}

	values := zrange.Val()
	if values == nil {
		return 0, nil
	}

	intVal := len(values)
	result := make([]interface{}, intVal)
	for i, v := range values {
		result[i] = v
	}

	log.Debugf("Returned: %d", intVal)
	return intVal, result
}

// GetKeyPrefix returns storage key prefix
func (r *RedisCluster) GetKeyPrefix() string {
	return r.KeyPrefix
}

// AddToSortedSet adds value to sorted set with score
func (r *RedisCluster) AddToSortedSet(ctx context.Context, keyName, value string, score float64) {
	fixedKey := r.fixKey(keyName)

	log.Debug("Pushing raw key to sorted set", log.String("keyName", keyName), log.String("fixedKey", fixedKey))

	if err := r.Up(); err != nil {
		log.Debug(err.Error())
		return
	}
	member := redis.Z{Score: score, Member: value}
	if err := r.singleton().ZAdd(ctx, fixedKey, &member).Err(); err != nil {
		log.Error(
			"ZADD command failed",
			log.String("keyName", keyName),
			log.String("fixedKey", fixedKey),
			log.String("error", err.Error()),
		)
	}
}

// GetSortedSetRange gets range from sorted set
func (r *RedisCluster) GetSortedSetRange(ctx context.Context, keyName, scoreFrom, scoreTo string) ([]string, []float64, error) {
	fixedKey := r.fixKey(keyName)
	log.Debug(
		"Getting sorted set range",
		log.String("keyName", keyName),
		log.String("fixedKey", fixedKey),
		log.String("scoreFrom", scoreFrom),
		log.String("scoreTo", scoreTo),
	)

	args := redis.ZRangeBy{Min: scoreFrom, Max: scoreTo}
	values, err := r.singleton().ZRangeByScoreWithScores(ctx, fixedKey, &args).Result()
	if err != nil {
		log.Error(
			"ZRANGEBYSCORE command failed",
			log.String("keyName", keyName),
			log.String("fixedKey", fixedKey),
			log.String("scoreFrom", scoreFrom),
			log.String("scoreTo", scoreTo),
			log.String("error", err.Error()),
		)
		return nil, nil, err
	}

	if len(values) == 0 {
		return nil, nil, nil
	}

	elements := make([]string, len(values))
	scores := make([]float64, len(values))

	for i, v := range values {
		elements[i] = fmt.Sprint(v.Member)
		scores[i] = v.Score
	}

	return elements, scores, nil
}

// RemoveSortedSetRange removes range from sorted set
func (r *RedisCluster) RemoveSortedSetRange(ctx context.Context, keyName, scoreFrom, scoreTo string) error {
	fixedKey := r.fixKey(keyName)

	log.Debug(
		"Removing sorted set range",
		log.String("keyName", keyName),
		log.String("fixedKey", fixedKey),
		log.String("scoreFrom", scoreFrom),
		log.String("scoreTo", scoreTo),
	)

	if err := r.singleton().ZRemRangeByScore(ctx, fixedKey, scoreFrom, scoreTo).Err(); err != nil {
		log.Debug(
			"ZREMRANGEBYSCORE command failed",
			log.String("keyName", keyName),
			log.String("fixedKey", fixedKey),
			log.String("scoreFrom", scoreFrom),
			log.String("scoreTo", scoreTo),
			log.String("error", err.Error()),
		)
		return err
	}
	return nil
}

// Eval executes a Lua script
func (r *RedisCluster) Eval(ctx context.Context, script string, keys []string, args []interface{}) (interface{}, error) {
	if err := r.Up(); err != nil {
		return nil, err
	}

	var fixedKeys []string
	if len(keys) > 0 {
		fixedKeys = make([]string, len(keys))
		for i, key := range keys {
			fixedKeys[i] = r.fixKey(key)
		}
	}

	client := r.singleton()
	switch v := client.(type) {
	case *redis.ClusterClient:
		return v.Eval(ctx, script, fixedKeys, args).Result()
	case *redis.Client:
		return v.Eval(ctx, script, fixedKeys, args).Result()
	default:
		return nil, errors.New("unsupported redis client type")
	}
}

// EvalSha executes a pre-loaded Lua script by SHA1
func (r *RedisCluster) EvalSha(ctx context.Context, sha1 string, keys []string, args []interface{}) (interface{}, error) {
	if err := r.Up(); err != nil {
		return nil, err
	}

	var fixedKeys []string
	if len(keys) > 0 {
		fixedKeys = make([]string, len(keys))
		for i, key := range keys {
			fixedKeys[i] = r.fixKey(key)
		}
	}

	client := r.singleton()
	switch v := client.(type) {
	case *redis.ClusterClient:
		return v.EvalSha(ctx, sha1, fixedKeys, args).Result()
	case *redis.Client:
		return v.EvalSha(ctx, sha1, fixedKeys, args).Result()
	default:
		return nil, errors.New("unsupported redis client type")
	}
}

// ScriptLoad loads a Lua script to Redis server
func (r *RedisCluster) ScriptLoad(ctx context.Context, script string) (string, error) {
	if err := r.Up(); err != nil {
		return "", err
	}

	client := r.singleton()
	switch v := client.(type) {
	case *redis.ClusterClient:
		return v.ScriptLoad(ctx, script).Result()
	case *redis.Client:
		return v.ScriptLoad(ctx, script).Result()
	default:
		return "", errors.New("unsupported redis client type")
	}
}

// SetNX sets a key only if it doesn't exist (atomic operation)
func (r *RedisCluster) SetNX(ctx context.Context, keyName string, value interface{}, expiration time.Duration) (bool, error) {
	if err := r.Up(); err != nil {
		return false, err
	}

	fixedKey := r.fixKey(keyName)
	result, err := r.singleton().SetNX(ctx, fixedKey, value, expiration).Result()
	if err != nil {
		log.Errorf("redis服务出现问题,请马上修改: %s", err.Error())
		return false, err
	}
	return result, nil
}

// HashStr 哈希字符串实现
func HashStr(s string) string {
	h := 0
	for _, c := range s {
		h = (h << 5) - h + int(c)
	}
	if h < 0 {
		h = -h
	}
	return strconv.Itoa(h)
}
