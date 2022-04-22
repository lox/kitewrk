package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/lox/kitewrk/buildkite"
)

type Runner struct {
	client *buildkite.Client
	org    string
}

type Params struct {
	PipelineID string
	Builds     int
	Branch     string
	Commit     string
}

func (r *Runner) Run(params Params) *Result {
	res := &Result{}

	res.Add(params.Builds)
	go func() {
		for i := 0; i < params.Builds; i++ {
			t := time.Now()
			buildResp, err := r.client.BuildCreate(buildkite.BuildCreateParams{
				PipelineID: params.PipelineID,
				Commit:     params.Commit,
				Branch:     params.Branch,
				Message:    fmt.Sprintf(":rocket: kitewrk build %d of %d", i+1, params.Builds),
			})
			if err != nil {
				fmt.Println("Error creating build", err)
				res.errors = append(res.errors, err)
				res.Done()
				continue
			}
			fmt.Printf("Spawned build #%d (%d of %d) in %v\n",
				buildResp.Number, i+1, params.Builds,
				durationFmt(time.Now().Sub(t)),
			)

			if buildResp != nil {
				go r.pollBuild(*buildResp, i, res)
			}
		}
	}()

	return res
}

func durationFmt(d time.Duration) string {
	return fmt.Sprintf("%0.2fs", d.Seconds())
}

func timestampFmt(a, b time.Time) string {
	return durationFmt(b.Sub(a))
}

func (r *Runner) pollBuild(br buildkite.BuildCreateResponse, idx int, res *Result) {
	ctx := context.Background()
	defer res.Done()

	for {
		select {
		case <-ctx.Done():
			res.errors = append(res.errors, ctx.Err())
			return

		case <-time.After(1 * time.Second):
			buildResp, err := r.client.GetBuild(buildkite.GetBuildParams{
				Slug: fmt.Sprintf("%s/%s/%d", br.OrgSlug, br.PipelineSlug, br.Number),
			})
			if err != nil {
				res.errors = append(res.errors, ctx.Err())
				return
			}

			if buildResp.FinishedAt != nil {
				if buildResp.State == "not_run" {
					fmt.Printf("Build #%d finished with %q, disable build skipping\n",
						buildResp.Number, buildResp.State)
					os.Exit(1)
				}

				log.Printf("Build #%d is %q, finished in %v\n",
					buildResp.Number,
					buildResp.State,
					durationFmt(buildResp.Time),
				)
				fmt.Printf("\tCreated at %v\n", buildResp.CreatedAt.Local().Format(time.StampMilli))
				fmt.Printf("\tJob Wait Time %v\n", durationFmt(buildResp.JobWaitTime))
				fmt.Printf("\tJob Run Time %v\n", durationFmt(buildResp.JobTime))
				res.builds = append(res.builds, *buildResp)
				return
			}
		}
	}
}

func getOrgFromURL(u string) string {
	re := regexp.MustCompile(`organizations/(.+?)/pipelines`)
	match := re.FindStringSubmatch(u)
	return match[1]
}

func New(client *buildkite.Client) *Runner {
	return &Runner{
		client: client,
	}
}

type Summary struct {
	Total, Passes, Failures int
	BuildTimes              []time.Duration
	JobWaitTimes            []time.Duration
	JobRunTimes             []time.Duration
}

type Result struct {
	sync.WaitGroup
	errors []error
	builds []buildkite.GetBuildResponse
}

func (res *Result) Errors() []error {
	res.Wait()
	return res.errors
}

func (res *Result) Summary() (s Summary) {
	res.Wait()
	s.Total = len(res.builds)

	for _, b := range res.builds {
		switch b.State {
		case buildkite.BuildPassedState:
			s.Passes++
			s.BuildTimes = append(s.BuildTimes, b.Time)
			s.JobWaitTimes = append(s.JobWaitTimes, b.JobWaitTime)
			s.JobRunTimes = append(s.JobRunTimes, b.JobTime)
		default:
			s.Failures++
		}
	}
	return
}
