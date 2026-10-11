package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
)

func NewClient(baseUrl string) *Client {
	return &Client{
		baseUrl: baseUrl,
		client:  &http.Client{},
	}
}

func (c *Client) GetUser(token Token) (user *UserResponse, err error) {
	req, err := http.NewRequest("GET", c.baseUrl+"/api/v4/user", nil)
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

func (c *Client) ListProjects(token Token) (projects []Project, err error) {

	req, err := http.NewRequest("GET", c.baseUrl+"/api/v4/projects?membership=true", nil)
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
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
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
