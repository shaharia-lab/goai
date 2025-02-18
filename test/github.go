package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-github/v60/github"
	"github.com/shaharia-lab/goai/mcp"
	"golang.org/x/oauth2"
	"log"
	"os"
	"time"
)

// getGitHubClient creates a new GitHub client using token from environment
func getGitHubClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc), nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intValue(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func timeValue(t *github.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

var githubCreateRepository = mcp.Tool{
	Name:        "create_repository",
	Description: "Create a new GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Repository name"
			},
			"description": {
				"type": "string",
				"description": "Repository description"
			},
			"private": {
				"type": "boolean",
				"description": "Whether repository should be private"
			},
			"autoInit": {
				"type": "boolean",
				"description": "Initialize with README"
			}
		},
		"required": ["name"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Private     bool   `json:"private"`
			AutoInit    bool   `json:"autoInit"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		repo := &github.Repository{
			Name:        github.String(input.Name),
			Description: github.String(input.Description),
			Private:     github.Bool(input.Private),
			AutoInit:    github.Bool(input.AutoInit),
		}

		repository, _, err := client.Repositories.Create(ctx, "", repo)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Repository created successfully: %s", repository.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubCreateIssue = mcp.Tool{
	Name:        "create_issue",
	Description: "Create a new issue in a GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"title": {
				"type": "string",
				"description": "Issue title"
			},
			"body": {
				"type": "string",
				"description": "Issue description"
			},
			"labels": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Labels to add"
			}
		},
		"required": ["owner", "repo", "title"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner  string   `json:"owner"`
			Repo   string   `json:"repo"`
			Title  string   `json:"title"`
			Body   string   `json:"body"`
			Labels []string `json:"labels"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		issue := &github.IssueRequest{
			Title:  github.String(input.Title),
			Body:   github.String(input.Body),
			Labels: &input.Labels,
		}

		createdIssue, _, err := client.Issues.Create(ctx, input.Owner, input.Repo, issue)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Issue created successfully: %s", createdIssue.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubGetFileContents = mcp.Tool{
	Name:        "get_file_contents",
	Description: "Get contents of a file from a GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"path": {
				"type": "string",
				"description": "Path to file"
			},
			"branch": {
				"type": "string",
				"description": "Branch name (optional)"
			}
		},
		"required": ["owner", "repo", "path"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner  string `json:"owner"`
			Repo   string `json:"repo"`
			Path   string `json:"path"`
			Branch string `json:"branch"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.RepositoryContentGetOptions{
			Ref: input.Branch,
		}

		content, _, _, err := client.Repositories.GetContents(ctx, input.Owner, input.Repo, input.Path, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		decodedContent, err := content.GetContent()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "text",
					Text: decodedContent,
				},
			},
		}, nil
	},
}

var githubCreateOrUpdateFile = mcp.Tool{
	Name:        "create_or_update_file",
	Description: "Create or update a single file in a GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner (username or organization)"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"path": {
				"type": "string",
				"description": "Path where to create/update the file"
			},
			"content": {
				"type": "string",
				"description": "Content of the file"
			},
			"message": {
				"type": "string",
				"description": "Commit message"
			},
			"branch": {
				"type": "string",
				"description": "Branch to create/update the file in"
			},
			"sha": {
				"type": "string",
				"description": "SHA of file being replaced (for updates)"
			}
		},
		"required": ["owner", "repo", "path", "content", "message", "branch"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner   string `json:"owner"`
			Repo    string `json:"repo"`
			Path    string `json:"path"`
			Content string `json:"content"`
			Message string `json:"message"`
			Branch  string `json:"branch"`
			SHA     string `json:"sha,omitempty"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		fileContent := &github.RepositoryContentFileOptions{
			Message: github.String(input.Message),
			Content: []byte(input.Content),
			Branch:  github.String(input.Branch),
		}

		if input.SHA != "" {
			fileContent.SHA = github.String(input.SHA)
		}

		commit, _, err := client.Repositories.CreateFile(
			ctx,
			input.Owner,
			input.Repo,
			input.Path,
			fileContent,
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("File successfully created/updated. Commit SHA: %s", commit.GetSHA()),
				},
			},
		}, nil
	},
}

var githubPushFiles = mcp.Tool{
	Name:        "push_files",
	Description: "Push multiple files in a single commit",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"branch": {
				"type": "string",
				"description": "Branch to push to"
			},
			"message": {
				"type": "string",
				"description": "Commit message"
			},
			"files": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "File path"
						},
						"content": {
							"type": "string",
							"description": "File content"
						}
					},
					"required": ["path", "content"]
				}
			}
		},
		"required": ["owner", "repo", "branch", "message", "files"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner   string `json:"owner"`
			Repo    string `json:"repo"`
			Branch  string `json:"branch"`
			Message string `json:"message"`
			Files   []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"files"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get the reference to the branch
		ref, _, err := client.Git.GetRef(ctx, input.Owner, input.Repo, "refs/heads/"+input.Branch)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to get ref: %v", err)
		}

		// Get the tree based on the commit
		baseTree, _, err := client.Git.GetTree(ctx, input.Owner, input.Repo, *ref.Object.SHA, false)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to get tree: %v", err)
		}

		// Create tree entries
		entries := make([]*github.TreeEntry, 0, len(input.Files))
		for _, file := range input.Files {
			entries = append(entries, &github.TreeEntry{
				Path:    github.String(file.Path),
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				Content: github.String(file.Content),
			})
		}

		// Create a new tree
		tree, _, err := client.Git.CreateTree(ctx, input.Owner, input.Repo, *baseTree.SHA, entries)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to create tree: %v", err)
		}

		// Get the parent commit
		parent, _, err := client.Repositories.GetCommit(ctx, input.Owner, input.Repo, *ref.Object.SHA, nil)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to get parent commit: %v", err)
		}

		// Create the commit
		commit := &github.Commit{
			Message: github.String(input.Message),
			Tree:    tree,
			Parents: []*github.Commit{{SHA: parent.SHA}},
		}
		newCommit, _, err := client.Git.CreateCommit(ctx, input.Owner, input.Repo, commit, nil)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to create commit: %v", err)
		}

		// Update the reference
		ref.Object.SHA = newCommit.SHA
		_, _, err = client.Git.UpdateRef(ctx, input.Owner, input.Repo, ref, false)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to update ref: %v", err)
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Successfully pushed %d files. Commit SHA: %s", len(input.Files), *newCommit.SHA),
				},
			},
		}, nil
	},
}

