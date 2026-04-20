package storage

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisOptions = redis.Options

type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Del(ctx context.Context, keys ...string) (int64, error)
	Incr(ctx context.Context, key string) (int64, error)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}

type RedisInstance struct {
	client *redis.Client
}

func NewRedisInstance(addr string) *RedisInstance {
	return &RedisInstance{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func NewRedisInstanceWithOptions(options *RedisOptions) *RedisInstance {
	return &RedisInstance{
		client: redis.NewClient(options),
	}
}

func (r *RedisInstance) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisInstance) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisInstance) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *RedisInstance) Del(ctx context.Context, keys ...string) (int64, error) {
	return r.client.Del(ctx, keys...).Result()
}

func (r *RedisInstance) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

func (r *RedisInstance) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}

func (r *RedisInstance) Close() error {
	return r.client.Close()
}

func (r *RedisInstance) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
