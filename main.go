package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lox/kitewrk/buildkite"
	"github.com/lox/kitewrk/runner"

	"github.com/bsipos/thist"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	Version string
)

func main() {
	run(os.Args[1:], os.Exit)
}

func registerAction(app *kingpin.Application) error {
	var pipelineID, org, pipelineSlug string
	var branch, commit string
	var graphqlToken string
	var buildCount int
	var debug bool

	app.Flag("debug", "Whether to show debug info").
		BoolVar(&debug)

	app.Flag("org", "The organization to create builds in").
		StringVar(&org)

	app.Flag("pipeline-slug", "The buildkite pipeline to create builds in").
		StringVar(&pipelineSlug)

	app.Flag("pipeline-id", "The buildkite pipeline to create builds in").
		StringVar(&pipelineID)

	app.Flag("branch", "The buildkite branch to target").
		Default("master").
		StringVar(&branch)

	app.Flag("commit", "The buildkite commit to target").
		Default("HEAD").
		StringVar(&commit)

	app.Flag("graphql-token", "A buildkite graphql token").
		Required().
		StringVar(&graphqlToken)

	app.Flag("builds", "Number of builds to create").
		Default("8").
		IntVar(&buildCount)

	app.Action(func(c *kingpin.ParseContext) error {
		t := time.Now()

		client, err := buildkite.NewClient(graphqlToken)
		if err != nil {
			return err
		}

		result := runner.New(client).Run(runner.Params{
			PipelineID: pipelineID,
			Builds:     buildCount,
			Branch:     branch,
			Commit:     commit,
		})

		if errs := result.Errors(); len(errs) > 0 {
			log.Fatal(errs)
		}

		s := result.Summary()

		log.Printf("%#v", s)

		log.Printf("Finished %d builds in %v",
			s.Total,
			time.Now().Sub(t),
		)

		h := thist.NewHist(nil, "Wait Times", "auto", -1, true)

		for _, t := range s.JobWaitTimes {
			h.Update(t.Seconds())
		}

		fmt.Println(h.Draw())

		// log.Printf("Average Build time: %v", s.JobRunTimes)
		// log.Printf("Average Wait time: %v")

		return nil
	})

	return nil
}

func run(args []string, exit func(code int)) {
	app := kingpin.New("kitewrk",
		`A tool for benchmarking and load-testing Buildkite builds`)

	app.Version(Version)
	app.Writer(os.Stdout)
	app.DefaultEnvars()
	app.Terminate(exit)

	registerAction(app)

	kingpin.MustParse(app.Parse(args))
}
