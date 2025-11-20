package models

type MetadataResponse struct {
	Metadata struct {
		Sources []struct {
			Tables []struct {
				Table struct {
					Name string `json:"name"`
				} `json:"table"`
			} `json:"tables"`
		} `json:"sources"`
	} `json:"metadata"`
}

type GraphQLResponse struct {
	Data struct {
		TableType struct {
			Name   string `json:"name"`
			Fields []struct {
				Name string `json:"name"`
				Type struct {
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"type"`
			} `json:"fields"`
		} `json:"tableType"`
		PKType struct {
			Name   string `json:"name"`
			Fields []struct {
				Name string `json:"name"`
				Type struct {
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"type"`
			} `json:"fields"`
		} `json:"pkType"`
	} `json:"data"`
	Errors []any `json:"errors"`
}

type BulkOperation struct {
	Type string `json:"type"`
	Args []any  `json:"args"`
}

type EventTrigger struct {
	Type            string     `json:"type"`
	Source          string     `json:"source"`
	ResourceVersion int        `json:"resource_version"`
	MainArgs        []MainArgs `json:"args"`
}

type Update struct {
	Columns []string `json:"columns"`
}
type RetryConf struct {
	NumRetries  int `json:"num_retries"`
	IntervalSec int `json:"interval_sec"`
	TimeoutSec  int `json:"timeout_sec"`
}
type QueryParams struct {
}
type Body struct {
	Action   string `json:"action"`
	Template string `json:"template"`
}
type RequestTransform struct {
	Version        int         `json:"version"`
	TemplateEngine string      `json:"template_engine"`
	Method         string      `json:"method"`
	URL            string      `json:"url"`
	QueryParams    QueryParams `json:"query_params"`
	Body           Body        `json:"body"`
}
type Args struct {
	Name             string           `json:"name"`
	Table            Table            `json:"table"`
	Webhook          string           `json:"webhook"`
	WebhookFromEnv   any              `json:"webhook_from_env"`
	Insert           any              `json:"insert"`
	Update           Update           `json:"update"`
	Delete           any              `json:"delete"`
	EnableManual     bool             `json:"enable_manual"`
	RetryConf        RetryConf        `json:"retry_conf"`
	Replace          bool             `json:"replace"`
	Headers          []any            `json:"headers"`
	RequestTransform RequestTransform `json:"request_transform"`
	Source           string           `json:"source"`
}

type MainArgs struct {
	Type string `json:"type"`
	Args Args   `json:"args"`
}

type Event struct {
	Table Table `json:"table"`
}

type Data struct {
	New map[string]any `json:"new"`
	Old map[string]any `json:"old"`
}

type Table struct {
	Data   Data   `json:"data"`
	Name   string `json:"name"`
	Schema string `json:"schema"`
}
