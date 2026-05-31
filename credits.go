package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type UsageBreakdown struct {
	DisplayName               string  `json:"displayName"`
	CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
	UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
}

type UsageLimitsResp struct {
	UsageBreakdownList []UsageBreakdown `json:"usageBreakdownList"`
}

func callUsageLimits(token string) (*UsageLimitsResp, error) {
	req, _ := http.NewRequest("GET", "https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result UsageLimitsResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w (body: %s)", err, body)
	}
	return &result, nil
}

// totalUsed returns the sum of current usage across all breakdown items.
func totalUsed(r *UsageLimitsResp) float64 {
	var sum float64
	for _, u := range r.UsageBreakdownList {
		sum += u.CurrentUsageWithPrecision
	}
	return sum
}
