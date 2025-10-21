// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	goversion "github.com/hashicorp/go-version"

	"github.com/percona/everest-operator/api/v1alpha1"
)

type pmmErrorMessage struct {
	Message string `json:"message"`
}

// CreatePMMApiKey creates a new API key in PMM by using the provided username and password.
func CreatePMMApiKey(
	ctx context.Context,
	hostname, apiKeyName, user, password string,
	skipTLSVerify bool,
) (string, error) {
	auth := BasicAuth{
		User:     user,
		Password: password,
	}
	version, err := GetPMMVersion(ctx, hostname, auth, skipTLSVerify)
	if err != nil {
		return "", err
	}

	// PMM2 and PMM3 use different API to create tokens
	if version.UsesLegacyAuth() {
		return createKey(ctx, hostname, apiKeyName, auth, skipTLSVerify)
	} else {
		return createServiceAccountAndToken(ctx, hostname, apiKeyName, auth, skipTLSVerify)
	}
}

// GetPMMVersion makes an API request to the PMM server to figure out the current version
func GetPMMVersion(ctx context.Context, hostname string, auth Auth, skipTLSVerify bool) (*v1alpha1.PMMServerVersion, error) {
	resp, err := doJSONRequest[struct {
		Version string `json:"version"`
	}](ctx, http.MethodGet, fmt.Sprintf("%s/v1/version", hostname), auth, "", skipTLSVerify)
	if err != nil {
		return nil, err
	}
	v, err := goversion.NewVersion(resp.Version)

	return &v1alpha1.PMMServerVersion{Version: *v}, nil
}

func createKey(ctx context.Context, hostname, apiKeyName string, auth Auth, skipTLSVerify bool) (string, error) {
	body := nameAndRoleMap(apiKeyName)
	resp, err := doJSONRequest[map[string]interface{}](ctx, http.MethodPost, fmt.Sprintf("%s/graph/api/auth/keys", hostname), auth, body, skipTLSVerify)
	if err != nil {
		return "", err
	}
	key, ok := resp["key"].(string)
	if !ok {
		return "", errors.New("cannot unmarshal key in createAdminToken")
	}
	return key, nil
}

func createServiceAccountAndToken(ctx context.Context, hostname, apiKeyName string, auth Auth, skipTLSVerify bool) (string, error) {
	// for transparency, use the same name for the generated service account and token
	nameAndRole := nameAndRoleMap(apiKeyName)
	account, err := doJSONRequest[struct {
		Uid string `json:"uid"`
	}](ctx, http.MethodPost, fmt.Sprintf("%s/graph/api/serviceaccounts", hostname), auth, nameAndRole, skipTLSVerify)
	if err != nil {
		return "", err
	}
	token, err := doJSONRequest[struct {
		Key string `json:"key"`
	}](ctx, http.MethodPost, fmt.Sprintf("%s/graph/api/serviceaccounts/%s/tokens", hostname, account.Uid), auth, nameAndRole, skipTLSVerify)
	if err != nil {
		return "", err
	}

	return token.Key, nil
}

// makes an HTTP request using JSON content type
func doJSONRequest[T any](ctx context.Context, method, url string, auth Auth, body any, skipTLSVerify bool) (T, error) {
	var zero T
	b, err := json.Marshal(body)
	if err != nil {
		return zero, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return zero, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if auth != nil {
		auth.Apply(req)
	}
	req.Close = true

	httpClient := newHTTPClient(skipTLSVerify)
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var pmmErr *pmmErrorMessage
		if err := json.Unmarshal(data, &pmmErr); err != nil {
			return zero, errors.Join(err, fmt.Errorf("PMM returned an unknown error. HTTP %d", resp.StatusCode))
		}
		return zero, fmt.Errorf("PMM returned an error: %s", pmmErr.Message)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("unmarshal response: %w", err)
	}

	return result, nil
}

// Auth an interface to apply auth to a request.
type Auth interface {
	Apply(req *http.Request)
}

// BasicAuth represents basic auth with User/Password
type BasicAuth struct {
	User     string
	Password string
}

func (a BasicAuth) Apply(req *http.Request) {
	req.SetBasicAuth(a.User, a.Password)
}

// BearerAuth represents bearer auth with a token
type BearerAuth struct {
	Token string
}

func (a BearerAuth) Apply(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.Token)
}

func nameAndRoleMap(name string) map[string]string {
	return map[string]string{
		"name": name,
		"role": "Admin",
	}
}

func newHTTPClient(insecure bool) *http.Client {
	client := http.DefaultClient
	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure, //nolint:gosec
		},
	}
	return client
}