var githubSearchRepositories = mcp.Tool{
	Name:        "search_repositories",
	Description: "Search for GitHub repositories",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query"
			},
			"page": {
				"type": "integer",
				"description": "Page number for pagination",
				"default": 1
			},
			"perPage": {
				"type": "integer",
				"description": "Results per page (max 100)",
				"default": 30,
				"maximum": 100
			}
		},
		"required": ["query"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Query   string `json:"query"`
			Page    int    `json:"page"`
			PerPage int    `json:"perPage"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		if input.Page < 1 {
			input.Page = 1
		}
		if input.PerPage < 1 || input.PerPage > 100 {
			input.PerPage = 30
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.SearchOptions{
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		results, _, err := client.Search.Repositories(ctx, input.Query, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format results
		formattedResults := struct {
			TotalCount int                  `json:"total_count"`
			Items      []*github.Repository `json:"items"`
			Page       int                  `json:"page"`
			PerPage    int                  `json:"per_page"`
		}{
			TotalCount: results.GetTotal(),
			Items:      results.Repositories,
			Page:       input.Page,
			PerPage:    input.PerPage,
		}

		resultJSON, err := json.MarshalIndent(formattedResults, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
			},
		}, nil
	},
}

var githubCreatePullRequest = mcp.Tool{
	Name:        "create_pull_request",
	Description: "Create a new pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"title": {
				"type": "string",
				"description": "PR title"
			},
			"body": {
				"type": "string",
				"description": "PR description"
			},
			"head": {
				"type": "string",
				"description": "Branch containing changes"
			},
			"base": {
				"type": "string",
				"description": "Branch to merge into"
			},
			"draft": {
				"type": "boolean",
				"description": "Create as draft PR",
				"default": false
			},
			"maintainer_can_modify": {
				"type": "boolean",
				"description": "Allow maintainer edits",
				"default": true
			}
		},
		"required": ["owner", "repo", "title", "head", "base"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner               string `json:"owner"`
			Repo                string `json:"repo"`
			Title               string `json:"title"`
			Body                string `json:"body"`
			Head                string `json:"head"`
			Base                string `json:"base"`
			Draft               bool   `json:"draft"`
			MaintainerCanModify bool   `json:"maintainer_can_modify"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		newPR := &github.NewPullRequest{
			Title:               github.String(input.Title),
			Body:                github.String(input.Body),
			Head:                github.String(input.Head),
			Base:                github.String(input.Base),
			Draft:               github.Bool(input.Draft),
			MaintainerCanModify: github.Bool(input.MaintainerCanModify),
		}

		pr, _, err := client.PullRequests.Create(ctx, input.Owner, input.Repo, newPR)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(pr, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Pull request created successfully: %s", pr.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubForkRepository = mcp.Tool{
	Name:        "fork_repository",
	Description: "Fork a GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"organization": {
				"type": "string",
				"description": "Organization to fork to"
			}
		},
		"required": ["owner", "repo"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner        string `json:"owner"`
			Repo         string `json:"repo"`
			Organization string `json:"organization,omitempty"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		forkOpts := &github.RepositoryCreateForkOptions{}
		if input.Organization != "" {
			forkOpts.Organization = input.Organization
		}

		fork, _, err := client.Repositories.CreateFork(ctx, input.Owner, input.Repo, forkOpts)
		if err != nil {
			// GitHub returns 202 Accepted when fork is in progress
			if _, ok := err.(*github.AcceptedError); !ok {
				return mcp.CallToolResult{}, err
			}
		}

		resultJSON, err := json.MarshalIndent(fork, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Repository forked successfully: %s", fork.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubCreateBranch = mcp.Tool{
	Name:        "create_branch",
	Description: "Create a new branch in a GitHub repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"branch": {
				"type": "string",
				"description": "Name for new branch"
			},
			"from_branch": {
				"type": "string",
				"description": "Source branch (defaults to repo default)"
			}
		},
		"required": ["owner", "repo", "branch"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			Branch     string `json:"branch"`
			FromBranch string `json:"from_branch"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// If from_branch is not specified, get the default branch
		if input.FromBranch == "" {
			repo, _, err := client.Repositories.Get(ctx, input.Owner, input.Repo)
			if err != nil {
				return mcp.CallToolResult{}, fmt.Errorf("failed to get repository: %v", err)
			}
			input.FromBranch = repo.GetDefaultBranch()
		}

		// Get the SHA of the source branch
		ref, _, err := client.Git.GetRef(ctx, input.Owner, input.Repo, "refs/heads/"+input.FromBranch)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to get source branch reference: %v", err)
		}

		// Create new branch reference
		newRef := &github.Reference{
			Ref: github.String("refs/heads/" + input.Branch),
			Object: &github.GitObject{
				SHA: ref.Object.SHA,
			},
		}

		createdRef, _, err := client.Git.CreateRef(ctx, input.Owner, input.Repo, newRef)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to create branch: %v", err)
		}

		result := struct {
			Name   string `json:"name"`
			Ref    string `json:"ref"`
			SHA    string `json:"sha"`
			Source string `json:"source_branch"`
		}{
			Name:   input.Branch,
			Ref:    createdRef.GetRef(),
			SHA:    createdRef.Object.GetSHA(),
			Source: input.FromBranch,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Branch '%s' created successfully from '%s'", input.Branch, input.FromBranch),
				},
			},
		}, nil
	},
}

