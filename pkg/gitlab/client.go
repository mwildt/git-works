package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

type (
	Token struct {
		Value     string
		TokenType string
	}

	Client struct {
		client  *http.Client
		baseUrl string
	}

	UserResponse struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		State    string `json:"state"`
		WebURL   string `json:"web_url"`
	}

	Project struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
		HttpRepoUrl       string `json:"http_url_to_repo"`
		LastActivityAt    string `json:"last_activity_at"`
	}

	Branch struct {
		Name   string `json:"name"`
		WebUrl string `json:"web_url"`
		Commit struct {
			Id         string `json:"id"`
			AuthorName string `json:"author_name"`
			CreatedAt  string `json:"created_at"`
		}
	}

	ProjectSearchResult struct {
		Projects []Project `json:"projects"`
		NextPage int       `json:"next_page"`
	}
)

func NewClient(baseUrl string) *Client {
	return &Client{
		baseUrl: baseUrl,
		client:  &http.Client{},
	}
}

func (c *Client) GetUser(token Token) (user *UserResponse, err error) {
	url := c.baseUrl + "/api/v4/user"
	slog.Default().Info("Getting user", "url", url, "token-type", token.TokenType)
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set(token.TokenType, token.Value)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	if err = json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return
	}
	return
}

func (c *Client) SearchProjects(token Token, search string, page int) (*ProjectSearchResult, error) {
	if page < 1 {
		page = 1
	}
	query := url.Values{}
	query.Set("search", search)
	query.Set("search_namespaces", "true")
	query.Set("membership", "true")
	query.Set("simple", "true")
	query.Set("page", strconv.Itoa(page))
	query.Set("per_page", "20")

	req, err := http.NewRequest("GET", c.baseUrl+"/api/v4/projects?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(token.TokenType, token.Value)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab: %s: %s", resp.Status, body)
	}
	result := &ProjectSearchResult{}
	if err := json.NewDecoder(resp.Body).Decode(&result.Projects); err != nil {
		return nil, err
	}
	if next := resp.Header.Get("X-Next-Page"); next != "" {
		if nextPage, err := strconv.Atoi(next); err == nil {
			result.NextPage = nextPage
		}
	}
	return result, nil
}

func (c *Client) ListBranches(token Token, projectid string) (branches []Branch, err error) {
	url := c.baseUrl + "/api/v4/projects/" + projectid + "/repository/branches"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(token.TokenType, token.Value)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab: %s: %s", resp.Status, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}
	return branches, nil
}
