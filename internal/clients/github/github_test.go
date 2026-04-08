package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_ConfiguresTimeout(t *testing.T) {
	t.Parallel()

	c := NewClient("token")
	require.NotNil(t, c.github)
	require.NotNil(t, c.github.Client())
	assert.Equal(t, githubHTTPTimeout, c.github.Client().Timeout)
}

func TestClient_ListAccessibleRepos_SkipsArchived(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "all", r.URL.Query().Get("visibility"))
		assert.Equal(t, "owner,collaborator,organization_member", r.URL.Query().Get("affiliation"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"active-repo","full_name":"octo/active-repo","private":true,"html_url":"https://github.com/octo/active-repo","archived":false,"owner":{"login":"octo"}},
			{"name":"old-repo","full_name":"octo/old-repo","private":false,"html_url":"https://github.com/octo/old-repo","archived":true,"owner":{"login":"octo"}}
		]`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)

	repos, err := client.ListAccessibleRepos(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "octo/active-repo", repos[0].FullName)
	assert.Equal(t, "octo", repos[0].Owner)
	assert.Equal(t, "active-repo", repos[0].Name)
}

func TestClient_ListAccessibleRepos_Paginates(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "", "1":
			w.Header().Set("Link", "</user/repos?page=2>; rel=\"next\"")
			_, _ = w.Write([]byte(`[{"name":"repo-1","full_name":"octo/repo-1","private":false,"html_url":"https://github.com/octo/repo-1","archived":false,"owner":{"login":"octo"}}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"name":"repo-2","full_name":"octo/repo-2","private":false,"html_url":"https://github.com/octo/repo-2","archived":false,"owner":{"login":"octo"}}]`))
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	repos, err := client.ListAccessibleRepos(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 2)
	assert.Equal(t, "octo/repo-1", repos[0].FullName)
	assert.Equal(t, "octo/repo-2", repos[1].FullName)
}

func TestClient_GetReadme(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octo/repo/readme", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		content := base64.StdEncoding.EncodeToString([]byte("# hello"))
		_, _ = w.Write([]byte(fmt.Sprintf(`{"sha":"sha-123","path":"README.md","content":"%s","encoding":"base64"}`, content)))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	readme, err := client.GetReadme(context.Background(), "octo", "repo")
	require.NoError(t, err)
	assert.Equal(t, "octo/repo", readme.RepoFullName)
	assert.Equal(t, "sha-123", readme.SHA)
	assert.Equal(t, "README.md", readme.Path)
	assert.Equal(t, "# hello", readme.Content)
}

func TestClient_GetReadme_NotFound(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octo/repo/readme", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetReadme(context.Background(), "octo", "repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, errReadmeNotFound)
}

func TestClient_GetReadme_ValidationErrors(t *testing.T) {
	t.Parallel()

	client := NewClient("token")

	_, err := client.GetReadme(context.Background(), " ", "repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOwnerNameRequired)
}

func TestClient_ListAccessibleRepos_MissingToken(t *testing.T) {
	t.Parallel()

	client := NewClient(" ")
	_, err := client.ListAccessibleRepos(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errMissingGitHubToken)
}

func newTestClient(t *testing.T, baseURL string) *client {
	t.Helper()

	c := NewClient("token")

	parsed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	c.github.BaseURL = parsed
	c.github.UploadURL = parsed
	return c
}