var githubListIssues = mcp.Tool{
	Name:        "list_issues",
	Description: "List and filter repository issues",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"state": {
				"type": "string",
				"enum": ["open", "closed", "all"],
				"default": "open"
			},
			"labels": {
				"type": "array",
				"items": {"type": "string"}
			},
			"sort": {
				"type": "string",
				"enum": ["created", "updated", "comments"],
				"default": "created"
			},
			"direction": {
				"type": "string",
				"enum": ["asc", "desc"],
				"default": "desc"
			},
			"since": {
				"type": "string",
				"description": "ISO 8601 timestamp"
			},
			"page": {
				"type": "integer",
				"default": 1
			},
			"per_page": {
				"type": "integer",
				"default": 30,
				"maximum": 100
			}
		},
		"required": ["owner", "repo"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner     string   `json:"owner"`
			Repo      string   `json:"repo"`
			State     string   `json:"state"`
			Labels    []string `json:"labels"`
			Sort      string   `json:"sort"`
			Direction string   `json:"direction"`
			Since     string   `json:"since"`
			Page      int      `json:"page"`
			PerPage   int      `json:"per_page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.IssueListByRepoOptions{
			State:     input.State,
			Labels:    input.Labels,
			Sort:      input.Sort,
			Direction: input.Direction,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		if input.Since != "" {
			since, err := time.Parse(time.RFC3339, input.Since)
			if err != nil {
				return mcp.CallToolResult{}, fmt.Errorf("invalid since date format: %v", err)
			}
			opts.Since = since
		}

		issues, _, err := client.Issues.ListByRepo(ctx, input.Owner, input.Repo, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(issues, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d issues", len(issues)),
				},
			},
		}, nil
	},
}

var githubUpdateIssue = mcp.Tool{
	Name:        "update_issue",
	Description: "Update an existing issue",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"issue_number": {
				"type": "integer",
				"description": "Issue number to update"
			},
			"title": {
				"type": "string",
				"description": "New title"
			},
			"body": {
				"type": "string",
				"description": "New description"
			},
			"state": {
				"type": "string",
				"enum": ["open", "closed"]
			},
			"labels": {
				"type": "array",
				"items": {"type": "string"}
			},
			"assignees": {
				"type": "array",
				"items": {"type": "string"}
			},
			"milestone": {
				"type": "integer",
				"description": "Milestone number"
			}
		},
		"required": ["owner", "repo", "issue_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner       string   `json:"owner"`
			Repo        string   `json:"repo"`
			IssueNumber int      `json:"issue_number"`
			Title       string   `json:"title"`
			Body        string   `json:"body"`
			State       string   `json:"state"`
			Labels      []string `json:"labels"`
			Assignees   []string `json:"assignees"`
			Milestone   int      `json:"milestone"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		issue := &github.IssueRequest{}
		if input.Title != "" {
			issue.Title = github.String(input.Title)
		}
		if input.Body != "" {
			issue.Body = github.String(input.Body)
		}
		if input.State != "" {
			issue.State = github.String(input.State)
		}
		if input.Labels != nil {
			issue.Labels = &input.Labels
		}
		if input.Assignees != nil {
			issue.Assignees = &input.Assignees
		}
		if input.Milestone != 0 {
			issue.Milestone = github.Int(input.Milestone)
		}

		updatedIssue, _, err := client.Issues.Edit(ctx, input.Owner, input.Repo, input.IssueNumber, issue)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(updatedIssue, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Issue #%d updated successfully: %s", input.IssueNumber, updatedIssue.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubAddIssueComment = mcp.Tool{
	Name:        "add_issue_comment",
	Description: "Add a comment to an issue",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"issue_number": {
				"type": "integer",
				"description": "Issue number to comment on"
			},
			"body": {
				"type": "string",
				"description": "Comment text"
			}
		},
		"required": ["owner", "repo", "issue_number", "body"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner       string `json:"owner"`
			Repo        string `json:"repo"`
			IssueNumber int    `json:"issue_number"`
			Body        string `json:"body"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		comment := &github.IssueComment{
			Body: github.String(input.Body),
		}

		createdComment, _, err := client.Issues.CreateComment(ctx, input.Owner, input.Repo, input.IssueNumber, comment)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(createdComment, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Comment added successfully to issue #%d: %s", input.IssueNumber, createdComment.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubSearchCode = mcp.Tool{
	Name:        "search_code",
	Description: "Search for code across GitHub repositories",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"q": {
				"type": "string",
				"description": "Search query using GitHub code search syntax"
			},
			"sort": {
				"type": "string",
				"enum": ["", "indexed"],
				"default": ""
			},
			"order": {
				"type": "string",
				"enum": ["asc", "desc"],
				"default": "desc"
			},
			"per_page": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 30
			},
			"page": {
				"type": "integer",
				"minimum": 1,
				"default": 1
			}
		},
		"required": ["q"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Query   string `json:"q"`
			Sort    string `json:"sort"`
			Order   string `json:"order"`
			PerPage int    `json:"per_page"`
			Page    int    `json:"page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.SearchOptions{
			Sort:  input.Sort,
			Order: input.Order,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		results, _, err := client.Search.Code(ctx, input.Query, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		formattedResults := struct {
			TotalCount int                  `json:"total_count"`
			Items      []*github.CodeResult `json:"items"`
			Page       int                  `json:"page"`
			PerPage    int                  `json:"per_page"`
		}{
			TotalCount: results.GetTotal(),
			Items:      results.CodeResults,
			Page:       input.Page,
			PerPage:    input.PerPage,
		}

		resultJSON, err := json.MarshalIndent(formattedResults, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d code results", results.GetTotal()),
				},
			},
		}, nil
	},
}

