// backend/internal/elastic/templates.go

package elastic

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

func (c *Client) EnsureIndexTemplates(
	ctx context.Context,
) error {

	if !c.Enabled() {
		return nil
	}

	templates := []struct {
		Name string
		Body string
	}{
		{
			Name: "pulseops-telemetry-template",
			Body: telemetryTemplate(),
		},
		{
			Name: "pulseops-incidents-template",
			Body: incidentTemplate(),
		},
		{
			Name: "pulseops-logs-template",
			Body: logsTemplate(),
		},
	}

	for _, tmpl := range templates {

		res, err := c.es.Indices.PutIndexTemplate(
			tmpl.Name,
			bytes.NewReader([]byte(tmpl.Body)),
			c.es.Indices.PutIndexTemplate.WithContext(ctx),
		)

		if err != nil {
			return fmt.Errorf(
				"create template [%s]: %w",
				tmpl.Name,
				err,
			)
		}

		defer res.Body.Close()

		if res.IsError() {

			body, _ := io.ReadAll(res.Body)

			return fmt.Errorf(
				"template [%s] failed [%s]: %s",
				tmpl.Name,
				res.Status(),
				string(body),
			)
		}
	}

	return nil
}

func telemetryTemplate() string {
	return `
{
  "index_patterns": ["telemetry-events-*"],
  "template": {
    "mappings": {
      "properties": {
        "eventType": {
          "type": "keyword"
        },
        "timestamp": {
          "type": "date"
        },
        "deviceId": {
          "type": "keyword"
        },
        "serviceName": {
          "type": "keyword"
        },
        "serviceStatus": {
          "type": "keyword"
        },
        "heartbeat": {
          "type": "boolean"
        },
        "networkReachable": {
          "type": "boolean"
        },
        "cpuUsage": {
          "type": "float"
        },
        "memoryUsage": {
          "type": "float"
        },
        "incidentId": {
          "type": "keyword"
        },
        "recentLogs": {
          "type": "text"
        }
      }
    }
  }
}
`
}

func incidentTemplate() string {
	return `
{
  "index_patterns": ["incident-events-*"],
  "template": {
    "mappings": {
      "properties": {
        "eventType": {
          "type": "keyword"
        },
        "timestamp": {
          "type": "date"
        },
        "incidentId": {
          "type": "keyword"
        },
        "deviceId": {
          "type": "keyword"
        },
        "serviceName": {
          "type": "keyword"
        },
        "serviceStatus": {
          "type": "keyword"
        },
        "severity": {
          "type": "keyword"
        },
        "state": {
          "type": "keyword"
        },
        "reason": {
          "type": "text"
        },
        "active": {
          "type": "boolean"
        }
      }
    }
  }
}
`
}

func logsTemplate() string {
	return `
{
  "index_patterns": ["endpoint-logs-*"],
  "template": {
    "mappings": {
      "properties": {
        "eventType": {
          "type": "keyword"
        },
        "timestamp": {
          "type": "date"
        },
        "deviceId": {
          "type": "keyword"
        },
        "serviceName": {
          "type": "keyword"
        },
        "incidentId": {
          "type": "keyword"
        },
        "message": {
          "type": "text"
        },
        "source": {
          "type": "keyword"
        }
      }
    }
  }
}
`
}
