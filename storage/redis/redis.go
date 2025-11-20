package redis

import (
	"context"
	"fmt"
	"time"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/storage"

	"github.com/go-redis/redis/v8"
)

type Storage struct {
	pool      map[string]*redis.Client
	rpsLimits map[string]int32
}

func NewRedis(cfg map[string]config.Config, projects map[string]int32) storage.RedisStorageI {
	redisPool := make(map[string]*redis.Client)

	for k, v := range cfg {
		redisPool[k] = redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%s", v.GetRequestRedisHost, v.GetRequestRedisPort),
			DB:   v.GetRequestRedisDatabase,
		})
	}

	strg := Storage{
		pool:      redisPool,
		rpsLimits: projects,
	}

	for projectId, limit := range projects {
		strg.Set(context.Background(), fmt.Sprintf("rate_limit:%s:limit", projectId), limit, 0, projectId, "")
	}

	return strg
}

func (s Storage) SetX(ctx context.Context, key string, value string, duration time.Duration, projectId string, nodeType string) error {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}

	return s.pool[projectId].SetEX(ctx, key, value, duration).Err()
}

func (s Storage) Get(ctx context.Context, key string, projectId string, nodeType string) (string, error) {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Get(ctx, key).Result()
}

func (s Storage) Del(ctx context.Context, keys string, projectId string, nodeType string) error {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Del(ctx, keys).Err()
}

func (s Storage) Set(ctx context.Context, key string, value interface{}, duration time.Duration, projectId string, nodeType string) error {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Set(ctx, key, value, duration).Err()
}

func (s Storage) DelMany(ctx context.Context, keys []string, projectId string, nodeType string) error {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}

	return s.pool[projectId].Del(ctx, keys...).Err()
}

func (s Storage) GetResult(ctx context.Context, key string, projectId string, nodeType string) *redis.StringCmd {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Get(ctx, key)
}

func (s Storage) Incr(ctx context.Context, key string, projectId string, nodeType string) *redis.IntCmd {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Incr(ctx, key)
}

func (s Storage) Expire(ctx context.Context, key string, expiration time.Duration, projectId string, nodeType string) *redis.BoolCmd {
	if nodeType != config.ENTER_PRICE_TYPE {
		projectId = config.UCODE_NAMESPACE
	}
	return s.pool[projectId].Expire(ctx, key, expiration)
}