var githubSearchIssues = mcp.Tool{
	Name:        "search_issues",
	Description: "Search for issues and pull requests",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"q": {
				"type": "string",
				"description": "Search query using GitHub issues search syntax"
			},
			"sort": {
				"type": "string",
				"enum": ["", "comments", "reactions", "reactions-+1", "reactions--1", "reactions-smile", "reactions-thinking_face", "reactions-heart", "reactions-tada", "interactions", "created", "updated"],
				"default": ""
			},
			"order": {
				"type": "string",
				"enum": ["asc", "desc"],
				"default": "desc"
			},
			"per_page": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 10
			},
			"page": {
				"type": "integer",
				"minimum": 1,
				"default": 1
			}
		},
		"required": ["q"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Query   string `json:"q"`
			Sort    string `json:"sort"`
			Order   string `json:"order"`
			PerPage int    `json:"per_page"`
			Page    int    `json:"page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.SearchOptions{
			Sort:  input.Sort,
			Order: input.Order,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		results, _, err := client.Search.Issues(ctx, input.Query, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		log.Printf("Found %d issues and pull requests", results.GetTotal())

		formattedResults := struct {
			TotalCount int             `json:"total_count"`
			Items      []*github.Issue `json:"items"`
			Page       int             `json:"page"`
			PerPage    int             `json:"per_page"`
		}{
			TotalCount: results.GetTotal(),
			Items:      results.Issues,
			Page:       input.Page,
			PerPage:    input.PerPage,
		}

		resultJSON, err := json.MarshalIndent(formattedResults, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d issues and pull requests", results.GetTotal()),
				},
			},
		}, nil
	},
}

var githubSearchUsers = mcp.Tool{
	Name:        "search_users",
	Description: "Search for GitHub users",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"q": {
				"type": "string",
				"description": "Search query using GitHub users search syntax"
			},
			"sort": {
				"type": "string",
				"enum": ["", "followers", "repositories", "joined"],
				"default": ""
			},
			"order": {
				"type": "string",
				"enum": ["asc", "desc"],
				"default": "desc"
			},
			"per_page": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 30
			},
			"page": {
				"type": "integer",
				"minimum": 1,
				"default": 1
			}
		},
		"required": ["q"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Query   string `json:"q"`
			Sort    string `json:"sort"`
			Order   string `json:"order"`
			PerPage int    `json:"per_page"`
			Page    int    `json:"page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.SearchOptions{
			Sort:  input.Sort,
			Order: input.Order,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		results, _, err := client.Search.Users(ctx, input.Query, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		formattedResults := struct {
			TotalCount int            `json:"total_count"`
			Items      []*github.User `json:"items"`
			Page       int            `json:"page"`
			PerPage    int            `json:"per_page"`
		}{
			TotalCount: results.GetTotal(),
			Items:      results.Users,
			Page:       input.Page,
			PerPage:    input.PerPage,
		}

		resultJSON, err := json.MarshalIndent(formattedResults, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d users", results.GetTotal()),
				},
			},
		}, nil
	},
}

