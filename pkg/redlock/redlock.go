package redlock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/example/redlock-fencing-demo/pkg/fencing"
	"github.com/example/redlock-fencing-demo/pkg/storage"
)

type Instance interface {
	Get(ctx context.Context, key string) (string, error)
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	Del(ctx context.Context, keys ...string) (int64, error)
	Incr(ctx context.Context, key string) (int64, error)
}

type Redlock struct {
	instances []Instance
	quorum    int
	ttl       time.Duration
	keyPrefix string
	mu        sync.Mutex
}

func NewRedlock(instances []Instance, ttl time.Duration) *Redlock {
	return &Redlock{
		instances: instances,
		quorum:    len(instances)/2 + 1,
		ttl:       ttl,
		keyPrefix: "redlock:",
	}
}

func (r *Redlock) Lock(ctx context.Context, lockName string) (fencing.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokenKey := r.keyPrefix + lockName + ":token"
	lockKey := r.keyPrefix + lockName

	newToken, err := r.instances[0].Incr(ctx, tokenKey)
	if err != nil {
		return 0, err
	}

	for i := 1; i < len(r.instances); i++ {
		r.instances[i].Incr(ctx, tokenKey)
	}

	acquired := 0

	for _, inst := range r.instances {
		success, err := r.tryAcquire(ctx, inst, lockKey, fencing.Token(newToken))
		if err != nil {
			continue
		}
		if success {
			acquired++
		}
	}

	if acquired >= r.quorum {
		return fencing.Token(newToken), nil
	}

	r.forceRelease(ctx, lockName)
	return 0, errors.New("failed to acquire lock: insufficient instances responded")
}

func (r *Redlock) tryAcquire(ctx context.Context, inst Instance, lockKey string, token fencing.Token) (bool, error) {
	script := `
		local key = KEYS[1]
		local current = redis.call('get', key)
		if not current then
			redis.call('set', key, ARGV[1], 'PX', ARGV[2])
			return 1
		end
		return 0
	`

	result, err := inst.Eval(ctx, script, []string{lockKey}, token, r.ttl.Milliseconds())
	if err != nil {
		return false, err
	}

	return result == int64(1), nil
}

func (r *Redlock) Unlock(ctx context.Context, lockName string, token fencing.Token) error {
	lockKey := r.keyPrefix + lockName

	for _, inst := range r.instances {
		script := `
			local key = KEYS[1]
			local current = redis.call('get', key)
			if current == ARGV[1] then
				redis.call('del', key)
				return 1
			end
			return 0
		`
		inst.Eval(ctx, script, []string{lockKey}, token)
	}
	return nil
}

func (r *Redlock) forceRelease(ctx context.Context, lockName string) {
	lockKey := r.keyPrefix + lockName
	tokenKey := r.keyPrefix + lockName + ":token"

	for _, inst := range r.instances {
		inst.Del(ctx, lockKey, tokenKey)
	}
}

func (r *Redlock) WithFencingWriter(inst Instance) *fencing.Writer {
	return fencing.NewWriter(fencingStore{inst: inst})
}

type fencingStore struct {
	inst Instance
}

func (s fencingStore) Get(ctx context.Context, key string) (string, error) {
	return s.inst.Get(ctx, key)
}

func (s fencingStore) Set(ctx context.Context, key string, value string) error {
	script := "redis.call('set', KEYS[1], ARGV[1]) return 1"
	_, err := s.inst.Eval(ctx, script, []string{key}, value)
	return err
}

func (s fencingStore) Incr(ctx context.Context, key string) (int64, error) {
	return s.inst.Incr(ctx, key)
}

func NewRedisInstance(addr string) Instance {
	return storage.NewRedisInstance(addr)
}

func NewRedisInstanceWithOptions(options *storage.RedisOptions) Instance {
	return storage.NewRedisInstanceWithOptions(options)
}
