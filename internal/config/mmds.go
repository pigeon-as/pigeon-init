package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	mmdsAddr     = "http://169.254.169.254"
	mmdsTokenTTL = "60"
)

// FetchMMDS retrieves the RunConfig from MMDS (V2 first, V1 fallback).
func FetchMMDS(ctx context.Context) (*RunConfig, error) {
	data, err := fetchV2(ctx)
	if err != nil {
		data, err = fetchV1(ctx)
		if err != nil {
			return nil, fmt.Errorf("mmds: %w", err)
		}
	}

	var cfg RunConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("mmds: parse config: %w", err)
	}
	return &cfg, nil
}

func fetchV2(ctx context.Context) ([]byte, error) {
	tokenReq, err := http.NewRequestWithContext(ctx, "PUT", mmdsAddr+"/latest/api/token", nil)
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("X-metadata-token-ttl-seconds", mmdsTokenTTL)

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("v2 token: %w", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("v2 token: status %d", tokenResp.StatusCode)
	}

	token, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, fmt.Errorf("v2 token read: %w", err)
	}

	// Fetch metadata with token.
	dataReq, err := http.NewRequestWithContext(ctx, "GET", mmdsAddr, nil)
	if err != nil {
		return nil, err
	}
	dataReq.Header.Set("X-metadata-token", strings.TrimSpace(string(token)))
	dataReq.Header.Set("Accept", "application/json")

	dataResp, err := http.DefaultClient.Do(dataReq)
	if err != nil {
		return nil, fmt.Errorf("v2 get: %w", err)
	}
	defer dataResp.Body.Close()

	if dataResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("v2 get: status %d", dataResp.StatusCode)
	}

	return io.ReadAll(dataResp.Body)
}

func fetchV1(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", mmdsAddr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("v1 get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("v1 get: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
