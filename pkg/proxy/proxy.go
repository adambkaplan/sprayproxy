/*
Copyright © 2023 The Spray Proxy Contributors

SPDX-License-Identifier: Apache-2.0
*/
package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

type BackendsFunc func() []string

type SprayProxy struct {
	backends     BackendsFunc
	inesecureTLS bool
}

func NewSprayProxy(insecureTLS bool, backends ...string) (*SprayProxy, error) {
	backendFn := func() []string {
		return backends
	}

	return &SprayProxy{
		backends:     backendFn,
		inesecureTLS: insecureTLS,
	}, nil
}

func (p *SprayProxy) HandleProxy(c *gin.Context) {
	errors := []error{}
	// Read in body from incoming request
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(c.Request.Body)
	defer c.Request.Body.Close()
	if err != nil {
		c.String(http.StatusRequestEntityTooLarge, "too large: %v", err)
		return
	}
	body := buf.Bytes()

	client := &http.Client{}
	if p.inesecureTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	for _, backend := range p.backends() {
		backendURL, err := url.Parse(backend)
		if err != nil {
			continue
		}
		copy := c.Copy()
		newURL := copy.Request.URL
		newURL.Host = backendURL.Host
		newURL.Scheme = backendURL.Scheme
		newRequest, err := http.NewRequest(copy.Request.Method, newURL.String(), bytes.NewReader(body))
		if err != nil {
			fmt.Printf("failed to create request: %v\n", err)
			errors = append(errors, err)
			continue
		}
		newRequest.Header = copy.Request.Header
		resp, err := client.Do(newRequest)
		if err != nil {
			fmt.Printf("proxy error: %v\n", err)
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		fmt.Printf("proxied request to %s with response %d\n", newURL, resp.StatusCode)
		if resp.StatusCode >= 400 {
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("failed to read response: %v\n", err)
			} else {
				fmt.Printf("response body: %v\n", string(respBody))
			}
		}

		// // Create a new request with a disconnected context
		// newRequest := copy.Request.Clone(context.Background())
		// // Deep copy the request body since this needs to be read multiple times
		// newRequest.Body = io.NopCloser(bytes.NewReader(body))

		// proxy := httputil.NewSingleHostReverseProxy(backendURL)
		// proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		// 	errors = append(errors, err)
		// 	rw.WriteHeader(http.StatusBadGateway)
		// }
		// if p.inesecureTLS {
		// 	proxy.Transport = &http.Transport{
		// 		TLSClientConfig: &tls.Config{
		// 			InsecureSkipVerify: true,
		// 		},
		// 	}
		// }
		// doProxy(backend, proxy, newRequest)
	}
	if len(errors) > 0 {
		// we have a bad gateway/connection somewhere
		c.String(http.StatusBadGateway, "failed to proxy: %v", errors)
		return
	}
	c.String(http.StatusOK, "proxied")
}

func (p *SprayProxy) Backends() []string {
	return p.backends()
}

// InsecureSkipTLSVerify indicates if the proxy is skipping TLS verification.
// This setting is insecure and should not be used in production.
func (p *SprayProxy) InsecureSkipTLSVerify() bool {
	return p.inesecureTLS
}

// doProxy proxies the provided request to a backend, with response data to an "empty" response instance.
func doProxy(dest string, proxy *httputil.ReverseProxy, req *http.Request) {
	writer := NewSprayWriter()
	proxy.ServeHTTP(writer, req)
	fmt.Printf("proxied %s to backend %s\n", req.URL, dest)
}
