package main

import (
	"log"
	"net/url"
	"os"
	"time"

	buildkite "github.com/buildkite/go-buildkite/buildkite"
	"github.com/lox/kitewrk/runner"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	Version string
)

func main() {
	run(os.Args[1:], os.Exit)
}

func registerAction(app *kingpin.Application) error {
	var pipeline, org string
	var branch, commit string
	var apiToken string
	var apiEndpoint *url.URL
	var buildCount int
	var debug bool

	app.Flag("debug", "Whether to show debug info").
		BoolVar(&debug)

	app.Flag("org", "The organization to create builds in").
		StringVar(&org)

	app.Flag("pipeline", "The buildkite pipeline to create builds in").
		Required().
		StringVar(&pipeline)

	app.Flag("branch", "The buildkite branch to target").
		Default("master").
		StringVar(&branch)

	app.Flag("commit", "The buildkite commit to target").
		Default("HEAD").
		StringVar(&commit)

	app.Flag("api-token", "A buildkite api token").
		Required().
		StringVar(&apiToken)

	app.Flag("api-endpoint", "The buildkite api endpoint to use").
		URLVar(&apiEndpoint)

	app.Flag("builds", "Number of builds to create").
		Default("8").
		IntVar(&buildCount)

	app.Action(func(c *kingpin.ParseContext) error {
		config, err := buildkite.NewTokenConfig(apiToken, false)
		if err != nil {
			log.Fatalf("Failed to create config: %s", err)
		}

		buildkite.SetHttpDebug(debug)

		t := time.Now()
		client := buildkite.NewClient(config.Client())

		if apiEndpoint != nil {
			client.BaseURL = apiEndpoint
			config.APIHost = apiEndpoint.Host
		}

		result := runner.New(client).Run(runner.Params{
			Org:      org,
			Pipeline: pipeline,
			Builds:   buildCount,
			Branch:   branch,
			Commit:   commit,
		})

		if errs := result.Errors(); len(errs) > 0 {
			log.Fatal(errs)
		}

		s := result.Summary()

		log.Printf("Finished %d builds in %v",
			s.Total,
			time.Now().Sub(t),
		)

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
