// backend/internal/elastic/client.go

package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	es8 "github.com/elastic/go-elasticsearch/v8"
	esapi "github.com/elastic/go-elasticsearch/v8/esapi"
)

type Client struct {
	es     *es8.Client
	config *Config
}

func NewClient(cfg *Config) (*Client, error) {
	if !cfg.Enabled {
		return &Client{
			es:     nil,
			config: cfg,
		}, nil
	}

	es, err := es8.NewClient(es8.Config{
		Addresses: []string{cfg.Endpoint},
		APIKey:    cfg.APIKey,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create elastic client: %w", err)
	}

	return &Client{
		es:     es,
		config: cfg,
	}, nil
}

func (c *Client) Enabled() bool {
	return c != nil &&
		c.config != nil &&
		c.config.Enabled
}

func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}

	res, err := c.es.Info(
		c.es.Info.WithContext(ctx),
	)

	if err != nil {
		return fmt.Errorf("elastic ping failed: %w", err)
	}

	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("elastic ping error: %s", string(body))
	}

	return nil
}

func (c *Client) indexDocument(
	ctx context.Context,
	index string,
	doc any,
) error {

	if !c.Enabled() {
		return nil
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal elastic document: %w", err)
	}

	res, err := c.es.Index(
		index,
		bytes.NewReader(body),
		c.es.Index.WithContext(ctx),
		c.es.Index.WithRefresh("false"),
	)

	if err != nil {
		return fmt.Errorf("elastic index request failed: %w", err)
	}

	defer res.Body.Close()

	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)

		return fmt.Errorf(
			"elastic indexing error [%s]: %s",
			res.Status(),
			string(respBody),
		)
	}

	return nil
}

func (c *Client) IndexTelemetryEvent(
	ctx context.Context,
	doc TelemetryEventDocument,
) error {

	index := IndexName(
		c.config.IndexTelemetry,
		doc.Timestamp,
	)
	if err := ValidateTelemetryDocument(doc); err != nil {
		return err
	}

	return c.indexDocument(ctx, index, doc)
}

func (c *Client) IndexIncidentEvent(
	ctx context.Context,
	doc IncidentEventDocument,
) error {

	index := IndexName(
		c.config.IndexIncidents,
		doc.Timestamp,
	)

	if err := ValidateIncidentDocument(doc); err != nil {
		return err
	}

	return c.indexDocument(ctx, index, doc)
}

func (c *Client) IndexLogEvent(
	ctx context.Context,
	doc LogEventDocument,
) error {

	index := IndexName(
		c.config.IndexLogs,
		doc.Timestamp,
	)

	if err := ValidateLogDocument(doc); err != nil {
		return err
	}

	return c.indexDocument(ctx, index, doc)
}

func DefaultTimestamp() time.Time {
	return time.Now().UTC()
}

// Search executes a search query against the provided indices and returns the raw response body.
func (c *Client) Search(ctx context.Context, indices []string, query any, size int) ([]byte, error) {
	if !c.Enabled() {
		return nil, nil
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal elastic search query: %w", err)
	}

	opts := []func(*esapi.SearchRequest){
		c.es.Search.WithContext(ctx),
		c.es.Search.WithBody(bytes.NewReader(body)),
	}

	if len(indices) > 0 {
		opts = append(opts, c.es.Search.WithIndex(indices...))
	}
	if size > 0 {
		opts = append(opts, c.es.Search.WithSize(size))
	}

	res, err := c.es.Search(opts...)
	if err != nil {
		return nil, fmt.Errorf("elastic search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("elastic search error [%s]: %s", res.Status(), string(respBody))
	}

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read elastic search response: %w", err)
	}

	// Return raw bytes for higher-level parsing by callers.
	return respBody, nil
}
