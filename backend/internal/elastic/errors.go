// backend/internal/elastic/errors.go

package elastic

import "errors"

var (
	ErrElasticDisabled = errors.New("elastic disabled")
)
