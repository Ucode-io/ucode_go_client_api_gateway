package config

import (
	"time"

	"github.com/golanguzb70/ratelimiter"
)

const (
	COMMIT_TYPE_APP                      = "APP"
	COMMIT_TYPE_TABLE                    = "TABLE"
	COMMIT_TYPE_FIELD                    = "FIELD"
	COMMIT_TYPE_RELATION                 = "RELATION"
	COMMIT_TYPE_SECTION                  = "SECTION"
	COMMIT_TYPE_VIEW                     = "VIEW"
	COMMIT_TYPE_VIEW_RELATION            = "VIEW_RELATION"
	COMMIT_TYPE_CLIENT_PLATFORM          = "CLIENT_PLATFORM"
	COMMIT_TYPE_CLIENT_TYPE              = "CLIENT_TYPE"
	COMMIT_TYPE_ROLE                     = "ROLE"
	COMMIT_TYPE_TEST_LOGIN               = "TEST_LOGIN"
	COMMIT_TYPE_CONNECTION               = "CONNECTION"
	COMMIT_TYPE_AUTOMATIC_FILTER         = "AUTOMATIC_FILTER"
	COMMIT_TYPE_CUSTOM_EVENT             = "CUSTOM_EVENT"
	COMMIT_TYPE_RECORD_PERMISSION        = "RECORD_PERMISSION"
	COMMIT_TYPE_ACTION_PERMISSION        = "ACTION_PERMISSION"
	COMMIT_TYPE_FIELD_PERMISSION         = "FIELD_PERMISSION"
	COMMIT_TYPE_VIEW_PERMISSION          = "VIEW_PERMISSION"
	COMMIT_TYPE_VIEW_RELATION_PERMISSION = "VIEW_RELATION_PERMISSION"
	COMMIT_TYPE_DASHBOARD                = "DASHBOARD"
	COMMIT_TYPE_VARIABLE                 = "VARIABLE"
	COMMIT_TYPE_PANEL                    = "PANEL"
	COMMIT_TYPE_FUNCTION                 = "FUNCTION"
	COMMIT_TYPE_SCENARIO                 = "SCENARIO"
	LOW_NODE_TYPE                        = "LOW"
	HIGH_NODE_TYPE                       = "HIGH"
	ENTER_PRICE_TYPE                     = "ENTER_PRICE"
	UCODE_NAMESPACE                      = "u-code"
	CACHE_WAIT                           = "WAIT"
	LIMITER_RANGE                        = 100
)

const (
	LRU_CACHE_SIZE     = 10000
	REDIS_TIMEOUT      = 5 * time.Minute
	REDIS_KEY_TIMEOUT  = 280 * time.Second
	REDIS_WAIT_TIMEOUT = 1 * time.Second
	REDIS_SLEEP        = 100 * time.Millisecond
	TIME_LAYOUT        = "15:04"
	REDIS_EXPIRATION   = time.Second
)

var (
	WithGoogle = "google"
	Default    = "default"
	WithPhone  = "phone"
	WithApple  = "apple"
	WithEmail  = "email"

	OpenFaaSBaseUrl string = "https://ofs.u-code.io/function/"
	KnativeBaseUrl  string = "knative-fn.u-code.io"

	ValidRecipients = map[string]bool{
		"EMAIL": true,
		"PHONE": true,
	}

	RateLimitCfg = []*ratelimiter.LeakyBucket{
		{
			Method:         "POST",
			Path:           "/v2/send-code",
			RequestLimit:   5,
			Interval:       "minute",
			Type:           "body",
			KeyField:       "recipient",
			AllowOnFailure: true,
			NotAllowMsg:    "send-code request limit exceeded",
			NotAllowCode:   "TOO_MANY_REQUESTS",
		},
	}
	PublicStatus = "unapproved"
)
