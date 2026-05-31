package credits

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Lumos-Labs-HQ/kiromax/internal/db"
	"github.com/Lumos-Labs-HQ/kiromax/internal/session"
)

type usageBreakdown struct {
	DisplayName               string  `json:"displayName"`
	CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
	UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
}

type usageLimitsResp struct {
	UsageBreakdownList []usageBreakdown `json:"usageBreakdownList"`
}

func callUsageLimits(token string) (*usageLimitsResp, error) {
	req, _ := http.NewRequest("GET", "https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result usageLimitsResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(body))
	}
	return &result, nil
}

func Print(s session.Session, dataDB string) error {
	// For active session, always read from live data.sqlite3 (token may be refreshed)
	if s.Active {
		if d, err := db.Open(dataDB); err == nil {
			s.Token, s.TokenExpiresAt = db.ReadToken(d)
			d.Close()
		}
	}
	if s.Token == "" {
		return fmt.Errorf("no token for session %s", s.FileName)
	}
	if !s.TokenExpiresAt.IsZero() && time.Now().After(s.TokenExpiresAt) {
		fmt.Fprintf(os.Stderr, "warning: token expired at %s — swap to this session to refresh\n",
			s.TokenExpiresAt.Local().Format("2006-01-02 15:04"))
	}
	result, err := callUsageLimits(s.Token)
	if err != nil {
		return err
	}
	if len(result.UsageBreakdownList) == 0 {
		fmt.Println("N/A")
		return nil
	}
	var parts []string
	for _, u := range result.UsageBreakdownList {
		parts = append(parts, fmt.Sprintf("%s: %.2f / %.0f", u.DisplayName, u.CurrentUsageWithPrecision, u.UsageLimitWithPrecision))
	}
	fmt.Println(strings.Join(parts, " | "))
	return nil
}
