package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

var jobID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var sidecarBase = "http://updater:8989"
var sidecarHTTP = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type JobRef struct {
	JobID string `json:"job_id"`
}
type Job struct {
	State   string  `json:"state"`
	Message *string `json:"message"`
}

func token() string { return os.Getenv("PMAIL_UPDATER_TOKEN") }
func sidecarStatus() UpdaterStatus {
	t := token()
	if os.Getenv("PMAIL_UPDATER_ENABLED") != "1" || len(t) < 32 || len(t) > 256 {
		return UpdaterStatus{}
	}
	kind := "compose-sidecar"
	return UpdaterStatus{true, &kind}
}
func call(ctx context.Context, method, path string, out any) error {
	if !sidecarStatus().Enabled {
		return ErrDisabled
	}
	r, err := http.NewRequestWithContext(ctx, method, sidecarBase+path, nil)
	if err != nil {
		return ErrSidecar
	}
	r.Header.Set("Authorization", "Bearer "+token())
	res, err := sidecarHTTP.Do(r)
	if err != nil {
		return ErrSidecar
	}
	defer res.Body.Close()
	if res.StatusCode == 409 {
		return ErrBusy
	}
	if res.StatusCode == 404 {
		return ErrJob
	}
	if res.StatusCode != 200 && res.StatusCode != 202 {
		return ErrSidecar
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(b) > 4096 || json.Unmarshal(b, out) != nil {
		return ErrSidecar
	}
	return nil
}
func Apply(ctx context.Context) (JobRef, error) {
	var r JobRef
	err := call(ctx, http.MethodPost, "/jobs", &r)
	if err == nil && !jobID.MatchString(r.JobID) {
		err = ErrSidecar
	}
	return r, err
}
func Status(ctx context.Context, id string) (Job, error) {
	if !jobID.MatchString(id) {
		return Job{}, ErrJob
	}
	var job Job
	err := call(ctx, http.MethodGet, "/jobs/"+id, &job)
	if err == ErrJob {
		message := "update_job_lost"
		return Job{State: "failed", Message: &message}, nil
	}
	if err != nil {
		return Job{}, err
	}
	switch job.State {
	case "pending", "pulling", "restarting", "done", "failed":
		return job, nil
	}
	return Job{}, ErrSidecar
}
