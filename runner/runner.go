package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	buildkite "github.com/buildkite/go-buildkite/buildkite"
)

type Runner struct {
	client *buildkite.Client
	org    string
}

type Params struct {
	Org      string
	Pipeline string
	Builds   int
	Branch   string
	Commit   string
}

func (r *Runner) Run(params Params) *Result {
	res := &Result{}

	res.Add(params.Builds)
	go func() {
		for i := 0; i < params.Builds; i++ {
			t := time.Now()
			build, _, err := r.client.Builds.Create(params.Org, params.Pipeline, &buildkite.CreateBuild{
				Commit:  params.Commit,
				Branch:  params.Branch,
				Message: fmt.Sprintf(":rocket: kitewrk build %d of %d", i+1, params.Builds),
			})
			if err != nil {
				log.Println("Error creating build", err)
				res.errors = append(res.errors, err)
				res.Done()
				continue
			}
			log.Printf("Spawned build #%d (%d of %d) in %v",
				*build.Number, i+1, params.Builds,
				durationFmt(time.Now().Sub(t)),
			)

			if build != nil {
				go r.pollBuild(build, i, res)
			}
		}
	}()

	return res
}

func durationFmt(d time.Duration) string {
	return fmt.Sprintf("%0.2fs", d.Seconds())
}

func timestampFmt(a, b *buildkite.Timestamp) string {
	return durationFmt(b.Time.Sub(a.Time))
}

func (r *Runner) pollBuild(b *buildkite.Build, idx int, res *Result) {
	ctx := context.Background()
	defer res.Done()

	for {
		select {
		case <-ctx.Done():
			res.errors = append(res.errors, ctx.Err())
			return

		case <-time.After(1 * time.Second):
			bx, _, err := r.client.Builds.Get(
				getOrgFromURL(*b.URL), *b.Pipeline.Slug, strconv.Itoa(*b.Number),
			)
			if err != nil {
				res.errors = append(res.errors, ctx.Err())
				return
			}
			if bx.FinishedAt != nil {
				if *bx.State == "not_run" {
					log.Printf("Build #%d finished with %q, disable build skipping")
					os.Exit(1)
				}

				log.Printf("Build #%d is %q, finished in %v",
					*bx.Number,
					*bx.State,
					timestampFmt(bx.CreatedAt, bx.FinishedAt),
				)
				log.Printf("\tCreated at %v", bx.CreatedAt.Local().Format(time.StampMilli))
				log.Printf("\tScheduled in %v", timestampFmt(bx.CreatedAt, bx.ScheduledAt))
				log.Printf("\tStarted in %v", timestampFmt(bx.CreatedAt, bx.StartedAt))
				log.Printf("\tFinished in %v", timestampFmt(bx.StartedAt, bx.FinishedAt))
				res.builds = append(res.builds, bx)
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
	WaitTime                []time.Duration
	RunTime                 []time.Duration
}

type Result struct {
	sync.WaitGroup
	errors []error
	builds []*buildkite.Build
}

func (res *Result) Errors() []error {
	res.Wait()
	return res.errors
}

func (res *Result) Summary() (s Summary) {
	res.Wait()
	s.Total = len(res.builds)

	for _, b := range res.builds {
		switch *b.State {
		case "failed":
			s.Failures++
		case "passed":
			s.Passes++
		}
	}
	return
}
