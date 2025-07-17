package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	URL         string `json:"url"`
}

type RepoInfo struct {
	Owner  string
	Repo   string
	Branch string
	Path   string
}

func main() {
	var outputDir string
	var repoLink string

	args := os.Args[1:]

	if len(args) < 1 || len(args) > 3 {
		showUsage()
		os.Exit(1)
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "-d" || arg == "--dir" {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: %s flag requires a directory path\n", arg)
				os.Exit(1)
			}
			outputDir = args[i+1]
			i += 2
		} else if strings.HasPrefix(arg, "-d=") {
			outputDir = arg[3:]
			i++
		} else if strings.HasPrefix(arg, "--dir=") {
			outputDir = arg[6:]
			i++
		} else if strings.HasPrefix(arg, "https://github.com/") {
			repoLink = arg
			i++
		} else {
			fmt.Fprintf(os.Stderr, "Error: Unknown argument: %s\n", arg)
			showUsage()
			os.Exit(1)
		}
	}

	if repoLink == "" {
		fmt.Fprintf(os.Stderr, "Error: GitHub repository link is required\n")
		showUsage()
		os.Exit(1)
	}

	if outputDir == "" {
		outputDir = "."
	}

	repoInfo, err := parseGitHubLink(repoLink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing repository link: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Repository: %s/%s\n", repoInfo.Owner, repoInfo.Repo)
	fmt.Printf("Branch: %s\n", repoInfo.Branch)
	fmt.Printf("Path: %s\n", repoInfo.Path)
	if outputDir != "." {
		fmt.Printf("Output Directory: %s\n", outputDir)
	}
	fmt.Printf("Downloading...\n\n")

	if err := downloadSubdirectory(*repoInfo, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	folderName := filepath.Base(repoInfo.Path)
	finalPath := filepath.Join(outputDir, folderName)
	fmt.Printf("\n✓ Download completed successfully!\n")
	fmt.Printf("Files saved to: %s\n", finalPath)
}

func showUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <github-repo-link>\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Download a specific folder from a GitHub repository.\n\n")

	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  -d, --dir <path>    Download to specific directory (default: current directory)\n\n")

	fmt.Fprintf(os.Stderr, "Examples:\n")

	fmt.Fprintf(os.Stderr, "  %s <github-repo-link>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -d ./downloads <github-repo-link>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --dir=/home/user/projects <github-repo-link>\n", os.Args[0])

	fmt.Fprintf(os.Stderr, "\nThe tool will create a folder with the same name as the target directory.\n")
	fmt.Fprintf(os.Stderr, "Note: Only works with public repositories.\n")
}

func parseGitHubLink(link string) (*RepoInfo, error) {

	link = strings.TrimSuffix(link, "/")

	re := regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/(?:tree|blob)/([^/]+)/(.+)$`)
	matches := re.FindStringSubmatch(link)

	if len(matches) != 5 {
		return nil, fmt.Errorf("invalid GitHub repository link. Expected format: https://github.com/owner/repo/tree/branch/path")
	}

	return &RepoInfo{
		Owner:  matches[1],
		Repo:   matches[2],
		Branch: matches[3],
		Path:   matches[4],
	}, nil
}

func downloadSubdirectory(repoInfo RepoInfo, outputDir string) error {

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	folderName := filepath.Base(repoInfo.Path)
	outputPath := filepath.Join(outputDir, folderName)

	if _, err := os.Stat(outputPath); err == nil {
		fmt.Printf("Directory '%s' already exists. Do you want to overwrite it? (y/N): ", outputPath)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted successfully")
		}
		err := os.RemoveAll(outputPath)
		if err != nil {
			return fmt.Errorf("failed to remove directory: %v ", err)

		}
		fmt.Println("Directory removed.")
	}

	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	return downloadDirectory(repoInfo, repoInfo.Path, outputPath)
}

func downloadDirectory(repoInfo RepoInfo, remotePath, localPath string) error {

	contents, err := getDirectoryContents(repoInfo, remotePath)
	if err != nil {
		return fmt.Errorf("failed to get directory contents for %s: %v", remotePath, err)
	}

	if len(contents) == 0 {
		fmt.Printf("Warning: Directory '%s' is empty or doesn't exist\n", remotePath)
		return nil
	}

	for _, item := range contents {
		itemLocalPath := filepath.Join(localPath, item.Name)

		if item.Type == "dir" {

			if err := os.MkdirAll(itemLocalPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", itemLocalPath, err)
			}

			fmt.Printf("📁 %s/\n", item.Path)
			if err := downloadDirectory(repoInfo, item.Path, itemLocalPath); err != nil {
				return err
			}
		} else if item.Type == "file" {

			fmt.Printf("📄 %s\n", item.Name)
			if err := downloadFile(item.DownloadURL, itemLocalPath); err != nil {
				return fmt.Errorf("failed to download file %s: %v", item.Path, err)
			}
		}
	}

	return nil
}

func getDirectoryContents(repoInfo RepoInfo, path string) ([]GitHubContent, error) {

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		repoInfo.Owner, repoInfo.Repo, path, repoInfo.Branch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "myGithubCli")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("repository, branch, or path not found")
		}
		if resp.StatusCode == 403 {
			return nil, fmt.Errorf("rate limit exceeded or repository is private")
		}
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var contents []GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	return contents, nil
}

func downloadFile(downloadURL, localPath string) error {

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "myGithubCli")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file, status: %d", resp.StatusCode)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file content: %v", err)
	}

	return nil
}
