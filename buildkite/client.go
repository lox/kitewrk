package buildkite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	graphQLEndpoint = "https://graphql.buildkite.com/v1"

	BuildPassedState = `PASSED`
)

type Build struct {
	State string
}

type Timestamp struct {
	time.Time
}

// NewClient returns a new Buildkite GraphQL client
func NewClient(token string) (*Client, error) {
	u, err := url.Parse(graphQLEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse graphql endpoint url")
	}
	header := make(http.Header)
	header.Add("Content-Type", "application/json")
	header.Add("Authorization", "Bearer "+token)
	return &Client{
		token:      token,
		endpoint:   u,
		header:     header,
		httpClient: http.DefaultClient,
	}, nil
}

// Client is a Buildkite GraphQL client
type Client struct {
	token      string
	endpoint   *url.URL
	httpClient *http.Client
	header     http.Header
}

// Do sends a GraphQL query with bound variables and returns a Response
func (c *Client) Do(query string, vars map[string]interface{}) (*Response, error) {
	b, err := json.MarshalIndent(struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     strings.TrimSpace(query),
		Variables: vars,
	}, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal vars")
	}

	req, err := http.NewRequest(http.MethodPost, c.endpoint.String(), bytes.NewReader(b))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create http request")
	}
	req.Header = c.header

	if os.Getenv(`DEBUG`) != "" {
		if dump, err := httputil.DumpRequest(req, true); err == nil {
			fmt.Printf("DEBUG request uri=%s\n%s\n", req.URL, dump)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}

	if os.Getenv(`DEBUG`) != "" {
		if dump, err := httputil.DumpResponse(resp, true); err == nil {
			fmt.Printf("DEBUG response uri=%s\n%s\n", req.URL, dump)
		}
	}

	return &Response{resp}, checkResponseForErrors(resp)
}

// Response is a GraphQL response
type Response struct {
	*http.Response
}

// DecodeInto decodes a JSON body into the provided type
func (r *Response) DecodeInto(v interface{}) error {
	return errors.Wrap(json.NewDecoder(r.Body).Decode(v), "error decoding response")
}

type responseError struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (r *responseError) Error() string {
	var errors []string
	for _, err := range r.Errors {
		errors = append(errors, err.Message)
	}
	return fmt.Sprintf("graphql error: %s", strings.Join(errors, ", "))
}

func checkResponseForErrors(r *http.Response) error {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read body")
	}

	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	var errResp responseError

	_ = json.Unmarshal(data, &errResp)
	if len(errResp.Errors) > 0 {
		return &errResp
	}

	if r.StatusCode != http.StatusOK {
		return errors.Errorf("response returned status %s", r.Status)
	}

	return nil
}

type BuildCreateParams struct {
	PipelineID string
	Commit     string
	Branch     string
	Message    string
}

type BuildCreateResponse struct {
	OrgSlug      string
	PipelineSlug string
	URL          string
	Number       int
}

func (c *Client) BuildCreate(params BuildCreateParams) (*BuildCreateResponse, error) {
	resp, err := c.Do(`mutation($input: BuildCreateInput!) {
		buildCreate(input: $input) {
		  build {
			url
			number
			organization {
			  slug
			}
			pipeline {
			  slug
			}
		  }
		}
	  }`, map[string]interface{}{
		`input`: map[string]interface{}{
			"pipelineID": params.PipelineID,
			"commit":     params.Commit,
			"branch":     params.Branch,
			"message":    params.Message,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create build")
	}

	if err := checkResponseForErrors(resp.Response); err != nil {
		return nil, err
	}

	var r struct {
		Data struct {
			BuildCreate struct {
				Build struct {
					URL      string `json:"url"`
					Number   int    `json:"number"`
					Pipeline struct {
						Slug string `json:"slug"`
					} `json:"pipeline"`
					Organization struct {
						Slug string `json:"slug"`
					} `json:"organization"`
				} `json:"build"`
			} `json:"buildCreate"`
		} `json:"data"`
	}

	if err := resp.DecodeInto(&r); err != nil {
		return nil, err
	}

	return &BuildCreateResponse{
		URL:          r.Data.BuildCreate.Build.URL,
		Number:       r.Data.BuildCreate.Build.Number,
		OrgSlug:      r.Data.BuildCreate.Build.Organization.Slug,
		PipelineSlug: r.Data.BuildCreate.Build.Pipeline.Slug,
	}, nil
}

type GetBuildParams struct {
	Slug string
}

type GetBuildResponse struct {
	Number      int
	State       string
	CreatedAt   *time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Time        time.Duration
	JobTime     time.Duration
	JobWaitTime time.Duration
}

func (c *Client) GetBuild(params GetBuildParams) (*GetBuildResponse, error) {
	resp, err := c.Do(`query($buildSlug:ID!){
		build(slug: $buildSlug) {
			number
			state
			createdAt
			scheduledAt
			finishedAt
			startedAt
			finishedAt
			jobs(first: 1) {
			edges {
				node {
				... on JobTypeCommand {
					state
					createdAt
					runnableAt
					scheduledAt
					finishedAt
					startedAt
					finishedAt
				}
				}
			}
			}
		}
	}`, map[string]interface{}{
		`buildSlug`: params.Slug,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get org member")
	}

	if err := checkResponseForErrors(resp.Response); err != nil {
		return nil, err
	}

	var r struct {
		Data struct {
			Build struct {
				Number     int        `json:"number"`
				State      string     `json:"state"`
				CreatedAt  *time.Time `json:"createdAt"`
				StartedAt  *time.Time `json:"startedAt"`
				FinishedAt *time.Time `json:"finishedAt"`
				Jobs       struct {
					Edges []struct {
						Node struct {
							State      string     `json:"state"`
							StartedAt  *time.Time `json:"startedAt"`
							FinishedAt *time.Time `json:"finishedAt"`
							RunnableAt *time.Time `json:"runnableAt"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"jobs"`
			} `json:"build"`
		} `json:"data"`
	}

	if err := resp.DecodeInto(&r); err != nil {
		return nil, err
	}

	var buildTime, jobWaitTime, jobTime time.Duration

	for _, edge := range r.Data.Build.Jobs.Edges {
		if edge.Node.State == `FINISHED` {
			if edge.Node.RunnableAt != nil {
				jobWaitTime = jobWaitTime + edge.Node.StartedAt.Sub(*edge.Node.RunnableAt)
			}
			if edge.Node.FinishedAt != nil {
				jobTime = jobTime + edge.Node.FinishedAt.Sub(*edge.Node.StartedAt)
			}
		}
	}

	if r.Data.Build.FinishedAt != nil {
		buildTime = r.Data.Build.FinishedAt.Sub(*r.Data.Build.StartedAt)
	}

	return &GetBuildResponse{
		Number:      r.Data.Build.Number,
		State:       r.Data.Build.State,
		CreatedAt:   r.Data.Build.CreatedAt,
		StartedAt:   r.Data.Build.StartedAt,
		FinishedAt:  r.Data.Build.FinishedAt,
		Time:        buildTime,
		JobTime:     jobTime,
		JobWaitTime: jobWaitTime,
	}, nil
}
