package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v73/github"
	"golang.org/x/oauth2"
)

const (
	defaultPerPage    = 100
	githubHTTPTimeout = 20 * time.Second
)

var (
	errOwnerNameRequired  = errors.New("owner and name are required")
	errMissingGitHubToken = errors.New("missing GitHub token (GH_TOKEN)")
	errReadmeNotFound     = errors.New("readme not found")
)

type client struct {
	github *gh.Client
	token  string
}

type Repository struct {
	Owner    string
	Name     string
	FullName string
	Private  bool
	HTMLURL  string
}

type Readme struct {
	RepoFullName string
	SHA          string
	Path         string
	Content      string
}

// NewClient applies auth and timeout defaults in one place so every GitHub
// call inherits the same reliability and security baseline.
func NewClient(token string) *client {
	trimmedToken := strings.TrimSpace(token)
	httpClient := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: trimmedToken}))
	httpClient.Timeout = githubHTTPTimeout

	return &client{
		github: gh.NewClient(httpClient),
		token:  trimmedToken,
	}
}

// ListAccessibleRepos filters archived repositories to keep the sync workflow
// focused on actively maintained content.
func (c *client) ListAccessibleRepos(ctx context.Context) ([]Repository, error) {
	if err := c.validateToken(); err != nil {
		return nil, err
	}

	var repos []Repository
	opts := &gh.RepositoryListByAuthenticatedUserOptions{
		Visibility:  "all",
		Affiliation: "owner,collaborator,organization_member",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: gh.ListOptions{PerPage: defaultPerPage},
	}

	for {
		rawRepos, response, err := c.github.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}

		for _, r := range rawRepos {
			if r.GetArchived() {
				continue
			}
			repos = append(repos, Repository{
				Owner:    r.GetOwner().GetLogin(),
				Name:     r.GetName(),
				FullName: r.GetFullName(),
				Private:  r.GetPrivate(),
				HTMLURL:  r.GetHTMLURL(),
			})
		}

		if response == nil || response.NextPage == 0 {
			break
		}

		opts.Page = response.NextPage
	}

	return repos, nil
}

// GetReadme normalizes README retrieval so higher layers can reason in terms of
// markdown content and domain errors, not API response formats.
func (c *client) GetReadme(ctx context.Context, owner, name string) (Readme, error) {
	if err := c.validateToken(); err != nil {
		return Readme{}, err
	}

	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return Readme{}, errOwnerNameRequired
	}

	readme, _, err := c.github.Repositories.GetReadme(ctx, owner, name, nil)
	if err != nil {
		var apiErr *gh.ErrorResponse
		if errors.As(err, &apiErr) && apiErr.Response != nil && apiErr.Response.StatusCode == http.StatusNotFound {
			return Readme{}, fmt.Errorf("%w for repository %s/%s", errReadmeNotFound, owner, name)
		}
		return Readme{}, fmt.Errorf("get readme: %w", err)
	}
	decoded, err := readme.GetContent()
	if err != nil {
		return Readme{}, fmt.Errorf("decode readme content: %w", err)
	}

	return Readme{
		RepoFullName: fmt.Sprintf("%s/%s", owner, name),
		SHA:          readme.GetSHA(),
		Path:         readme.GetPath(),
		Content:      decoded,
	}, nil
}

func (c *client) validateToken() error {
	if c.token == "" {
		return errMissingGitHubToken
	}
	return nil
}
