package annotator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Annotator struct {
	url       string
	token     string
	dashboard string
	client    *http.Client
}

func New(grafanaURL, token, dashboardUID string) *Annotator {
	if grafanaURL == "" || token == "" {
		return nil
	}
	return &Annotator{
		url:       grafanaURL,
		token:     token,
		dashboard: dashboardUID,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

type annotation struct {
	DashboardUID string   `json:"dashboardUID,omitempty"`
	Time         int64    `json:"time"`
	Text         string   `json:"text"`
	Tags         []string `json:"tags"`
}

func (a *Annotator) Annotate(text string, tags ...string) {
	if a == nil {
		return
	}

	ann := annotation{
		DashboardUID: a.dashboard,
		Time:         time.Now().UnixMilli(),
		Text:         text,
		Tags:         tags,
	}

	body, _ := json.Marshal(ann)
	url := fmt.Sprintf("%s/api/annotations", a.url)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[annotator] create request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[annotator] post: %v", err)
		return
	}
	resp.Body.Close()
}
