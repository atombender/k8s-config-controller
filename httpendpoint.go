package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cenk/backoff"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type HTTPEndpoint struct {
	url    string
	method string
	client *http.Client
}

func NewHTTPEndpoint(url, method string) *HTTPEndpoint {
	return &HTTPEndpoint{
		url:    url,
		method: method,
		client: &http.Client{},
	}
}

// Implements Reloadable
func (e *HTTPEndpoint) Reload() error {
	req, err := http.NewRequest(e.method, e.url, nil)
	if err != nil {
		return errors.Wrapf(err, "Unable to create request for %s", e.url)
	}

	boff := backoff.NewExponentialBackOff()
	boff.MaxElapsedTime = 10 * time.Second

	if err := backoff.Retry(func() error {
		glog.Infof("Reloading with %s to %s", e.method, e.url)

		resp, err := e.client.Do(req)
		if err != nil {
			glog.Warningf("%s request to %s failed", e.method, e.url)
			return errors.Wrapf(err, "%s request to %s failed", e.method, e.url)
		}
		if resp.StatusCode != 200 {
			glog.Warningf("Unexpected status code %d from %s", resp.StatusCode, e.url)
			return fmt.Errorf("Unexpected status code %d from %s", resp.StatusCode, e.url)
		}
		return nil
	}, boff); err != nil {
		return err
	}

	glog.Info("Reload request succeeded")
	return nil
}