var githubListCommits = mcp.Tool{
	Name:        "list_commits",
	Description: "Gets commits of a branch in a repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"sha": {
				"type": "string",
				"description": "Branch name or commit SHA"
			},
			"page": {
				"type": "integer",
				"minimum": 1,
				"default": 1
			},
			"per_page": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 30
			}
		},
		"required": ["owner", "repo"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner   string `json:"owner"`
			Repo    string `json:"repo"`
			SHA     string `json:"sha"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.CommitsListOptions{
			SHA: input.SHA,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		commits, _, err := client.Repositories.ListCommits(ctx, input.Owner, input.Repo, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format commits to include essential information
		formattedCommits := make([]struct {
			SHA            string `json:"sha"`
			Message        string `json:"message"`
			AuthorName     string `json:"author_name"`
			AuthorEmail    string `json:"author_email"`
			CommitterName  string `json:"committer_name"`
			CommitterEmail string `json:"committer_email"`
			Date           string `json:"date"`
			URL            string `json:"url"`
		}, len(commits))

		for i, commit := range commits {
			formattedCommits[i] = struct {
				SHA            string `json:"sha"`
				Message        string `json:"message"`
				AuthorName     string `json:"author_name"`
				AuthorEmail    string `json:"author_email"`
				CommitterName  string `json:"committer_name"`
				CommitterEmail string `json:"committer_email"`
				Date           string `json:"date"`
				URL            string `json:"url"`
			}{
				SHA:            commit.GetSHA(),
				Message:        commit.GetCommit().GetMessage(),
				AuthorName:     commit.GetCommit().GetAuthor().GetName(),
				AuthorEmail:    commit.GetCommit().GetAuthor().GetEmail(),
				CommitterName:  commit.GetCommit().GetCommitter().GetName(),
				CommitterEmail: commit.GetCommit().GetCommitter().GetEmail(),
				Date:           commit.GetCommit().GetCommitter().GetDate().String(),
				URL:            commit.GetHTMLURL(),
			}
		}

		resultJSON, err := json.MarshalIndent(formattedCommits, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Retrieved %d commits", len(commits)),
				},
			},
		}, nil
	},
}

var githubGetIssue = mcp.Tool{
	Name:        "get_issue",
	Description: "Gets the contents of an issue within a repository",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"issue_number": {
				"type": "integer",
				"description": "Issue number to retrieve"
			}
		},
		"required": ["owner", "repo", "issue_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner       string `json:"owner"`
			Repo        string `json:"repo"`
			IssueNumber int    `json:"issue_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		issue, _, err := client.Issues.Get(ctx, input.Owner, input.Repo, input.IssueNumber)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(issue, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Issue #%d: %s\nState: %s\nCreated: %s\nURL: %s",
						issue.GetNumber(),
						issue.GetTitle(),
						issue.GetState(),
						issue.GetCreatedAt().String(),
						issue.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubGetPullRequest = mcp.Tool{
	Name:        "get_pull_request",
	Description: "Get details of a specific pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			}
		},
		"required": ["owner", "repo", "pull_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get PR details
		pr, _, err := client.PullRequests.Get(ctx, input.Owner, input.Repo, input.PullNumber)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get review status
		reviews, _, err := client.PullRequests.ListReviews(ctx, input.Owner, input.Repo, input.PullNumber, nil)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format the response with PR details and review status
		response := struct {
			*github.PullRequest
			Reviews       []*github.PullRequestReview `json:"reviews"`
			ReviewSummary struct {
				Approved   int `json:"approved"`
				ChangesReq int `json:"changes_requested"`
				Commented  int `json:"commented"`
				Dismissed  int `json:"dismissed"`
				Pending    int `json:"pending"`
			} `json:"review_summary"`
		}{
			PullRequest: pr,
			Reviews:     reviews,
		}

		// Count review states
		for _, review := range reviews {
			switch review.GetState() {
			case "APPROVED":
				response.ReviewSummary.Approved++
			case "CHANGES_REQUESTED":
				response.ReviewSummary.ChangesReq++
			case "COMMENTED":
				response.ReviewSummary.Commented++
			case "DISMISSED":
				response.ReviewSummary.Dismissed++
			case "PENDING":
				response.ReviewSummary.Pending++
			}
		}

		resultJSON, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Pull Request #%d: %s\nState: %s\nMergeable: %v\nReviews: %d approved, %d changes requested\nURL: %s",
						pr.GetNumber(),
						pr.GetTitle(),
						pr.GetState(),
						pr.GetMergeable(),
						response.ReviewSummary.Approved,
						response.ReviewSummary.ChangesReq,
						pr.GetHTMLURL()),
				},
			},
		}, nil
	},
}

var githubListPullRequests = mcp.Tool{
	Name:        "list_pull_requests",
	Description: "List and filter repository pull requests",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"state": {
				"type": "string",
				"enum": ["open", "closed", "all"],
				"default": "open"
			},
			"head": {
				"type": "string",
				"description": "Filter by head user/org and branch"
			},
			"base": {
				"type": "string",
				"description": "Filter by base branch"
			},
			"sort": {
				"type": "string",
				"enum": ["created", "updated", "popularity", "long-running"],
				"default": "created"
			},
			"direction": {
				"type": "string",
				"enum": ["asc", "desc"],
				"default": "desc"
			},
			"per_page": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 30
			},
			"page": {
				"type": "integer",
				"minimum": 1,
				"default": 1
			}
		},
		"required": ["owner", "repo"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner     string `json:"owner"`
			Repo      string `json:"repo"`
			State     string `json:"state"`
			Head      string `json:"head"`
			Base      string `json:"base"`
			Sort      string `json:"sort"`
			Direction string `json:"direction"`
			PerPage   int    `json:"per_page"`
			Page      int    `json:"page"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		opts := &github.PullRequestListOptions{
			State:     input.State,
			Head:      input.Head,
			Base:      input.Base,
			Sort:      input.Sort,
			Direction: input.Direction,
			ListOptions: github.ListOptions{
				Page:    input.Page,
				PerPage: input.PerPage,
			},
		}

		prs, _, err := client.PullRequests.List(ctx, input.Owner, input.Repo, opts)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format pull requests with essential information
		formattedPRs := make([]struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			Head      string `json:"head"`
			Base      string `json:"base"`
			User      string `json:"user"`
			Created   string `json:"created_at"`
			Updated   string `json:"updated_at"`
			Mergeable *bool  `json:"mergeable"`
			URL       string `json:"html_url"`
		}, len(prs))

		for i, pr := range prs {
			formattedPRs[i] = struct {
				Number    int    `json:"number"`
				Title     string `json:"title"`
				State     string `json:"state"`
				Head      string `json:"head"`
				Base      string `json:"base"`
				User      string `json:"user"`
				Created   string `json:"created_at"`
				Updated   string `json:"updated_at"`
				Mergeable *bool  `json:"mergeable"`
				URL       string `json:"html_url"`
			}{
				Number:    pr.GetNumber(),
				Title:     pr.GetTitle(),
				State:     pr.GetState(),
				Head:      pr.GetHead().GetRef(),
				Base:      pr.GetBase().GetRef(),
				User:      pr.GetUser().GetLogin(),
				Created:   pr.GetCreatedAt().String(),
				Updated:   pr.GetUpdatedAt().String(),
				Mergeable: pr.Mergeable,
				URL:       pr.GetHTMLURL(),
			}
		}

		resultJSON, err := json.MarshalIndent(formattedPRs, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d pull requests", len(prs)),
				},
			},
		}, nil
	},
}

var githubCreatePullRequestReview = mcp.Tool{
	Name:        "create_pull_request_review",
	Description: "Create a review on a pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			},
			"body": {
				"type": "string",
				"description": "Review comment text"
			},
			"event": {
				"type": "string",
				"enum": ["APPROVE", "REQUEST_CHANGES", "COMMENT"],
				"description": "Review action"
			},
			"commit_id": {
				"type": "string",
				"description": "SHA of commit to review"
			},
			"comments": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"path": {
							"type": "string",
							"description": "File path"
						},
						"position": {
							"type": "integer",
							"description": "Line position in diff"
						},
						"body": {
							"type": "string",
							"description": "Comment text"
						}
					},
					"required": ["path", "position", "body"]
				}
			}
		},
		"required": ["owner", "repo", "pull_number", "body", "event"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
			Body       string `json:"body"`
			Event      string `json:"event"`
			CommitID   string `json:"commit_id"`
			Comments   []struct {
				Path     string `json:"path"`
				Position int    `json:"position"`
				Body     string `json:"body"`
			} `json:"comments"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Convert input comments to GitHub comments
		comments := make([]*github.DraftReviewComment, len(input.Comments))
		for i, c := range input.Comments {
			comments[i] = &github.DraftReviewComment{
				Path:     &c.Path,
				Position: &c.Position,
				Body:     &c.Body,
			}
		}

		// Create review request
		review := &github.PullRequestReviewRequest{
			Body:     &input.Body,
			Event:    &input.Event,
			Comments: comments,
		}
		if input.CommitID != "" {
			review.CommitID = &input.CommitID
		}

		// Submit review
		result, _, err := client.PullRequests.CreateReview(ctx, input.Owner, input.Repo, input.PullNumber, review)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Created %s review on PR #%d\nReview ID: %d\nState: %s",
						input.Event,
						input.PullNumber,
						result.GetID(),
						result.GetState()),
				},
			},
		}, nil
	},
}

