package runner

import (
	"context"
	"fmt"
	"log"
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
}

func (r *Runner) Run(params Params) *Result {
	res := &Result{}

	res.Add(params.Builds)
	go func() {
		for i := 0; i < params.Builds; i++ {
			log.Printf("Creating build %d", i+1)
			build, _, err := r.client.Builds.Create(params.Org, params.Pipeline, &buildkite.CreateBuild{
				Commit:  "HEAD",
				Branch:  "master",
				Message: fmt.Sprintf(":rocket: kitewrk build %d of %d", i+1, params.Builds),
			})
			if err != nil {
				log.Println("Error creating build", err)
				res.errors = append(res.errors, err)
				res.Done()
				continue
			}
			if build != nil {
				go r.pollBuild(build, i, res)
			}
		}
	}()

	return res
}

func (r *Runner) pollBuild(b *buildkite.Build, idx int, res *Result) {
	ctx := context.Background()
	// ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
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
				log.Printf("Build %d finished at %v", idx+1, *bx.FinishedAt)
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

type Result struct {
	sync.WaitGroup
	errors []error
	builds []*buildkite.Build
}

func (res *Result) Errors() []error {
	res.Wait()
	return res.errors
}
