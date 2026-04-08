package anytypefactory

import (
	"errors"
	"strings"

	anytypesdk "github.com/epheo/anytype-go"
	_ "github.com/epheo/anytype-go/client"
)

var ErrAppKeyRequired = errors.New("anytype app key is required")

type Factory struct{ baseURL string }

func New(baseURL string) *Factory {
	return &Factory{baseURL: strings.TrimSpace(baseURL)}
}

func (f *Factory) UnauthedClient() anytypesdk.Client {
	return anytypesdk.NewClient(anytypesdk.WithBaseURL(f.baseURL))
}

func (f *Factory) AuthedClient(appKey string) (anytypesdk.Client, error) {
	trimmedKey := strings.TrimSpace(appKey)
	if trimmedKey == "" {
		return nil, ErrAppKeyRequired
	}

	return anytypesdk.NewClient(
		anytypesdk.WithBaseURL(f.baseURL),
		anytypesdk.WithAppKey(trimmedKey),
	), nil
}