var githubMergePullRequest = mcp.Tool{
	Name:        "merge_pull_request",
	Description: "Merge a pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			},
			"commit_title": {
				"type": "string",
				"description": "Title for merge commit"
			},
			"commit_message": {
				"type": "string",
				"description": "Extra detail for merge commit"
			},
			"merge_method": {
				"type": "string",
				"enum": ["merge", "squash", "rebase"],
				"default": "merge",
				"description": "Merge method to use"
			}
		},
		"required": ["owner", "repo", "pull_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner         string `json:"owner"`
			Repo          string `json:"repo"`
			PullNumber    int    `json:"pull_number"`
			CommitTitle   string `json:"commit_title"`
			CommitMessage string `json:"commit_message"`
			MergeMethod   string `json:"merge_method"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// First check if PR is mergeable
		pr, _, err := client.PullRequests.Get(ctx, input.Owner, input.Repo, input.PullNumber)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		if !pr.GetMergeable() {
			return mcp.CallToolResult{}, fmt.Errorf("pull request #%d is not mergeable", input.PullNumber)
		}

		// Prepare commit message
		commitMessage := input.CommitMessage
		if commitMessage == "" {
			commitMessage = pr.GetBody()
		}

		commitTitle := input.CommitTitle
		if commitTitle == "" {
			commitTitle = pr.GetTitle()
		}

		// Perform merge
		result, _, err := client.PullRequests.Merge(
			ctx,
			input.Owner,
			input.Repo,
			input.PullNumber,
			commitMessage,
			&github.PullRequestOptions{
				CommitTitle: commitTitle,
				MergeMethod: input.MergeMethod,
			},
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Successfully merged PR #%d\nMerge SHA: %s\nMessage: %s",
						input.PullNumber,
						result.GetSHA(),
						result.GetMessage()),
				},
			},
		}, nil
	},
}

var githubGetPullRequestFiles = mcp.Tool{
	Name:        "get_pull_request_files",
	Description: "Get the list of files changed in a pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			}
		},
		"required": ["owner", "repo", "pull_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// List files with options for pagination
		opts := &github.ListOptions{
			PerPage: 100, // Maximum files per page
		}

		var allFiles []*github.CommitFile
		for {
			files, resp, err := client.PullRequests.ListFiles(
				ctx,
				input.Owner,
				input.Repo,
				input.PullNumber,
				opts,
			)
			if err != nil {
				return mcp.CallToolResult{}, err
			}
			allFiles = append(allFiles, files...)
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}

		// Format files with essential information
		formattedFiles := make([]struct {
			Filename    string `json:"filename"`
			Status      string `json:"status"`
			Additions   int    `json:"additions"`
			Deletions   int    `json:"deletions"`
			Changes     int    `json:"changes"`
			BlobURL     string `json:"blob_url"`
			RawURL      string `json:"raw_url"`
			Patch       string `json:"patch,omitempty"`
			SHA         string `json:"sha"`
			PreviousSHA string `json:"previous_sha,omitempty"`
		}, len(allFiles))

		totalChanges := struct {
			Files     int `json:"files"`
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
			Changes   int `json:"changes"`
		}{}

		for i, file := range allFiles {
			formattedFiles[i] = struct {
				Filename    string `json:"filename"`
				Status      string `json:"status"`
				Additions   int    `json:"additions"`
				Deletions   int    `json:"deletions"`
				Changes     int    `json:"changes"`
				BlobURL     string `json:"blob_url"`
				RawURL      string `json:"raw_url"`
				Patch       string `json:"patch,omitempty"`
				SHA         string `json:"sha"`
				PreviousSHA string `json:"previous_sha,omitempty"`
			}{
				Filename:    file.GetFilename(),
				Status:      file.GetStatus(),
				Additions:   file.GetAdditions(),
				Deletions:   file.GetDeletions(),
				Changes:     file.GetChanges(),
				BlobURL:     file.GetBlobURL(),
				RawURL:      file.GetRawURL(),
				Patch:       file.GetPatch(),
				SHA:         file.GetSHA(),
				PreviousSHA: file.GetPreviousFilename(),
			}

			totalChanges.Files++
			totalChanges.Additions += file.GetAdditions()
			totalChanges.Deletions += file.GetDeletions()
			totalChanges.Changes += file.GetChanges()
		}

		resultJSON, err := json.MarshalIndent(struct {
			Files        interface{} `json:"files"`
			TotalChanges interface{} `json:"total_changes"`
		}{
			Files:        formattedFiles,
			TotalChanges: totalChanges,
		}, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("PR #%d changes:\n"+
						"Files changed: %d\n"+
						"Additions: %d\n"+
						"Deletions: %d\n"+
						"Total changes: %d",
						input.PullNumber,
						totalChanges.Files,
						totalChanges.Additions,
						totalChanges.Deletions,
						totalChanges.Changes),
				},
			},
		}, nil
	},
}

type statusCheck struct {
	Context    string    `json:"context"`
	State      string    `json:"state"`
	TargetURL  string    `json:"target_url"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	Creator    string    `json:"creator"`
	AvatarURL  string    `json:"avatar_url"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
}

var githubGetPullRequestStatus = mcp.Tool{
	Name:        "get_pull_request_status",
	Description: "Get the combined status of all status checks for a pull request",
	InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "owner": {
                "type": "string",
                "description": "Repository owner"
            },
            "repo": {
                "type": "string",
                "description": "Repository name"
            },
            "pull_number": {
                "type": "integer",
                "description": "Pull request number"
            }
        },
        "required": ["owner", "repo", "pull_number"]
    }`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get PR to get the head SHA
		pr, _, err := client.PullRequests.Get(ctx, input.Owner, input.Repo, input.PullNumber)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get combined status
		combinedStatus, _, err := client.Repositories.GetCombinedStatus(
			ctx,
			input.Owner,
			input.Repo,
			pr.GetHead().GetSHA(),
			nil,
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get check runs
		checkRuns, _, err := client.Checks.ListCheckRunsForRef(
			ctx,
			input.Owner,
			input.Repo,
			pr.GetHead().GetSHA(),
			&github.ListCheckRunsOptions{},
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format the response
		response := struct {
			CombinedStatus struct {
				State      string    `json:"state"`
				SHA        string    `json:"sha"`
				TotalCount int       `json:"total_count"`
				UpdatedAt  time.Time `json:"updated_at,omitempty"`
			} `json:"combined_status"`
			StatusChecks []statusCheck `json:"status_checks"`
		}{
			CombinedStatus: struct {
				State      string    `json:"state"`
				SHA        string    `json:"sha"`
				TotalCount int       `json:"total_count"`
				UpdatedAt  time.Time `json:"updated_at,omitempty"`
			}{
				State:      stringValue(combinedStatus.State),
				SHA:        stringValue(combinedStatus.SHA),
				TotalCount: intValue(combinedStatus.TotalCount),
			},
		}

		// Add status checks
		for _, status := range combinedStatus.Statuses {
			statusCheck := statusCheck{
				Context:   stringValue(status.Context),
				State:     stringValue(status.State),
				TargetURL: stringValue(status.TargetURL),
				UpdatedAt: timeValue(status.UpdatedAt),
				AvatarURL: stringValue(status.AvatarURL),
			}

			if status.Creator != nil {
				statusCheck.Creator = status.Creator.GetLogin()
			}

			response.StatusChecks = append(response.StatusChecks, statusCheck)
		}

		// Add check runs
		for _, check := range checkRuns.CheckRuns {
			checkResponse := statusCheck{
				Context:    stringValue(check.Name),
				State:      stringValue(check.Status),
				TargetURL:  stringValue(check.HTMLURL),
				UpdatedAt:  timeValue(check.CompletedAt),
				Status:     stringValue(check.Status),
				Conclusion: stringValue(check.Conclusion),
			}

			if check.App != nil && check.App.Owner != nil {
				checkResponse.Creator = stringValue(check.App.Owner.Login)
				checkResponse.AvatarURL = stringValue(check.App.Owner.AvatarURL)
			}

			response.StatusChecks = append(response.StatusChecks, checkResponse)
		}

		resultJSON, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Combined Status: %s\nTotal Checks: %d\nLast Updated: %s",
						response.CombinedStatus.State,
						len(response.StatusChecks),
						response.CombinedStatus.UpdatedAt.Format(time.RFC3339)),
				},
			},
		}, nil
	},
}

