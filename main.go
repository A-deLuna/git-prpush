package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var dryRunFlag = flag.Bool("dry", false, "Tags commits that will be uploaded in a non-dry run")

func main() {
	flag.Parse()
	paths := findCommitPaths("main")
  var active []string
	for _, p := range paths {
		t := findTipsOfPrs(p)
		if *dryRunFlag {
			active = append(active, tagBranches(t)...)
		} else {
			pushBranches(t)
		}
	}

	removeStaleTags(active)
}

type commit struct {
	sha      string
	message  string
	psBranch string
	isMerge  bool
}

type head struct {
	sha string
	ref string
}

type pushResult struct {
	success bool
	message string
}

func pushBranch(head head) {
	cmd := exec.Command("git", "push", "--force", "origin",
		fmt.Sprintf("%s:refs/heads/%s", head.sha, head.ref))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println(cmd)
	_ = cmd.Run()
}

func tagBranch(head head) {
	cmd := exec.Command("git", "tag", "--force", tagName(head), head.sha)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println(cmd)
	_ = cmd.Run()
}

func deleteTag(tag string) {
	cmd := exec.Command("git", "tag", "--delete", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println(cmd)
	_ = cmd.Run()
}

var BRANCH_PREFIX = "PR_BRANCH"
func tagName(head head) string {
  return fmt.Sprintf("%s/%s", BRANCH_PREFIX, head.ref)
}

func shouldIgnoreRef(ref string) bool {
	ref = strings.ToLower(ref)
	ignore := map[string]struct{}{
		"":     {},
		"null": {},
		"nil":  {}}
	_, ok := ignore[ref]
	return ok
}

var pushed map[string]struct{} = map[string]struct{}{}

func dfsPushes(heads []head, f func(h head)) {
	for _, h := range heads {
		_, ok := pushed[h.sha]
		if shouldIgnoreRef(h.ref) || ok {
			continue
		}
		f(h)
		pushed[h.sha] = struct{}{}
	}

}

func removeStaleTags(active []string) {
  m := make(map[string]struct{})
  for _, t := range active {
    m[t] = struct{}{}
  }
	tags := listTags()
	for _, tag := range tags {
    if !strings.HasPrefix(tag, BRANCH_PREFIX) {
      continue
    }
    if _, ok := m[tag]; ok {
      continue
    }

    deleteTag(tag)
	}
}

func listTags() []string {
	var b bytes.Buffer
	cmd := exec.Command("git", "tag", "--list")
	cmd.Stdout = &b
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error running get sha err: %v", err)
	}

	return strings.Split(strings.TrimSpace(b.String()), "\n")
}

func tagBranches(heads []head) []string {
	var tags []string
	dfsPushes(heads, func(head head) {
		tagBranch(head)
		tags = append(tags, tagName(head))
	})

  return tags
}

func pushBranches(heads []head) {
	dfsPushes(heads, pushBranch)
}

func findTipsOfPrs(commits []commit) []head {
	var stoppers []int
	for i, commit := range commits {
		if commit.psBranch != "" || commit.isMerge {
			stoppers = append(stoppers, i)
		}
	}

	if len(stoppers) == 0 {
		return nil
	}

	var tips []head
	last := 0
	for i := 0; i < len(stoppers); i++ {
		if !commits[stoppers[i]].isMerge && commits[stoppers[i]].psBranch != "" {
			tips = append(tips, head{
				sha: commits[last].sha,
				ref: commits[stoppers[i]].psBranch,
			})
		}
		last = stoppers[i] + 1
	}
	return tips
}

func findBranchTags(commits []commit) []commit {
	for i, commit := range commits {
		commits[i].psBranch = findBranchTag(commit.message)
	}
	return commits
}

func findBranchTag(message string) string {
	message = strings.TrimSpace(message)
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, BRANCH_PREFIX + "=") {
			return strings.TrimPrefix(line, BRANCH_PREFIX + "=")
		}
	}
	return ""
}

func traversePaths(source, target string, path *[]commit, paths *[][]commit) {
	if source == target {
		c := make([]commit, len(*path))
		copy(c, *path)
		*paths = append(*paths, c)
		return
	}

	parents := getParents(source)

	*path = append(*path, makeCommit(source))

	for _, p := range parents {
		traversePaths(p, target, path, paths)
	}

	*path = (*path)[:len(*path)-1]
}

func makeCommit(sha string) commit {
	return commit{
		sha:      sha,
		psBranch: findBranchTag(getMessage(sha)),
		isMerge:  len(getParents(sha)) > 1,
	}
}

func findCommitPaths(branch string) [][]commit {
	var path []commit
	var paths [][]commit

	source := getSha("HEAD")
	target := getSha(branch)

	traversePaths(source, target, &path, &paths)
	return paths
}

func getParents(ref string) []string {
	var b bytes.Buffer
	cmd := exec.Command("git", "show", "--no-patch", "--format=%P", ref)
	cmd.Stdout = &b
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error running get parents err: %v", err)
	}

	return strings.Split(strings.TrimSpace(b.String()), " ")
}

func getSha(ref string) string {
	var b bytes.Buffer
	cmd := exec.Command("git", "show", "--no-patch", "--format=%H", ref)
	cmd.Stdout = &b
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error running get sha err: %v", err)
	}

	return strings.TrimSpace(b.String())
}


func getMessage(sha string) string {
	var b bytes.Buffer
	cmd := exec.Command("git", "show", "--no-patch", "--format=%B", sha)
	cmd.Stdout = &b
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error running get message %v", err)
		log.Fatalf("Error running get message err: %v", err)
	}

	return b.String()
}

