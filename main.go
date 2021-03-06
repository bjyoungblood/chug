package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bjyoungblood/chug/Godeps/_workspace/src/golang.org/x/oauth2"
	"github.com/bjyoungblood/chug/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"

	"github.com/bjyoungblood/chug/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/bjyoungblood/chug/Godeps/_workspace/src/github.com/google/go-github/github"
	"github.com/bjyoungblood/chug/Godeps/_workspace/src/github.com/libgit2/git2go"
	"github.com/bjyoungblood/chug/Godeps/_workspace/src/github.com/peterh/liner"
)

const VERSION = "0.2.0"

var issueMatcher = regexp.MustCompile(`#\d+`)

var (
	owner    = kingpin.Flag("owner", "Repository owner").Short('o').String()
	repoName = kingpin.Flag("repo", "Repository name").Short('r').String()
	token    = kingpin.Flag("token", "Github API token").Short('t').Required().Default("").OverrideDefaultFromEnvar("GITHUB_API_TOKEN").String()

	path = kingpin.Flag("path", "Path to local repo").Short('p').Default(".").String()
)

// SCP-like URL parsing from https://github.com/motemen/ghq/blob/master/url.go
var hasSchemePattern = regexp.MustCompile("^[^:]+://")
var scpLikeUrlPattern = regexp.MustCompile("^([^@]+@)?([^:]+):/?(.+)$")

func parseUrl(ref string) (*url.URL, error) {
	if !hasSchemePattern.MatchString(ref) && scpLikeUrlPattern.MatchString(ref) {
		matched := scpLikeUrlPattern.FindStringSubmatch(ref)
		user := matched[1]
		host := matched[2]
		path := matched[3]

		ref = fmt.Sprintf("ssh://%s%s/%s", user, host, path)
	}

	url, err := url.Parse(ref)
	if err != nil {
		return url, err
	}

	if !url.IsAbs() {
		if !strings.Contains(url.Path, "/") {
			url.Path = url.Path + "/" + url.Path
		}
		url.Scheme = "https"
		url.Host = "github.com"
		if url.Path[0] != '/' {
			url.Path = "/" + url.Path
		}
	}

	return url, nil
}

func ConvertGitURLHTTPToSSH(url *url.URL) (*url.URL, error) {
	sshURL := fmt.Sprintf("ssh://git@%s/%s", url.Host, url.Path)
	return url.Parse(sshURL)
}

func checkRemotes(repo *git.Repository) error {
	if owner != nil && *owner != "" && repoName != nil && *repoName != "" {
		return nil
	}

	origin, err := repo.LookupRemote("origin")
	if err != nil {
		return err
	}

	defer origin.Free()

	originUrl, err := parseUrl(origin.Url())
	if err != nil {
		return err
	}

	pathParts := strings.Split(originUrl.Path, "/")
	if len(pathParts) != 3 {
		return errors.New("Remote 'origin' doesn't appear to be a Github repository")
	}

	*owner = pathParts[1]
	*repoName = strings.Replace(pathParts[2], ".git", "", -1)

	return nil
}

func readRef(repo *git.Repository, line *liner.State, prompt string) (obj git.Object, err error) {
	spec, err := line.Prompt(prompt)
	if err != nil {
		return
	}

	obj, err = repo.RevparseSingle(spec)
	if err != nil {
		return
	}

	return
}

func extractIssueNumbers(repo *git.Repository, startTag, endTag git.Object) (issues []int, err error) {
	walker, err := repo.Walk()
	if err != nil {
		return nil, err
	}

	defer walker.Free()

	err = walker.PushRange(startTag.Id().String() + ".." + endTag.Id().String())
	if err != nil {
		return nil, err
	}

	issueStrings := make(map[string]struct{})
	err = walker.Iterate(func(commit *git.Commit) bool {
		matches := issueMatcher.FindAllStringSubmatch(commit.Message(), -1)
		for _, match := range matches {
			for _, issueStr := range match {
				issueStrings[issueStr[1:]] = struct{}{}
			}
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	for issueStr, _ := range issueStrings {
		issue, err := strconv.Atoi(issueStr)
		if err != nil {
			logrus.Error(err)
			continue
		}

		issues = append(issues, issue)
	}

	sort.Ints(issues)

	return
}

func getIssues(issueNumbers []int) ([]*github.Issue, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: *token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	ghClient := github.NewClient(tc)

	issues := []*github.Issue{}
	for _, issue := range issueNumbers {
		logrus.Infof("#%d", issue)
		issue, _, err := ghClient.Issues.Get(*owner, *repoName, issue)
		if err != nil {
			logrus.Errorf("Error fetching issue #%d: %v", issue, err)
			continue
		}

		issues = append(issues, issue)
	}

	return issues, nil
}

func formatIssues(issues []*github.Issue) string {
	loglines := make([]string, 0)

	for _, issue := range issues {
		line := fmt.Sprintf(
			"- #[%d](%s) %s",
			*issue.Number,
			*issue.HTMLURL,
			*issue.Title,
		)

		if issue.Assignee != nil {
			line += fmt.Sprintf(" ([%s](%s))", *issue.Assignee.Login, *issue.Assignee.HTMLURL)
		}

		loglines = append(loglines, line)
	}

	return strings.Join(loglines, "\n")
}

func main() {
	kingpin.Version(VERSION)
	kingpin.Parse()

	logrus.SetOutput(os.Stderr)

	repo, err := git.OpenRepository(*path)
	if err != nil {
		logrus.Fatal("Not a git repository")
	}

	defer repo.Free()

	if err := checkRemotes(repo); err != nil {
		logrus.Error(err)
		return
	}

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	startRef, err := readRef(repo, line, "Start ref: ")
	if err == liner.ErrPromptAborted {
		return
	} else if err != nil {
		logrus.Error(err)
		return
	}

	endRef, err := readRef(repo, line, "End ref: ")
	if err == liner.ErrPromptAborted {
		return
	} else if err != nil {
		logrus.Error(err)
		return
	}

	issueNumbers, err := extractIssueNumbers(repo, startRef, endRef)

	logrus.Infof("Found %d issues...", len(issueNumbers))

	issues, err := getIssues(issueNumbers)
	if err != nil {
		logrus.Error(err)
		return
	}

	fmt.Println(formatIssues(issues))
}