var githubUpdatePullRequestBranch = mcp.Tool{
	Name:        "update_pull_request_branch",
	Description: "Update a pull request branch with the latest changes from the base branch",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			},
			"expected_head_sha": {
				"type": "string",
				"description": "Expected SHA of the pull request's HEAD ref"
			}
		},
		"required": ["owner", "repo", "pull_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner           string `json:"owner"`
			Repo            string `json:"repo"`
			PullNumber      int    `json:"pull_number"`
			ExpectedHeadSHA string `json:"expected_head_sha"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		result, _, err := client.PullRequests.UpdateBranch(
			ctx,
			input.Owner,
			input.Repo,
			input.PullNumber,
			&github.PullRequestBranchUpdateOptions{
				ExpectedHeadSHA: github.String(input.ExpectedHeadSHA),
			},
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Branch update message: %s", result.GetMessage()),
				},
			},
		}, nil
	},
}

var githubGetPullRequestComments = mcp.Tool{
	Name:        "get_pull_request_comments",
	Description: "Get the review comments on a pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			}
		},
		"required": ["owner", "repo", "pull_number"]
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Get both review comments and issue comments
		reviewComments, _, err := client.PullRequests.ListComments(
			ctx,
			input.Owner,
			input.Repo,
			input.PullNumber,
			&github.PullRequestListCommentsOptions{},
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		issueComments, _, err := client.Issues.ListComments(
			ctx,
			input.Owner,
			input.Repo,
			input.PullNumber,
			&github.IssueListCommentsOptions{},
		)
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		// Format the comments
		response := struct {
			ReviewComments []struct {
				ID        int64     `json:"id"`
				Body      string    `json:"body"`
				User      string    `json:"user"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				Path      string    `json:"path,omitempty"`
				Position  int       `json:"position,omitempty"`
				CommitID  string    `json:"commit_id,omitempty"`
				HTMLURL   string    `json:"html_url"`
			} `json:"review_comments"`
			IssueComments []struct {
				ID        int64     `json:"id"`
				Body      string    `json:"body"`
				User      string    `json:"user"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				HTMLURL   string    `json:"html_url"`
			} `json:"issue_comments"`
		}{}

		for _, comment := range reviewComments {
			response.ReviewComments = append(response.ReviewComments, struct {
				ID        int64     `json:"id"`
				Body      string    `json:"body"`
				User      string    `json:"user"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				Path      string    `json:"path,omitempty"`
				Position  int       `json:"position,omitempty"`
				CommitID  string    `json:"commit_id,omitempty"`
				HTMLURL   string    `json:"html_url"`
			}{
				ID:        comment.GetID(),
				Body:      comment.GetBody(),
				User:      comment.GetUser().GetLogin(),
				CreatedAt: comment.GetCreatedAt().Time,
				UpdatedAt: comment.GetUpdatedAt().Time,
				Path:      comment.GetPath(),
				Position:  comment.GetPosition(),
				CommitID:  comment.GetCommitID(),
				HTMLURL:   comment.GetHTMLURL(),
			})
		}

		for _, comment := range issueComments {
			response.IssueComments = append(response.IssueComments, struct {
				ID        int64     `json:"id"`
				Body      string    `json:"body"`
				User      string    `json:"user"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				HTMLURL   string    `json:"html_url"`
			}{
				ID:        comment.GetID(),
				Body:      comment.GetBody(),
				User:      comment.GetUser().GetLogin(),
				CreatedAt: comment.GetCreatedAt().Time,
				UpdatedAt: comment.GetUpdatedAt().Time,
				HTMLURL:   comment.GetHTMLURL(),
			})
		}

		resultJSON, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf(
						"Found %d review comments and %d issue comments on PR #%d",
						len(response.ReviewComments),
						len(response.IssueComments),
						input.PullNumber,
					),
				},
			},
		}, nil
	},
}

var githubGetPullRequestReviews = mcp.Tool{
	Name:        "get_pull_request_reviews",
	Description: "Get the reviews on a pull request",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"required": ["owner", "repo", "pull_number"],
		"properties": {
			"owner": {
				"type": "string",
				"description": "Repository owner"
			},
			"repo": {
				"type": "string",
				"description": "Repository name"
			},
			"pull_number": {
				"type": "integer",
				"description": "Pull request number"
			}
		}
	}`),
	Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
		var input struct {
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			PullNumber int    `json:"pull_number"`
		}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return mcp.CallToolResult{}, err
		}

		client, err := getGitHubClient()
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		reviews, _, err := client.PullRequests.ListReviews(
			ctx,
			input.Owner,
			input.Repo,
			input.PullNumber,
			nil,
		)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to get pull request reviews: %v", err)
		}

		var response []struct {
			ID          int64     `json:"id"`
			State       string    `json:"state"`
			Body        string    `json:"body"`
			SubmittedAt time.Time `json:"submitted_at"`
			Reviewer    struct {
				Login     string `json:"login"`
				ID        int64  `json:"id"`
				HTMLURL   string `json:"html_url"`
				AvatarURL string `json:"avatar_url"`
			} `json:"reviewer"`
		}

		for _, review := range reviews {
			reviewer := struct {
				Login     string `json:"login"`
				ID        int64  `json:"id"`
				HTMLURL   string `json:"html_url"`
				AvatarURL string `json:"avatar_url"`
			}{
				Login:     review.User.GetLogin(),
				ID:        review.User.GetID(),
				HTMLURL:   review.User.GetHTMLURL(),
				AvatarURL: review.User.GetAvatarURL(),
			}

			response = append(response, struct {
				ID          int64     `json:"id"`
				State       string    `json:"state"`
				Body        string    `json:"body"`
				SubmittedAt time.Time `json:"submitted_at"`
				Reviewer    struct {
					Login     string `json:"login"`
					ID        int64  `json:"id"`
					HTMLURL   string `json:"html_url"`
					AvatarURL string `json:"avatar_url"`
				} `json:"reviewer"`
			}{
				ID:          review.GetID(),
				State:       review.GetState(),
				Body:        review.GetBody(),
				SubmittedAt: review.GetSubmittedAt().Time,
				Reviewer:    reviewer,
			})
		}

		resultJSON, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.CallToolResult{}, err
		}

		return mcp.CallToolResult{
			Content: []mcp.ToolResultContent{
				{
					Type: "json",
					Text: string(resultJSON),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Found %d reviews on PR #%d", len(response), input.PullNumber),
				},
			},
		}, nil
	},
}
